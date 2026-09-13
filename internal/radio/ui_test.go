package radio

import (
	"bytes"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
)

func TestMarquee(t *testing.T) {
	applyTheme(false)
	text := "abcdefghij"
	for _, tc := range []struct {
		width   int
		elapsed time.Duration
		want    string
	}{
		{20, time.Hour, text},
		{6, time.Second, "abcdef"},
		{6, marqueePause + 2*marqueeStep, "cdefgh"},
		{6, marqueePause + 8*marqueeStep, "ij   a"},
		{6, marqueePause + time.Duration(len(text)+marqueeGap)*marqueeStep + time.Second, "abcdef"},
	} {
		if got := marquee(text, tc.width, tc.elapsed); got != tc.want {
			t.Errorf("marquee(%d, %v) = %q, want %q", tc.width, tc.elapsed, got, tc.want)
		}
	}
	if got := sliceCells("日本語", 1, 4); got != " 本 " {
		t.Errorf("wide characters cut in half: %q", got)
	}
}

func TestGaugeAndFit(t *testing.T) {
	applyTheme(false)
	if got := segsText(gauge(10, 0.55, styleText, styleDim)); got != "━━━━━╸────" {
		t.Errorf("gauge = %q", got)
	}
	if got := segsText(gauge(4, 1.5, styleText, styleDim)); got != "━━━━" {
		t.Errorf("full gauge = %q", got)
	}
	segs := []seg{{"Hello ", styleText}, {"world", styleDim}}
	if got := segsText(fitSegs(segs, 20)); got != "Hello world" {
		t.Errorf("fit = %q", got)
	}
	if got := segsText(fitSegs(segs, 8)); got != "Hello w…" {
		t.Errorf("fit = %q", got)
	}

	applyTheme(true)
	defer applyTheme(false)
	if got := segsText(gauge(10, 0.55, styleText, styleDim)); got != "======----" {
		t.Errorf("ascii gauge = %q", got)
	}
}

func TestClockAndHints(t *testing.T) {
	if got := clock(75 * time.Second); got != "1:15" {
		t.Errorf("clock = %q", got)
	}
	if got := clock(3725 * time.Second); got != "1:02:05" {
		t.Errorf("clock = %q", got)
	}
	hints := []hint{{"a", "one"}, {"b", "two"}, {"c", "three"}}
	if got := segsText(hintSegs(hints, 100)); got != "a one   b two   c three" {
		t.Errorf("hints = %q", got)
	}
	if got := segsText(hintSegs(hints, 15)); got != "a one   b two" {
		t.Errorf("hints cut = %q", got)
	}
}

func TestLocale(t *testing.T) {
	for locale, want := range map[string]bool{
		"": true, "C": false, "POSIX": false, "en_US.UTF-8": true, "en_US.utf8": true,
		"de_DE.ISO-8859-1": false, "en_US": true, "sr_RS.UTF-8@latin": true,
	} {
		if got := localeIsUTF8(locale); got != want {
			t.Errorf("localeIsUTF8(%q) = %v", locale, got)
		}
	}
}

func TestLevelAndHistory(t *testing.T) {
	for db, want := range map[float64]float64{0: 1, -16: 0.5, -32: 0, 3: 1, math.Inf(-1): 0} {
		if got := dbToLevel(db); got != want {
			t.Errorf("dbToLevel(%v) = %v", db, got)
		}
	}
	if level, ok := levelFromMetadata(map[string]any{"lavfi.astats.Overall.RMS_level": "-inf"}); !ok || level != 0 {
		t.Errorf("silence = %v %v", level, ok)
	}
	if _, ok := levelFromMetadata(map[string]any{}); ok {
		t.Error("missing level accepted")
	}

	var history []Track
	for i := 0; i < historySize+5; i++ {
		prev := history
		history = appendTrack(history, Track{Song: fmt.Sprint(i)})
		if len(prev) > 0 && prev[len(prev)-1].Song != fmt.Sprint(i-1) {
			t.Fatal("history modified in place")
		}
	}
	if len(history) != historySize || history[0].Song != "5" || history[historySize-1].Song != fmt.Sprint(historySize+4) {
		t.Fatalf("history = %v", history)
	}
}

