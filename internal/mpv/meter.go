package mpv

import (
	"io"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/agejevasv/goradion/internal/logging"
)

type vuState int32

const (
	vuPending vuState = iota
	vuTrying
	vuOn
	vuUnavailable
	vuOff
)

// The meter filter is added once audio plays: mpv then refuses a bad filter
// and plays on, while a bad filter given at start-up or while idle makes every
// station fail. The handshake walks the candidates until mpv takes one.
const vuFilterLabel = "goradionvu"

type meterCandidate struct {
	name   string
	filter string
	bands  bool // measures the bands as well as the level
}

var meterCandidates = []meterCandidate{
	{"spectrum", "@" + vuFilterLabel + ":lavfi=[" + spectrumGraph() + "]", true},
	{"level", "@" + vuFilterLabel + ":lavfi=[astats=metadata=1:reset=1:measure_perchannel=none:measure_overall=RMS_level]", false},
	// Reports every statistic, for builds that refuse the measure options.
	{"plain level", "@" + vuFilterLabel + ":lavfi=[astats=metadata=1:reset=1]", false},
}

const (
	vuRequestID          = 7300 // + index into meterCandidates
	spectrumCheckRequest = 7398
	vuRemoveRequest      = 7399
	vuFloorDB            = -32.0
	spectrumWatchdog     = 3 * time.Second
)

// DisableVU must be called before Start.
func (p *Player) DisableVU() {
	p.vu.Store(int32(vuOff))
}

func (p *Player) meterAvailable() bool {
	switch vuState(p.vu.Load()) {
	case vuOff, vuUnavailable:
		return false
	}
	return true
}

// Level is between 0 and 1; ok is false when the meter is off or unsupported.
func (p *Player) Level() (level float64, at time.Time, ok bool) {
	if !p.meterAvailable() {
		return 0, time.Time{}, false
	}
	return math.Float64frombits(p.levelBits.Load()), unixNanoTime(p.levelAt.Load()), true
}

// Spectrum holds band levels between 0 and 1, bass first; ok is false when the
// meter is off or unsupported, or when the filter in use only measures the
// level.
func (p *Player) Spectrum() (bands [BandCount]float64, at time.Time, ok bool) {
	if !p.meterAvailable() || !meterCandidates[p.meter.Load()].bands {
		return bands, time.Time{}, false
	}
	if b := p.bands.Load(); b != nil {
		bands = *b
	}
	return bands, unixNanoTime(p.bandsAt.Load()), true
}

func unixNanoTime(nanos int64) time.Time {
	if nanos == 0 {
		return time.Time{}
	}
	return time.Unix(0, nanos)
}

// installVU writes to the event connection so that mpv's reply reaches handleVU.
func (p *Player) installVU(c io.Writer) {
	if p.vu.CompareAndSwap(int32(vuPending), int32(vuTrying)) {
		p.addMeter(c, int(p.meter.Load()))
	}
}

func (p *Player) addMeter(c io.Writer, candidate int) {
	p.meter.Store(int32(candidate))
	vuCommand(c, vuRequestID+candidate, "af", "add", meterCandidates[candidate].filter)
}

func (p *Player) removeMeter(c io.Writer) {
	vuCommand(c, vuRemoveRequest, "af", "remove", "@"+vuFilterLabel)
}

func vuCommand(c io.Writer, requestID int, args ...any) {
	if err := sendCommand(c, requestID, args...); err != nil {
		logging.Println(err)
	}
}

// suspectVU removes the filter after a stream error in case it was the cause;
// the next playback start adds it back, which mpv refuses if it was.
func (p *Player) suspectVU(c io.Writer) {
	if p.vu.CompareAndSwap(int32(vuOn), int32(vuPending)) {
		p.removeMeter(c)
		p.levelAt.Store(0)
		p.bandsAt.Store(0)
		p.disarmWatch()
	}
}

// handleVU consumes the messages that belong to the meter.
func (p *Player) handleVU(c io.Writer, m mpvMessage) bool {
	switch {
	case m.isPropertyChange("af-metadata/" + vuFilterLabel):
		if meta, ok := m["data"].(map[string]any); ok {
			p.storeReading(meta)
		}
		return true
	case m.isPropertyChange("audio-params/samplerate"):
		rate, _ := m["data"].(float64)
		p.sampleRate = int(rate)
		return true
	case m.isPropertyChange("core-idle"):
		p.coreIdle, _ = m["data"].(bool)
		if p.coreIdle {
			p.disarmWatch()
		} else {
			p.armWatch(c)
		}
		return true
	}

	id := m.requestID()
	if id < vuRequestID || id > vuRemoveRequest {
		return false
	}
	candidate := id - vuRequestID
	switch {
	case id == vuRemoveRequest:
		logging.Printf("audio meter: filter removed (%v)", m["error"])
	case id == spectrumCheckRequest:
		p.checkWatch(c)
	case candidate >= len(meterCandidates):
	case m["error"] == "success":
		p.vu.CompareAndSwap(int32(vuTrying), int32(vuOn))
		logging.Printf("audio meter: %s filter installed", meterCandidates[candidate].name)
		p.armWatch(c)
	case candidate+1 < len(meterCandidates):
		logging.Printf("audio meter: %s filter refused (%v)", meterCandidates[candidate].name, m["error"])
		p.addMeter(c, candidate+1)
	default:
		logging.Printf("audio meter: unavailable (%v)", m["error"])
		p.vu.CompareAndSwap(int32(vuTrying), int32(vuUnavailable))
	}
	return true
}

