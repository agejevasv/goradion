package radio

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/agejevasv/goradion/internal/mpv"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const (
	glowPeriod   = 2 * time.Second
	cardHeight   = 5 // borders and three rows
	volumeFlash  = 1500 * time.Millisecond
	vuCells      = 12
	vuFallPerSec = 1.4
	vuPeakHold   = time.Second
	vuStale      = 700 * time.Millisecond
)

// nowPlaying is only touched on the UI goroutine.
type nowPlaying struct {
	*tview.Box
	player  *mpv.Player
	shuffle *shuffle

	info    mpv.Info
	hasInfo bool

	playingSince time.Time
	songSince    time.Time
	flashUntil   time.Time

	level, peak float64
	peakAt      time.Time
	bands       [mpv.BandCount]float64
	levelTick   time.Time
	meterOK     bool // the player measures the level
	spectrumOK  bool // and the bands too

	gaugeX, gaugeY, gaugeW int

	lastWidth int
	lastFrame string
}

type cardFrame struct {
	title          []seg
	rows           [][]seg
	gaugeX, gaugeW int
}

func newNowPlaying(player *mpv.Player, sh *shuffle) *nowPlaying {
	n := &nowPlaying{Box: tview.NewBox(), player: player, shuffle: sh, meterOK: true, spectrumOK: true}
	n.SetBorder(true)
	return n
}

func (n *nowPlaying) update(inf mpv.Info, now time.Time) {
	prev := n.info
	if n.hasInfo && inf.Volume != prev.Volume {
		n.flash(now)
	}
	if inf.URL != prev.URL || inf.Station != prev.Station || inf.State == mpv.Buffering || inf.State == mpv.Stopped {
		n.playingSince = time.Time{}
	}
	if inf.Song != prev.Song {
		n.songSince = now
	}
	n.info, n.hasInfo = inf, true
	if n.state() == mpv.Playing && n.playingSince.IsZero() {
		n.playingSince = now
	}
}

func (n *nowPlaying) flash(now time.Time) {
	n.flashUntil = now.Add(volumeFlash)
}

func (n *nowPlaying) state() mpv.State {
	inf := n.info
	switch {
	case !n.hasInfo || inf.Station == "":
		return mpv.Idle
	case inf.URL == "":
		return mpv.Stopped
	}
	return inf.State
}

func (n *nowPlaying) playingURL() (string, mpv.State) {
	st := n.state()
	if st == mpv.Idle || st == mpv.Stopped {
		return "", st
	}
	return n.info.URL, st
}

func (n *nowPlaying) animating(now time.Time) bool {
	st := n.state()
	return st == mpv.Buffering || st == mpv.Playing || st == mpv.Failed ||
		now.Before(n.flashUntil) || n.shuffling() || n.level > 0 || n.peak > 0 ||
		n.bands != [mpv.BandCount]float64{}
}

func (n *nowPlaying) shuffling() bool {
	return n.shuffle != nil && n.shuffle.active && n.shuffle.interval > 0
}

func (n *nowPlaying) previousTrack() (mpv.Track, bool) {
	playing := n.state() == mpv.Playing
	for i := len(n.info.History) - 1; i >= 0; i-- {
		if t := n.info.History[i]; !playing || t.Song != n.info.Song {
			return t, true
		}
	}
	return mpv.Track{}, false
}

