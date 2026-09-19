package radio

import (
	"math"
	"time"

	"github.com/agejevasv/goradion/internal/mpv"
	"github.com/gdamore/tcell/v2"
)

// meterSource is the player, as far as the meter is concerned.
type meterSource interface {
	Level() (level float64, at time.Time, ok bool)
	Spectrum() (bands [mpv.BandCount]float64, at time.Time, ok bool)
}

func (n *nowPlaying) advance(now time.Time, src meterSource) {
	dt := now.Sub(n.levelTick).Seconds()
	if n.levelTick.IsZero() || dt > 0.5 || dt < 0 {
		dt = 0.05
	}
	n.levelTick = now
	playing := n.state() == mpv.Playing

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

// meter draws the spectrum, one cell per band with the bass on the left, or
// the level when the player measures nothing else. It shares the live colour
// with the card's dot: both show sound.
func (n *nowPlaying) meter() []seg {
	if !n.spectrumOK {
		return n.levelMeter()
	}
	const steps = 8
	out := make([]seg, mpv.BandCount)
	for i, level := range n.bands {
		step := min(int(math.Round(level*steps)), steps)
		if step <= 0 {
			out[i] = seg{glyphs.bands[0], styleDim}
			continue
		}
		out[i] = seg{glyphs.bands[(step-1)*len(glyphs.bands)/steps], styleText.Foreground(colorLive)}
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
		return colorLive
	case f <= 0.9:
		return colorWarn
	default:
		return colorDanger
	}
}
