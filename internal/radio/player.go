package radio

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	defaultVolume = 80
	buffering     = "Buffering..."
	stopped       = "Stopped"
	playing       = "Playing"
)

type Player struct {
	sync.Mutex
	Info        chan Info
	cmd         *exec.Cmd
	info        *Info
	retry       *Retry
	savedVolume int
	fadeCancel  context.CancelFunc

	// Atomic, because the UI must never take the player lock.
	vu        atomic.Int32 // a vuState
	meter     atomic.Int32 // index into meterCandidates, kept once mpv takes one
	levelBits atomic.Uint64
	levelAt   atomic.Int64
	bands     atomic.Pointer[[bandCount]float64] // replaced, never modified in place
	bandsAt   atomic.Int64

	// Only touched by the goroutine that reads mpv events.
	sampleRate int
	coreIdle   bool
	watchSince time.Time // when the spectrum watchdog was armed, zero when it is not
	watchTimer *time.Timer
}

type Info struct {
	Status   string
	Station  string
	Song     string
	PrevSong string
	Url      string
	Volume   int
	Bitrate  int
	// Replaced, never modified in place, so copies of Info can share it.
	History []Track
}

type Track struct {
	Time    time.Time
	Station string
	Song    string
}

const historySize = 20

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

type Retry struct {
	ctx    context.Context
	cancel context.CancelFunc
	count  uint64
}

func NewPlayer() *Player {
	return &Player{
		retry: new(Retry),
		Info:  make(chan Info),
		info: &Info{
			Volume: defaultVolume,
		},
	}
}

func (p *Player) Start() {
	p.Lock()
	p.cmd = exec.Command(
		"mpv",
		"-no-video",
		"--idle",
		"--display-tags=Artist,Title,icy-title",
		"--network-timeout=10",
		fmt.Sprintf("--volume=%d", defaultVolume),
		fmt.Sprintf("--input-ipc-server=%s", socket),
	)

	if err := p.cmd.Start(); err != nil {
		fmt.Println(err)
		fmt.Println("Please make sure 'mpv' is available.")
		fmt.Println("Install it using your package manager or visit https://mpv.io for more info.")
		os.Exit(1)
	}

	for i := 1; !mpvIsListening() && i <= 10; i++ {
		if i == 10 {
			fmt.Println("mpv failed to start, quitting")
			os.Exit(1)
		}
		log.Printf("waiting for mpv +%d ms\n", 8<<i)
		time.Sleep((8 << i) * time.Millisecond)
	}
	p.Unlock()

	go p.readMPVEvents()
}

func (p *Player) VolumeUp() {
	p.Lock()
	defer p.Unlock()

	defer func() {
		p.Info <- *p.info
	}()

	if p.info.Volume == 100 {
		return
	}

	log.Printf("setting volume %d\n", p.info.Volume+5)
	cmd := fmt.Sprintf(`{"command": ["set_property", "volume", %d]}%s`, p.info.Volume+5, "\n")
	writeToMPV([]byte(cmd))
	p.info.Volume += 5
}

func (p *Player) VolumeDn() {
	p.Lock()
	defer p.Unlock()

	defer func() {
		p.Info <- *p.info
	}()

	if p.info.Volume == 0 {
		return
	}

	log.Printf("setting volume %d\n", p.info.Volume-5)
	cmd := fmt.Sprintf(`{"command": ["set_property", "volume", %d]}%s`, p.info.Volume-5, "\n")
	writeToMPV([]byte(cmd))
	p.info.Volume -= 5
}

func (p *Player) FadeOut(ctx context.Context, duration time.Duration) {
	p.Lock()
	startVolume := p.info.Volume
	p.savedVolume = startVolume
	p.Unlock()

	if startVolume == 0 {
		return
	}

	steps := 20
	stepDuration := duration / time.Duration(steps)

	for i := 1; i <= steps; i++ {
		select {
		case <-ctx.Done():
			return
		default:
		}

		newVolume := startVolume * (steps - i) / steps

		p.Lock()
		p.info.Volume = newVolume
		cmd := fmt.Sprintf(`{"command": ["set_property", "volume", %d]}%s`, newVolume, "\n")
		writeToMPV([]byte(cmd))
		p.Info <- *p.info
		p.Unlock()

		if i < steps {
			time.Sleep(stepDuration)
		}
	}
}