func TestVUHandshake(t *testing.T) {
	InitLog(false)
	p := NewPlayer()
	var out bytes.Buffer
	reply := func(variant int, err string) {
		out.Reset()
		if !p.handleVU(&out, map[string]any{"request_id": float64(vuRequestID + variant), "error": err}) {
			t.Fatal("reply not consumed")
		}
	}

	p.installVU(&out)
	if !strings.Contains(out.String(), vuFilters[0]) || vuState(p.vu.Load()) != vuTrying {
		t.Fatalf("install: %s", out.String())
	}
	reply(0, "error running command")
	if !strings.Contains(out.String(), vuFilters[1]) {
		t.Fatalf("fallback not tried: %s", out.String())
	}
	reply(1, "success")
	if vuState(p.vu.Load()) != vuOn {
		t.Fatal("filter not on")
	}

	meta := map[string]any{"event": "property-change", "name": "af-metadata/" + vuFilterLabel,
		"data": map[string]any{"lavfi.astats.Overall.RMS_level": "-8.0"}}
	if !p.handleVU(&out, meta) {
		t.Fatal("level not consumed")
	}
	if level, at, ok := p.Level(); !ok || level != 0.75 || time.Since(at) > time.Second {
		t.Fatalf("level = %v %v %v", level, at, ok)
	}
	if p.handleVU(&out, map[string]any{"request_id": float64(0), "error": "success"}) {
		t.Fatal("unrelated reply consumed")
	}

	out.Reset()
	p.suspectVU(&out)
	if !strings.Contains(out.String(), `"remove"`) || vuState(p.vu.Load()) != vuPending {
		t.Fatalf("suspect: %s", out.String())
	}
	p.installVU(&out)
	reply(0, "error")
	reply(1, "error")
	if _, _, ok := p.Level(); ok || vuState(p.vu.Load()) != vuUnavailable {
		t.Fatal("meter should be unavailable")
	}

	q := NewPlayer()
	q.DisableVU()
	out.Reset()
	q.installVU(&out)
	if out.Len() != 0 {
		t.Fatal("disabled meter installed a filter")
	}
}

func TestEarlierHint(t *testing.T) {
	applyTheme(false)
	now := time.Now()
	n := newNowPlaying(nil)
	n.update(Info{Station: "Radio", Url: "http://radio", Song: "B", Volume: 50,
		History: []Track{{Song: "A"}, {Song: "B"}, {Song: "B"}}}, now)
	gauges := func() string { return segsText(n.render(now, 76).rows[2]) }

	if got := gauges(); !strings.HasSuffix(got, "earlier  A") {
		t.Fatalf("gauges row = %q", got)
	}
	n.shuffleActive, n.shuffleStart, n.shuffleInterval = true, now, time.Minute
	if got := gauges(); strings.Contains(got, "earlier") || !strings.Contains(got, "shuffle") {
		t.Fatalf("shuffle should replace the hint: %q", got)
	}
}

func resize(a *Application, screen tcell.SimulationScreen, w, h int) {
	screen.SetSize(w, h)
	a.app.QueueUpdateDraw(func() {})
}

func TestScreenLayouts(t *testing.T) {
	a, screen := newTestApp(t)
	resize(a, screen, 120, 40)
	waitFor(t, "wide layout", func() bool {
		txt := screenText(screen)
		return strings.Contains(txt, "Tags") && strings.Contains(txt, "Now playing") && strings.Contains(txt, "? help")
	})
	if !onUI(a, func() bool { return a.wide }) {
		t.Fatal("120 columns should be wide")
	}

	stations := onUI(a, func() []Station {
		a.openTag("Jazz")
		return a.getStationsFromCurrentView()
	})
	now := time.Now()
	a.app.QueueUpdateDraw(func() {
		a.card.update(Info{Station: stations[1].title, Url: stations[1].url, Song: "Lena Hart Trio - Autumn Leaves",
			Volume: 80, Bitrate: 192, History: []Track{
				{now, "Some Station", "Moss & Ember - Kettle Song"},
				{now, "Some Station", "Juniper Fields - Warm Static"},
				{now, stations[1].title, "Lena Hart Trio - Autumn Leaves"},
			}}, now)
	})
	waitFor(t, "card and marks", func() bool {
		txt := screenText(screen)
		return strings.Contains(txt, "Autumn Leaves") && strings.Contains(txt, "192 kb/s") &&
			strings.Contains(txt, "(b) "+glyphs.play+" "+stations[1].title) &&
			strings.Contains(txt, "earlier  Juniper Fields - Warm Static")
	})

	screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)
	waitFor(t, "tab to tags", func() bool { return onUI(a, a.frontPage) == a.pageNames[Tags] })
	if onUI(a, func() string { return a.tag }) != "Jazz" {
		t.Fatal("tab must keep the tag")
	}
	screen.InjectKey(tcell.KeyDown, 0, tcell.ModNone)
	waitFor(t, "preview next tag", func() bool { return onUI(a, func() string { return a.tag }) != "Jazz" })
	screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)
	waitFor(t, "tab to stations", func() bool { return onUI(a, a.frontPage) == a.pageNames[Main] })

	resize(a, screen, 80, 24)
	waitFor(t, "narrow layout", func() bool {
		txt := screenText(screen)
		return !strings.Contains(txt, "Favorites") && strings.Contains(txt, glyphs.back+" ") && strings.Contains(txt, "earlier")
	})
	if onUI(a, a.calculateStationListOffset) != 2 {
		t.Fatal("narrow station list should start with a back link")
	}
	screen.InjectKey(tcell.KeyEscape, 0, tcell.ModNone)
	waitFor(t, "esc to tags", func() bool {
		return onUI(a, a.frontPage) == a.pageNames[Tags] && strings.Contains(screenText(screen), " Tags ")
	})
}

func segsText(segs []seg) string {
	var sb strings.Builder
	for _, s := range segs {
		sb.WriteString(s.text)
	}
	return sb.String()
}
