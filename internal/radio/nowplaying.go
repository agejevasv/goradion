package radio

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const (
	cardHeight   = 5 // borders and three rows
	volumeFlash  = 1500 * time.Millisecond
	vuCells      = 12
	vuFallPerSec = 1.4
	vuPeakHold   = time.Second
	vuStale      = 700 * time.Millisecond
)

type playState int

const (
	stateIdle playState = iota
	stateStopped
	stateBuffering
	statePlaying
	stateFailed
)

// nowPlaying is only touched on the UI goroutine.
type nowPlaying struct {
	*tview.Box
	app *Application

	info    Info
	hasInfo bool

	playingSince time.Time
	songSince    time.Time
	flashUntil   time.Time

	level, peak float64
	peakAt      time.Time
	bands       [bandCount]float64
	levelTick   time.Time
	meterOK     bool // the player measures the level
	spectrumOK  bool // and the bands too

	// A copy of the shuffle state, see publishShuffle.
	shuffleActive   bool
	shuffleStart    time.Time
	shuffleInterval time.Duration

	gaugeX, gaugeY, gaugeW int

	lastWidth int
	lastFrame string
}

type cardFrame struct {
	title          []seg
	rows           [][]seg
	gaugeX, gaugeW int
}

func newNowPlaying(a *Application) *nowPlaying {
	n := &nowPlaying{Box: tview.NewBox(), app: a, meterOK: true, spectrumOK: true}
	n.SetBorder(true)
	return n
}

func (n *nowPlaying) update(inf Info, now time.Time) {
	prev := n.info
	if n.hasInfo && inf.Volume != prev.Volume {
		n.flash(now)
	}
	if inf.Url != prev.Url || inf.Station != prev.Station || inf.Status == buffering || inf.Status == stopped {
		n.playingSince = time.Time{}
	}
	if inf.Song != prev.Song {
		n.songSince = now
	}
	n.info, n.hasInfo = inf, true
	if n.state() == statePlaying && n.playingSince.IsZero() {
		n.playingSince = now
	}
}

func (n *nowPlaying) flash(now time.Time) {
	n.flashUntil = now.Add(volumeFlash)
}

func (n *nowPlaying) state() playState {
	inf := n.info
	switch {
	case !n.hasInfo || inf.Station == "":
		return stateIdle
	case inf.Status == stopped || inf.Url == "":
		return stateStopped
	case inf.Status == buffering:
		return stateBuffering
	case strings.HasPrefix(inf.Status, "Network or stream issues"):
		return stateFailed
	default:
		return statePlaying
	}
}

func (n *nowPlaying) playingURL() (string, playState) {
	st := n.state()
	if st == stateIdle || st == stateStopped {
		return "", st
	}
	return n.info.Url, st
}

// meterSource is the player, as far as the meter is concerned.
type meterSource interface {
	Level() (level float64, at time.Time, ok bool)
	Spectrum() (bands [bandCount]float64, at time.Time, ok bool)
}

func (n *nowPlaying) advance(now time.Time, src meterSource) {
	dt := now.Sub(n.levelTick).Seconds()
	if n.levelTick.IsZero() || dt > 0.5 || dt < 0 {
		dt = 0.05
	}
	n.levelTick = now
	playing := n.state() == statePlaying

	raw, rawAt, ok := src.Level()
	n.meterOK = ok
	target := 0.0
	if ok && playing && now.Sub(rawAt) < vuStale {
		target = raw
	}
	n.level = follow(n.level, target, dt)
	if n.level >= n.peak {
		n.peak, n.peakAt = n.level, now
	} else if now.Sub(n.peakAt) > vuPeakHold {
		n.peak = max(n.level, n.peak-vuFallPerSec*dt)
	}

	bands, bandsAt, ok := src.Spectrum()
	n.spectrumOK = ok
	fresh := ok && playing && now.Sub(bandsAt) < vuStale
	for i := range n.bands {
		if !fresh {
			bands[i] = 0
		}
		n.bands[i] = follow(n.bands[i], bands[i], dt)
	}
}

// follow jumps up to a louder target and falls slowly towards a quieter one.
func follow(current, target, dt float64) float64 {
	if target >= current {
		return target
	}
	return max(target, current-vuFallPerSec*dt)
}

func (n *nowPlaying) animating(now time.Time) bool {
	st := n.state()
	return st == stateBuffering || st == statePlaying || st == stateFailed ||
		now.Before(n.flashUntil) || n.shuffleActive || n.level > 0 || n.peak > 0 ||
		n.bands != [bandCount]float64{}
}