func (p *Player) FadeIn(ctx context.Context, duration time.Duration) {
	p.Lock()
	targetVolume := p.savedVolume
	if targetVolume == 0 {
		targetVolume = defaultVolume
	}
	p.Unlock()

	steps := 20
	stepDuration := duration / time.Duration(steps)

	for i := 1; i <= steps; i++ {
		select {
		case <-ctx.Done():
			return
		default:
		}

		newVolume := targetVolume * i / steps

		p.Lock()
		p.info.Volume = newVolume
		cmd := fmt.Sprintf(`{"command": ["set_property", "volume", %d]}%s`, newVolume, "\n")
		writeToMPV([]byte(cmd))
		p.Info <- *p.info
		p.Unlock()

		if i < steps {
			time.Sleep(stepDuration)
		}
	}
}

func (p *Player) SetVolume(volume int) {
	p.Lock()
	defer p.Unlock()

	if volume < 0 {
		volume = 0
	}
	if volume > 100 {
		volume = 100
	}

	p.info.Volume = volume
	cmd := fmt.Sprintf(`{"command": ["set_property", "volume", %d]}%s`, volume, "\n")
	writeToMPV([]byte(cmd))
	p.Info <- *p.info
}

func (p *Player) Toggle(station Station) {
	p.Lock()
	defer p.Unlock()

	if station.url == "" {
		return
	}

	if station.url == p.info.Url {
		p.Stop()
		p.info.PrevSong = ""
		p.info.Url = ""
		return
	}

	if p.retry.cancel != nil {
		p.retry.cancel()
	}

	ctx, cancel := context.WithCancel(context.Background())
	p.retry = &Retry{ctx: ctx, cancel: cancel}

	p.info.Station = station.title
	p.info.Url = station.url
	p.info.Status = buffering
	p.info.Bitrate = 0
	p.info.Song = ""
	p.info.PrevSong = ""
	p.Info <- *p.info

	p.Load(station.url)
}

func (p *Player) Stop() {
	if p.retry.cancel != nil {
		p.retry.cancel()
	}
	log.Printf("stopping %s\n", p.info.Url)
	cmd := fmt.Sprintf(`{"command": ["stop"]}%s`, "\n")
	writeToMPV([]byte(cmd))
	p.info.Status = stopped
	p.info.Song = ""
	p.info.Bitrate = 0
	p.Info <- *p.info
}

func (p *Player) Load(url string) {
	if url == "" {
		p.Stop()
		return
	}
	log.Printf("loading %s\n", url)
	cmd := fmt.Sprintf(`{"command": ["loadfile", "%s"]}%s`, url, "\n")
	writeToMPV([]byte(cmd))
	p.info.Url = url
}

func (p *Player) Quit() {
	log.Println("quitting mpv")
	cmd := fmt.Sprintf(`{"command": ["quit", 9]}%s`, "\n")

	if ok := writeToMPV([]byte(cmd)); !ok && p.cmd != nil {
		log.Println("mpv failed to quit via socket")
		p.cmd.Process.Signal(os.Kill)
		p.cmd.Wait()
	}
}

