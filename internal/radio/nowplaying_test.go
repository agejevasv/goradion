package radio

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/agejevasv/goradion/internal/mpv"
	"github.com/gdamore/tcell/v2"
)

type fakeMeter struct {
	level            float64
	bands            [mpv.BandCount]float64
	levelOK, bandsOK bool
	at               time.Time
}

func (f fakeMeter) Level() (float64, time.Time, bool) { return f.level, f.at, f.levelOK }
func (f fakeMeter) Spectrum() ([mpv.BandCount]float64, time.Time, bool) {
	return f.bands, f.at, f.bandsOK
}

func TestSpectrumCard(t *testing.T) {
	applyGlyphs(false)
	defer applyGlyphs(false)
	defer useTheme(themes[0])
	material, _ := findTheme("material") // its green differs from its accent
	useTheme(material)
	now := time.Now()
	n := newNowPlaying(nil, nil)
	n.update(mpv.Info{State: mpv.Playing, Station: "Radio", URL: "http://radio", Song: "A", Volume: 50, Bitrate: 128}, now)
	src := fakeMeter{level: 0.5, levelOK: true, bandsOK: true, at: now,
		bands: [mpv.BandCount]float64{0, 0.02, 0.1, 0.25, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 0.95, 1}}
	n.advance(now, src)

	cells := n.meter()
	if got := segsText(cells); got != "▁▁▁▂▃▄▅▆▆▇██" {
		t.Errorf("spectrum = %q", got)
	}
	for cell, want := range map[int]tcell.Color{0: colorDim, 1: colorDim, 2: colorLive, 7: colorLive, 11: colorLive} {
		if fg, _, _ := cells[cell].style.Decompose(); fg != want {
			t.Errorf("cell %d is %v, want %v", cell, fg, want)
		}
	}
	if row := segsText(n.render(now, 76).rows[0]); !strings.HasSuffix(row, "▁▁▁▂▃▄▅▆▆▇██  128 kb/s") {
		t.Errorf("first row = %q", row)
	}
	applyGlyphs(true)
	if got := segsText(n.meter()); got != "....::|||###" {
		t.Errorf("ascii spectrum = %q", got)
	}
	applyGlyphs(false)

	// Bars fall slowly when the music gets quieter.
	later := now.Add(100 * time.Millisecond)
	src.bands, src.at = [mpv.BandCount]float64{}, later
	n.advance(later, src)
	if math.Abs(n.bands[mpv.BandCount-1]-(1-vuFallPerSec*0.1)) > 1e-9 || n.bands[0] != 0 {
		t.Errorf("bands after 100 ms = %v", n.bands)
	}

	// Without a spectrum the same slot shows the level meter.
	src.bandsOK = false
	n.advance(later.Add(100*time.Millisecond), src)
	cells = n.meter()
	lit, _, _ := cells[5].style.Decompose()
	unlit, _, _ := cells[6].style.Decompose()
	if len(cells) != vuCells || lit != colorLive || unlit != colorDim {
		t.Errorf("level meter = %q", segsText(cells))
	}

	n.advance(later.Add(200*time.Millisecond), fakeMeter{})
	if row := segsText(n.render(later, 76).rows[0]); strings.ContainsAny(row, "▁█") {
		t.Errorf("meter shown without one: %q", row)
	}
}

func TestEarlierHint(t *testing.T) {
	applyGlyphs(false)
	now := time.Now()
	n := newNowPlaying(nil, nil)
	n.update(mpv.Info{State: mpv.Playing, Station: "Radio", URL: "http://radio", Song: "B", Volume: 50,
		History: []mpv.Track{{Song: "A"}, {Song: "B"}, {Song: "B"}}}, now)
	gauges := func() string { return segsText(n.render(now, 76).rows[2]) }

	if got := gauges(); !strings.HasSuffix(got, "earlier  A") {
		t.Fatalf("gauges row = %q", got)
	}
	n.shuffle = &shuffle{active: true, start: now, interval: time.Minute}
	if got := gauges(); strings.Contains(got, "earlier") || !strings.Contains(got, "shuffle") {
		t.Fatalf("shuffle should replace the hint: %q", got)
	}
}

func TestLiveGlow(t *testing.T) {
	defer useTheme(themes[0])
	material, _ := findTheme("material")
	useTheme(material)
	start := time.UnixMilli(0)
	if got := liveGlow(start); got != colorLive {
		t.Errorf("the glow starts lit, got %v", got)
	}
	if got := liveGlow(start.Add(glowPeriod / 2)); got != colorDim {
		t.Errorf("half a period on it is dim, got %v", got)
	}
	if got := liveGlow(start.Add(glowPeriod / 4)); got == colorLive || got == colorDim {
		t.Errorf("in between it blends, got %v", got)
	}
	useTheme(themes[0])
	if got := liveGlow(start.Add(glowPeriod / 4)); got != colorDim && got != colorLive {
		t.Errorf("the terminal theme blinks rather than blends, got %v", got)
	}
}