func (n *nowPlaying) previousTrack() (Track, bool) {
	playing := n.state() == statePlaying
	for i := len(n.info.History) - 1; i >= 0; i-- {
		if t := n.info.History[i]; !playing || t.Song != n.info.Song {
			return t, true
		}
	}
	return Track{}, false
}

func (n *nowPlaying) render(now time.Time, width int) cardFrame {
	var f cardFrame
	inf := n.info
	station := stripPlayCount(inf.Station)
	st := n.state()
	strong := styleText.Bold(true)

	var left, right []seg
	switch st {
	case stateIdle:
		f.title = []seg{{" Now playing ", styleDim}}
		left = []seg{{"Nothing playing", styleText}}
	case stateStopped:
		f.title = []seg{{" Stopped ", styleDim}}
		left = []seg{{glyphs.stop + " ", styleDim}, {station, styleText}}
	case stateBuffering:
		f.title = []seg{{" Tuning in ", styleText.Foreground(colorWarn)}}
		left = []seg{{spinnerFrame(now) + " ", styleText.Foreground(colorWarn)}, {station, strong}}
	case stateFailed:
		f.title = []seg{{" Signal lost ", styleText.Foreground(colorDanger)}}
		left = []seg{{glyphs.fail + " ", styleText.Foreground(colorDanger).Bold(true)}, {station, strong}}
	case statePlaying:
		f.title = []seg{{" Now playing ", styleAccent.Bold(true)}}
		left = []seg{{glyphs.play + " ", styleAccent}, {station, strong}}
	}
	if st == statePlaying || st == stateBuffering {
		if n.meterOK {
			right = append(right, n.meter()...)
		}
		if inf.Bitrate > 0 {
			if len(right) > 0 {
				right = append(right, seg{"  ", styleText})
			}
			right = append(right, seg{fmt.Sprintf("%d kb/s", inf.Bitrate), styleDim})
		}
	}
	f.rows = append(f.rows, spread(left, right, width))

	left, right = nil, nil
	switch st {
	case stateIdle, stateStopped:
		left = []seg{{"Pick a station, or press * for a random one.", styleDim}}
	case stateBuffering:
		left = []seg{{"Buffering" + glyphs.ellipsis, styleDim}}
	case stateFailed:
		left = []seg{{inf.Status, styleText.Foreground(colorDanger)}, {" " + glyphs.dot + " retrying", styleDim}}
	case statePlaying:
		if !n.playingSince.IsZero() {
			right = []seg{{clock(now.Sub(n.playingSince)) + " on air", styleDim}}
		}
		if inf.Song == "" {
			left = []seg{{glyphs.song + " ", styleDim}, {"No track information", styleDim}}
		} else {
			note := seg{glyphs.song + " ", styleAccent}
			room := width - segsWidth([]seg{note})
			if rw := segsWidth(right); rw > 0 {
				room -= rw + 2
			}
			left = []seg{note, {marquee(inf.Song, room, now.Sub(n.songSince)), styleText}}
		}
	}
	f.rows = append(f.rows, spread(left, right, width))

	f.rows = append(f.rows, n.gauges(now, width, &f))
	return f
}

func (n *nowPlaying) gauges(now time.Time, width int, f *cardFrame) []seg {
	fill, pct := styleAccent, styleText
	if now.Before(n.flashUntil) {
		bright := styleText.Foreground(colorAccentBright).Bold(true)
		fill, pct = bright, bright
	}

	const volLabel = "vol "
	half := width
	if n.shuffleActive {
		half = (width - 3) / 2
	}
	volGauge := min(max(half-len(volLabel)-5, 4), 30)
	row := []seg{{volLabel, styleDim}}
	f.gaugeX, f.gaugeW = len(volLabel), volGauge
	row = append(row, gauge(volGauge, float64(n.info.Volume)/100, fill, styleDim)...)
	row = append(row, seg{fmt.Sprintf(" %d%%", n.info.Volume), pct})

	if n.shuffleActive && n.shuffleInterval > 0 {
		remaining := max(n.shuffleInterval-now.Sub(n.shuffleStart), 0)
		label := "shuffle "
		if glyphs.shuffle != "" {
			label = glyphs.shuffle + " " + label
		}
		timeText := " " + clock(remaining+time.Second-1)
		labelW := segsWidth([]seg{{label, styleText}})
		shuffleGauge := min(max(half-labelW-len(timeText), 4), 30)
		elapsed := 1 - remaining.Seconds()/n.shuffleInterval.Seconds()
		right := []seg{{label, styleText.Foreground(colorWarn)}}
		right = append(right, gauge(shuffleGauge, elapsed, styleText.Foreground(colorWarn), styleDim)...)
		right = append(right, seg{timeText, styleText})
		return spread(row, right, width)
	}
	if track, ok := n.previousTrack(); ok {
		const label = "earlier  "
		if room := width - segsWidth(row) - 2; room >= len(label)+8 {
			return spread(row, fitSegs([]seg{{label, styleDim}, {track.Song, styleDim}}, room), width)
		}
	}
	return fitSegs(row, width)
}