func (p *Player) readMPVEvents() {
	c, err := netDial()
	if err != nil {
		log.Println(err)
		return
	}
	defer c.Close()

	cmds := []string{
		fmt.Sprintf(`{"command": ["observe_property", 1, "filtered-metadata"]}%s`, "\n"),
		fmt.Sprintf(`{"command": ["observe_property", 1, "audio-bitrate"]}%s`, "\n"),
		fmt.Sprintf(`{"command": ["observe_property", 1, "pause"]}%s`, "\n"),
	}

	if vuState(p.vu.Load()) != vuOff {
		cmds = append(cmds,
			fmt.Sprintf(`{"command": ["observe_property", 1, "af-metadata/%s"]}%s`, vuFilterLabel, "\n"),
			fmt.Sprintf(`{"command": ["observe_property", 1, "audio-params/samplerate"]}%s`, "\n"),
			fmt.Sprintf(`{"command": ["observe_property", 1, "core-idle"]}%s`, "\n"),
		)
	}

	for _, cmd := range cmds {
		if _, err = c.Write([]byte(cmd)); err != nil {
			log.Println(err)
		}
	}

	// A new reader per line would drop messages that mpv sends together.
	reader := bufio.NewReader(c)
	for {
		eventBytes, err := reader.ReadBytes('\n')

		if err != nil {
			log.Println("mpv events:", err)
			return
		}

		rsp := unmarshal(eventBytes)

		if p.handleVU(c, rsp) {
			continue
		}

		if eventIs(rsp, "property-change") && nameIs(rsp, "audio-bitrate") {
			br, ok := rsp["data"].(float64)
			if ok {
				p.info.Bitrate = int(math.Round(br / 1000.0))
				p.Info <- *p.info
			}
		} else {
			log.Println(rsp)
		}

		// MPV pauses (at least on Mac) after switching off bluetooth headphones (resuming playback on main soundcard).
		// Let's permanently disable pause, we don't need it.
		if eventIs(rsp, "property-change") && nameIs(rsp, "pause") {
			if pause, ok := rsp["data"].(bool); ok && pause {
				cmd := fmt.Sprintf(`{"command": ["set_property", "pause", false]}%s`, "\n")
				writeToMPV([]byte(cmd))
			}
		}

		if eventIs(rsp, "playback-restart") && p.info.Status == buffering {
			p.setStatusPlaying()
		}

		if eventIs(rsp, "playback-restart") {
			p.installVU(c)
		}

		if eventIs(rsp, "end-file") && reasonIsAnyOf(rsp, "error") {
			p.suspectVU(c)
		}

		if eventIs(rsp, "property-change") && nameIs(rsp, "filtered-metadata") {
			meta, ok := rsp["data"].(map[string]any)
			if ok {
				p.setCurrentSong(meta)
			}
		}

		if eventIs(rsp, "end-file") && reasonIsAnyOf(rsp, "eof", "error", "unknown") {
			go func() {
				select {
				case <-p.retry.ctx.Done():
					log.Println("Retry loading is cancelled")
					return
				case <-time.After((1 << p.retry.count) * time.Second):
					p.retry.count++
					p.info.PrevSong = ""
					p.info.Status = buffering
					p.Info <- *p.info
					p.Load(p.info.Url)
				}
			}()
			p.setStatusUnexpectedEndFile(rsp["reason"].(string))
		}
	}
}

func (p *Player) setStatusPlaying() {
	p.Lock()
	p.info.Status = playing
	p.info.Song = ""
	p.Info <- *p.info
	p.Unlock()
}

func (p *Player) setStatusUnexpectedEndFile(reason string) {
	p.Lock()
	p.info.Status = fmt.Sprintf("Network or stream issues: %s", reason)
	p.info.Song = ""
	p.Info <- *p.info
	p.Unlock()
}

func (p *Player) setCurrentSong(m map[string]any) {
	if p.info.Status == buffering {
		p.setStatusPlaying()
	}

	title, ok := m["icy-title"]

	song := ""

	if ok {
		song = title.(string)
	}

	artist, ok1 := m["Artist"]
	title, ok2 := m["Title"]

	if ok1 && ok2 {
		song = fmt.Sprintf("%s - %s", artist.(string), title.(string))
	}

	p.Lock()
	defer p.Unlock()

	if song != "" && song != p.info.PrevSong {
		p.info.PrevSong = song
		p.info.Status = ""
		p.info.Song = song
		p.info.History = appendTrack(p.info.History, Track{Time: time.Now(), Station: stripPlayCount(p.info.Station), Song: song})
		p.Info <- *p.info
	}
}

func writeToMPV(data []byte) bool {
	c, err := netDial()

	if err != nil {
		log.Println(err, string(data))
		return false
	}

	defer c.Close()

	if _, err = c.Write(data); err != nil {
		log.Println(err, string(data))
		return false
	}

	return true
}

func mpvIsListening() bool {
	_, err := netDial()
	return err == nil
}

func unmarshal(data []byte) map[string]any {
	res := make(map[string]any)

	if err := json.Unmarshal(data, &res); err != nil {
		log.Println(err)
	}

	return res
}

func reasonIsAnyOf(m map[string]any, needle ...string) bool {
	if m["reason"] == nil {
		return false
	}

	return slices.Contains(needle, m["reason"].(string))
}

func eventIs(m map[string]any, needle string) bool {
	return m["event"] != nil && m["event"].(string) == needle
}

func nameIs(m map[string]any, needle ...string) bool {
	if m["name"] == nil {
		return false
	}

	return slices.Contains(needle, m["name"].(string))
}

// DisableVU must be called before Start.
func (p *Player) DisableVU() {
	p.vu.Store(int32(vuOff))
}

// Level is between 0 and 1; ok is false when the meter is off or unsupported.
func (p *Player) Level() (level float64, at time.Time, ok bool) {
	switch vuState(p.vu.Load()) {
	case vuOff, vuUnavailable:
		return 0, time.Time{}, false
	}
	if nanos := p.levelAt.Load(); nanos != 0 {
		at = time.Unix(0, nanos)
	}
	return math.Float64frombits(p.levelBits.Load()), at, true
}