func (n *nowPlaying) render(now time.Time, width int) cardFrame {
	var f cardFrame
	inf := n.info
	station := inf.Station
	st := n.state()
	strong := styleText.Bold(true)

	var left, right []seg
	switch st {
	case mpv.Idle:
		f.title = []seg{{" Now playing ", styleDim}}
		left = []seg{{"Nothing playing", styleText}}
	case mpv.Stopped:
		f.title = []seg{{" Stopped ", styleDim}}
		left = []seg{{glyphs.stop + " ", styleDim}, {station, styleText}}
	case mpv.Buffering:
		f.title = []seg{{" Tuning in ", styleText.Foreground(colorWarn)}}
		left = []seg{{spinnerFrame(now) + " ", styleText.Foreground(colorWarn)}, {station, strong}}
	case mpv.Failed:
		f.title = []seg{{" Signal lost ", styleText.Foreground(colorDanger)}}
		left = []seg{{glyphs.fail + " ", styleText.Foreground(colorDanger).Bold(true)}, {station, strong}}
	case mpv.Playing:
		f.title = []seg{{" Now playing ", styleAccent.Bold(true)}}
		left = []seg{{glyphs.play + " ", styleAccent}, {station, strong}}
	}
	// The dot glows only while there is sound.
	dot := styleDim
	if st == mpv.Playing {
		dot = styleText.Foreground(liveGlow(now))
	}
	f.title = append([]seg{{" " + glyphs.live, dot}}, f.title...)
	if st == mpv.Playing || st == mpv.Buffering {
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
	case mpv.Idle, mpv.Stopped:
		left = []seg{{"Pick a station, or press * for a random one.", styleDim}}
	case mpv.Buffering:
		left = []seg{{"Buffering" + glyphs.ellipsis, styleDim}}
	case mpv.Failed:
		left = []seg{{inf.Status, styleText.Foreground(colorDanger)}, {" " + glyphs.dot + " retrying", styleDim}}
	case mpv.Playing:
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
		bright := styleText.Foreground(colorBright).Bold(true)
		fill, pct = bright, bright
	}

	const volLabel = "vol "
	half := width
	if n.shuffling() {
		half = (width - 3) / 2
	}
	volGauge := min(max(half-len(volLabel)-5, 4), 30)
	row := []seg{{volLabel, styleDim}}
	f.gaugeX, f.gaugeW = len(volLabel), volGauge
	row = append(row, gauge(volGauge, float64(n.info.Volume)/100, fill, styleDim)...)
	row = append(row, seg{fmt.Sprintf(" %d%%", n.info.Volume), pct})

	if n.shuffling() {
		remaining := n.shuffle.remaining(now)
		label := "shuffle "
		if glyphs.shuffle != "" {
			label = glyphs.shuffle + " " + label
		}
		timeText := " " + clock(remaining+time.Second-1)
		labelW := segsWidth([]seg{{label, styleText}})
		shuffleGauge := min(max(half-labelW-len(timeText), 4), 30)
		elapsed := 1 - remaining.Seconds()/n.shuffle.interval.Seconds()
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
		switch action {
		case tview.MouseScrollUp:
			n.flash(time.Now())
			n.player.ChangeVolume(mpv.VolumeStep)
		case tview.MouseScrollDown:
			n.flash(time.Now())
			n.player.ChangeVolume(-mpv.VolumeStep)
		case tview.MouseLeftClick:
			if y == n.gaugeY && n.gaugeW > 0 && x >= n.gaugeX-1 && x <= n.gaugeX+n.gaugeW {
				fraction := (float64(x-n.gaugeX) + 0.5) / float64(n.gaugeW)
				n.flash(time.Now())
				n.player.SetVolume(int(math.Round(fraction*100/mpv.VolumeStep)) * mpv.VolumeStep)
			}
		default:
			return false, nil
		}
		return true, nil
	})
}

// liveGlow fades the dot from the live colour to dim and back once per
// glowPeriod. The terminal's own colours can't be blended, so there it blinks.
func liveGlow(now time.Time) tcell.Color {
	phase := float64(now.UnixMilli()%glowPeriod.Milliseconds()) / float64(glowPeriod.Milliseconds())
	glow := 0.5 + 0.5*math.Cos(2*math.Pi*phase)
	if colorBg == tcell.ColorDefault {
		if glow < 0.5 {
			return colorDim
		}
		return colorLive
	}
	return mix(colorDim, colorLive, glow)
}