// meter draws the spectrum, one cell per band with the bass on the left, or
// the level when the player measures nothing else.
func (n *nowPlaying) meter() []seg {
	if !n.spectrumOK {
		return n.levelMeter()
	}
	const steps = 8
	out := make([]seg, bandCount)
	for i, level := range n.bands {
		step := min(int(math.Round(level*steps)), steps)
		if step <= 0 {
			out[i] = seg{glyphs.bands[0], styleDim}
			continue
		}
		out[i] = seg{glyphs.bands[(step-1)*len(glyphs.bands)/steps], styleAccent}
	}
	return out
}

func (n *nowPlaying) levelMeter() []seg {
	lit := int(math.Round(n.level * vuCells))
	peak := int(math.Round(n.peak*vuCells)) - 1
	out := make([]seg, vuCells)
	for i := range out {
		if i < lit || (i == peak && n.peak > 0.05) {
			out[i] = seg{glyphs.vuLit[i], styleText.Foreground(vuColor(i))}
		} else {
			out[i] = seg{glyphs.vuUnlit[i], styleDim}
		}
	}
	return out
}

func vuColor(cell int) tcell.Color {
	switch f := float64(cell+1) / vuCells; {
	case f <= 0.7:
		return colorAccent
	case f <= 0.9:
		return colorWarn
	default:
		return colorDanger
	}
}

func spread(left, right []seg, width int) []seg {
	rw := segsWidth(right)
	if rw > 0 && width-rw-2 < 12 {
		right, rw = nil, 0
	}
	room := width
	if rw > 0 {
		room -= rw + 2
	}
	out := append([]seg{}, fitSegs(left, room)...)
	if rw > 0 {
		out = append(out, seg{strings.Repeat(" ", width-segsWidth(out)-rw), styleText})
		out = append(out, right...)
	}
	return out
}

func frameKey(f cardFrame) string {
	return fmt.Sprint(f.title, f.rows)
}

func (n *nowPlaying) changed(now time.Time) bool {
	return n.lastWidth > 0 && frameKey(n.render(now, n.lastWidth)) != n.lastFrame
}

func (n *nowPlaying) Draw(screen tcell.Screen) {
	n.DrawForSubclass(screen, n)
	x, y, w, h := n.GetRect()
	width := w - 4
	if width <= 0 || h < 3 {
		return
	}
	f := n.render(time.Now(), width)
	drawSegs(screen, x+2, y, w-4, f.title)
	for i, row := range f.rows {
		if i >= h-2 {
			break
		}
		drawSegs(screen, x+2, y+1+i, width, row)
	}
	n.gaugeX, n.gaugeY, n.gaugeW = x+2+f.gaugeX, y+3, f.gaugeW
	n.lastWidth, n.lastFrame = width, frameKey(f)
}

func (n *nowPlaying) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (bool, tview.Primitive) {
	return n.WrapMouseHandler(func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (bool, tview.Primitive) {
		x, y := event.Position()
		if !n.InRect(x, y) {
			return false, nil
		}
		player := n.app.player
		switch action {
		case tview.MouseScrollUp:
			n.flash(time.Now())
			go player.VolumeUp()
		case tview.MouseScrollDown:
			n.flash(time.Now())
			go player.VolumeDn()
		case tview.MouseLeftClick:
			if y == n.gaugeY && n.gaugeW > 0 && x >= n.gaugeX-1 && x <= n.gaugeX+n.gaugeW {
				fraction := (float64(x-n.gaugeX) + 0.5) / float64(n.gaugeW)
				volume := min(max(int(math.Round(fraction*20))*5, 0), 100)
				n.flash(time.Now())
				go player.SetVolume(volume)
			}
		default:
			return false, nil
		}
		return true, nil
	})
}