// Spectrum holds band levels between 0 and 1, bass first; ok is false when the
// meter is off or unsupported, or when the filter in use only measures the
// level.
func (p *Player) Spectrum() (bands [bandCount]float64, at time.Time, ok bool) {
	switch vuState(p.vu.Load()) {
	case vuOff, vuUnavailable:
		return bands, time.Time{}, false
	}
	if !meterCandidates[p.meter.Load()].bands {
		return bands, time.Time{}, false
	}
	if b := p.bands.Load(); b != nil {
		bands = *b
	}
	if nanos := p.bandsAt.Load(); nanos != 0 {
		at = time.Unix(0, nanos)
	}
	return bands, at, true
}

// installVU writes to the event connection so that mpv's reply reaches handleVU.
func (p *Player) installVU(c io.Writer) {
	if p.vu.CompareAndSwap(int32(vuPending), int32(vuTrying)) {
		p.addMeter(c, int(p.meter.Load()))
	}
}

func (p *Player) addMeter(c io.Writer, candidate int) {
	p.meter.Store(int32(candidate))
	writeCommand(c, vuRequestID+candidate, "af", "add", meterCandidates[candidate].filter)
}

// suspectVU removes the filter after a stream error in case it was the cause;
// the next playback start adds it back, which mpv refuses if it was.
func (p *Player) suspectVU(c io.Writer) {
	if p.vu.CompareAndSwap(int32(vuOn), int32(vuPending)) {
		writeCommand(c, vuRemoveRequest, "af", "remove", "@"+vuFilterLabel)
		p.levelAt.Store(0)
		p.bandsAt.Store(0)
		p.disarmWatch()
	}
}

func (p *Player) handleVU(c io.Writer, rsp map[string]any) bool {
	if eventIs(rsp, "property-change") {
		switch {
		case nameIs(rsp, "af-metadata/"+vuFilterLabel):
			if meta, ok := rsp["data"].(map[string]any); ok {
				p.storeReading(meta)
			}
			return true
		case nameIs(rsp, "audio-params/samplerate"):
			rate, _ := rsp["data"].(float64)
			p.sampleRate = int(rate)
			return true
		case nameIs(rsp, "core-idle"):
			p.coreIdle, _ = rsp["data"].(bool)
			if p.coreIdle {
				p.disarmWatch()
			} else {
				p.armWatch(c)
			}
			return true
		}
	}

	id, ok := rsp["request_id"].(float64)
	if !ok || int(id) < vuRequestID || int(id) > vuRemoveRequest {
		return false
	}
	candidate := int(id) - vuRequestID
	switch {
	case int(id) == vuRemoveRequest:
		log.Printf("audio meter: filter removed (%v)", rsp["error"])
	case int(id) == spectrumCheckRequest:
		p.checkWatch(c)
	case candidate >= len(meterCandidates):
	case rsp["error"] == "success":
		p.vu.CompareAndSwap(int32(vuTrying), int32(vuOn))
		log.Printf("audio meter: %s filter installed", meterCandidates[candidate].name)
		p.armWatch(c)
	case candidate+1 < len(meterCandidates):
		log.Printf("audio meter: %s filter refused (%v)", meterCandidates[candidate].name, rsp["error"])
		p.addMeter(c, candidate+1)
	default:
		log.Printf("audio meter: unavailable (%v)", rsp["error"])
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
		bands := new([bandCount]float64)
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
			writeCommand(c, spectrumCheckRequest, "get_property", "core-idle")
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
	log.Printf("audio meter: no band readings for %v, replacing the %s filter", spectrumWatchdog, meterCandidates[candidate].name)
	writeCommand(c, vuRemoveRequest, "af", "remove", "@"+vuFilterLabel)
	p.addMeter(c, candidate+1)
}

type meterReading struct {
	level    float64            // dBFS
	bands    [bandCount]float64 // dBFS, bass first
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
	var channels [2 + bandCount]float64
	var seen [2 + bandCount]bool
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

func appendTrack(history []Track, t Track) []Track {
	start := max(len(history)+1-historySize, 0)
	out := make([]Track, 0, len(history)+1-start)
	out = append(out, history[start:]...)
	return append(out, t)
}

func writeCommand(c io.Writer, requestID int, command ...string) {
	b, err := json.Marshal(map[string]any{"command": command, "request_id": requestID})
	if err != nil {
		log.Println(err)
		return
	}
	if _, err := c.Write(append(b, '\n')); err != nil {
		log.Println(err)
	}
}