func (p *Player) storeReading(meta map[string]any) {
	r := readMeter(meta)
	now := time.Now().UnixNano()
	if r.hasLevel {
		p.levelBits.Store(math.Float64bits(dbToLevel(r.level)))
		p.levelAt.Store(now)
	}
	if r.hasBands {
		bands := new([BandCount]float64)
		for i, db := range r.bands {
			bands[i] = bandLevel(i, db, p.sampleRate)
		}
		p.bands.Store(bands)
		p.bandsAt.Store(now)
	}
}

// armWatch starts the spectrum watchdog while audio plays. The timer must not
// touch the event state, so it asks mpv a harmless question and checkWatch
// runs when the answer arrives.
func (p *Player) armWatch(c io.Writer) {
	if p.coreIdle || vuState(p.vu.Load()) != vuOn || !meterCandidates[p.meter.Load()].bands {
		return
	}
	p.watchSince = time.Now()
	if p.watchTimer == nil {
		p.watchTimer = time.AfterFunc(spectrumWatchdog, func() {
			vuCommand(c, spectrumCheckRequest, "get_property", "core-idle")
		})
	} else {
		p.watchTimer.Reset(spectrumWatchdog)
	}
}

func (p *Player) disarmWatch() {
	p.watchSince = time.Time{}
	if p.watchTimer != nil {
		p.watchTimer.Stop()
	}
}

// checkWatch replaces the spectrum with the next candidate when no band
// reading arrived while the watchdog was armed: FFmpeg 4.3 and older fail the
// graph only once audio flows, and mpv then plays on without it.
func (p *Player) checkWatch(c io.Writer) {
	since := p.watchSince
	if since.IsZero() || time.Since(since) < spectrumWatchdog {
		return
	}
	p.watchSince = time.Time{}
	candidate := int(p.meter.Load())
	if p.bandsAt.Load() >= since.UnixNano() || !meterCandidates[candidate].bands ||
		candidate+1 >= len(meterCandidates) || !p.vu.CompareAndSwap(int32(vuOn), int32(vuTrying)) {
		return
	}
	logging.Printf("audio meter: no band readings for %v, replacing the %s filter", spectrumWatchdog, meterCandidates[candidate].name)
	p.removeMeter(c)
	p.addMeter(c, candidate+1)
}

type meterReading struct {
	level    float64            // dBFS
	bands    [BandCount]float64 // dBFS, bass first
	hasLevel bool
	hasBands bool
}

// readMeter parses one af-metadata update. The level filters report
// lavfi.astats.Overall.RMS_level. The spectrum graph reports
// lavfi.astats.N.RMS_level for channel N-1 instead: channels 0 and 1 are the
// stereo, whose louder side gives the level, and the rest are the bands.
// Silence is "-inf".
func readMeter(meta map[string]any) meterReading {
	var r meterReading
	var channels [2 + BandCount]float64
	var seen [2 + BandCount]bool
	for key, value := range meta {
		name, ok := strings.CutPrefix(key, "lavfi.astats.")
		if !ok {
			continue
		}
		if name, ok = strings.CutSuffix(name, ".RMS_level"); !ok {
			continue
		}
		s, ok := value.(string)
		if !ok {
			continue
		}
		db, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
		if err != nil {
			continue
		}
		if name == "Overall" {
			r.level, r.hasLevel = db, true
		} else if n, err := strconv.Atoi(name); err == nil && n >= 1 && n <= len(channels) {
			channels[n-1], seen[n-1] = db, true
		}
	}
	if !r.hasLevel && seen[0] {
		r.level, r.hasLevel = channels[0], true
		if seen[1] {
			r.level = max(r.level, channels[1])
		}
	}
	r.hasBands = !slices.Contains(seen[2:], false)
	copy(r.bands[:], channels[2:])
	return r
}

func dbToLevel(db float64) float64 {
	if math.IsNaN(db) {
		return 0
	}
	return min(max(1-db/vuFloorDB, 0), 1)
}
