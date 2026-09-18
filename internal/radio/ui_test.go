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
	applyGlyphs(false)
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
	applyGlyphs(false)
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

	applyGlyphs(true)
	defer applyGlyphs(false)
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

func TestReadMeter(t *testing.T) {
	if r := readMeter(map[string]any{"lavfi.astats.Overall.RMS_level": "-inf"}); !r.hasLevel || !math.IsInf(r.level, -1) || r.hasBands {
		t.Errorf("silence = %+v", r)
	}
	if r := readMeter(map[string]any{}); r.hasLevel || r.hasBands {
		t.Errorf("empty update = %+v", r)
	}

	meta := map[string]any{"lavfi.astats.1.RMS_level": "-12.5", "lavfi.astats.2.RMS_level": "-9.25"}
	for band := range bandCount {
		meta[fmt.Sprintf("lavfi.astats.%d.RMS_level", band+3)] = fmt.Sprint(-20 - band)
	}
	if r := readMeter(meta); !r.hasLevel || r.level != -9.25 || !r.hasBands || r.bands[0] != -20 || r.bands[bandCount-1] != -31 {
		t.Errorf("spectrum = %+v", r)
	}
	delete(meta, "lavfi.astats.14.RMS_level")
	if r := readMeter(meta); !r.hasLevel || r.hasBands {
		t.Errorf("missing band = %+v", r)
	}

	// The plain level filter reports every channel as well as the overall level.
	plain := map[string]any{"lavfi.astats.1.RMS_level": "-3.0", "lavfi.astats.1.Peak_level": "0.0",
		"lavfi.astats.Overall.RMS_level": "-6.0"}
	if r := readMeter(plain); r.level != -6 || r.hasBands {
		t.Errorf("plain level = %+v", r)
	}
}

func TestSpectrumGraph(t *testing.T) {
	want := "aformat=channel_layouts=stereo,asplit=2[main][mono];" +
		"[mono]aformat=channel_layouts=mono,asplit=12[b0][b1][b2][b3][b4][b5][b6][b7][b8][b9][b10][b11];" +
		"[b0]bandpass=f=40:width_type=o:w=1[m0];[b1]bandpass=f=67:width_type=o:w=1[m1];" +
		"[b2]bandpass=f=113:width_type=o:w=1[m2];[b3]bandpass=f=190:width_type=o:w=1[m3];" +
		"[b4]bandpass=f=318:width_type=o:w=1[m4];[b5]bandpass=f=535:width_type=o:w=1[m5];" +
		"[b6]bandpass=f=898:width_type=o:w=1[m6];[b7]bandpass=f=1508:width_type=o:w=1[m7];" +
		"[b8]bandpass=f=2533:width_type=o:w=1[m8];[b9]bandpass=f=4254:width_type=o:w=1[m9];" +
		"[b10]bandpass=f=7145:width_type=o:w=1[m10];[b11]bandpass=f=12000:width_type=o:w=1[m11];" +
		"[main][m0][m1][m2][m3][m4][m5][m6][m7][m8][m9][m10][m11]amerge=inputs=13," +
		"astats=threads=1:metadata=1:reset=1:measure_perchannel=RMS_level:measure_overall=none," +
		"pan=stereo|c0=c0|c1=c1"
	if got := spectrumGraph(); got != want {
		t.Errorf("graph =\n%s\nwant\n%s", got, want)
	}
	for i, f := range bandFreqs {
		if want := math.Round(40 * math.Pow(300, float64(i)/(bandCount-1))); float64(f) != want {
			t.Errorf("band %d is %d Hz, want %v", i, f, want)
		}
	}
}

func TestBandLevel(t *testing.T) {
	// Pink noise at -17 dBFS puts every band near -24.5 dBFS.
	bass, mid, treble := bandLevel(0, -24.5, 44100), bandLevel(5, -24.5, 44100), bandLevel(bandCount-1, -24.5, 44100)
	if !(bass < mid && mid < treble) || math.Abs((bass+treble)/2-0.5) > 0.02 || treble-bass > 0.25 {
		t.Errorf("pink noise = %.2f %.2f %.2f, want a gentle tilt around mid-height", bass, mid, treble)
	}
	for _, tc := range []struct {
		band       int
		db         float64
		sampleRate int
		want       float64
	}{
		{5, -60, 44100, 0},
		{5, math.Inf(-1), 44100, 0},
		{5, math.NaN(), 44100, 0},
		{bandCount - 1, 0, 44100, 1},
		{bandCount - 1, -10, 24000, 0},
		{bandCount - 1, -10, 22050, 0},
		{bandCount - 2, 0, 22050, 1},
		{bandCount - 1, 0, 0, 1},
	} {
		if got := bandLevel(tc.band, tc.db, tc.sampleRate); got != tc.want {
			t.Errorf("bandLevel(%d, %v, %d) = %v, want %v", tc.band, tc.db, tc.sampleRate, got, tc.want)
		}
	}
}

func TestMeterHandshake(t *testing.T) {
	InitLog(false)
	p := NewPlayer()
	defer p.disarmWatch()
	var out bytes.Buffer
	reply := func(candidate int, err string) {
		t.Helper()
		out.Reset()
		if !p.handleVU(&out, map[string]any{"request_id": float64(vuRequestID + candidate), "error": err}) {
			t.Fatal("reply not consumed")
		}
	}

	p.installVU(&out)
	if !strings.Contains(out.String(), meterCandidates[0].filter) || vuState(p.vu.Load()) != vuTrying {
		t.Fatalf("install: %s", out.String())
	}
	if _, _, ok := p.Spectrum(); !ok {
		t.Fatal("the spectrum should be tried first")
	}
	reply(0, "error running command")
	if !strings.Contains(out.String(), meterCandidates[1].filter) {
		t.Fatalf("level filter not tried: %s", out.String())
	}
	reply(1, "success")
	if _, _, ok := p.Spectrum(); ok || vuState(p.vu.Load()) != vuOn {
		t.Fatal("the level filter should be on, without a spectrum")
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

	// After a stream error, the filter that mpv took comes back.
	out.Reset()
	p.suspectVU(&out)
	if !strings.Contains(out.String(), `"remove"`) || vuState(p.vu.Load()) != vuPending {
		t.Fatalf("suspect: %s", out.String())
	}
	out.Reset()
	p.installVU(&out)
	if !strings.Contains(out.String(), meterCandidates[1].filter) {
		t.Fatalf("reinstall: %s", out.String())
	}
	reply(1, "error")
	reply(2, "error")
	if _, _, ok := p.Level(); ok || vuState(p.vu.Load()) != vuUnavailable {
		t.Fatal("meter should be unavailable")
	}

	q := NewPlayer()
	q.DisableVU()
	out.Reset()
	q.installVU(&out)
	if _, _, ok := q.Spectrum(); ok || out.Len() != 0 {
		t.Fatal("disabled meter installed a filter")
	}
}

func TestSpectrumWatchdog(t *testing.T) {
	InitLog(false)
	p := NewPlayer()
	defer p.disarmWatch()
	var out bytes.Buffer
	event := func(name string, data any) {
		t.Helper()
		if !p.handleVU(&out, map[string]any{"event": "property-change", "name": name, "data": data}) {
			t.Fatalf("%s not consumed", name)
		}
	}
	reading := map[string]any{"lavfi.astats.1.RMS_level": "-16.0", "lavfi.astats.2.RMS_level": "-8.0"}
	for band := range bandCount {
		reading[fmt.Sprintf("lavfi.astats.%d.RMS_level", band+3)] = "-24.5"
	}
	// check answers the watchdog's question, as if its timer had run out when expired.
	check := func(expired bool) {
		t.Helper()
		if expired && !p.watchSince.IsZero() {
			p.watchSince = p.watchSince.Add(-spectrumWatchdog)
		}
		out.Reset()
		if !p.handleVU(&out, map[string]any{"request_id": float64(spectrumCheckRequest), "error": "success", "data": false}) {
			t.Fatal("watchdog answer not consumed")
		}
	}

	event("core-idle", true)
	p.installVU(&out)
	p.handleVU(&out, map[string]any{"request_id": float64(vuRequestID), "error": "success"})
	if !p.watchSince.IsZero() {
		t.Fatal("watchdog armed while mpv is idle")
	}
	event("audio-params/samplerate", 22050.0)
	event("core-idle", false)
	if p.watchSince.IsZero() {
		t.Fatal("watchdog not armed while audio plays")
	}

	event("af-metadata/"+vuFilterLabel, map[string]any{})
	event("af-metadata/"+vuFilterLabel, nil)
	if _, at, ok := p.Spectrum(); !ok || !at.IsZero() {
		t.Fatalf("empty updates stored a reading: %v %v", at, ok)
	}
	event("af-metadata/"+vuFilterLabel, reading)
	bands, at, ok := p.Spectrum()
	if level, _, _ := p.Level(); !ok || time.Since(at) > time.Second || level != dbToLevel(-8) {
		t.Fatalf("spectrum = %v %v %v, level %v", bands, at, ok, level)
	}
	if bands[0] != bandLevel(0, -24.5, 22050) || bands[bandCount-2] == 0 || bands[bandCount-1] != 0 {
		t.Errorf("bands = %v; 12 kHz lies above half of 22.05 kHz and must read silent", bands)
	}

	check(false)
	if p.watchSince.IsZero() || out.Len() != 0 {
		t.Fatalf("watchdog fired early: %s", out.String())
	}
	check(true)
	if !p.watchSince.IsZero() || out.Len() != 0 || vuState(p.vu.Load()) != vuOn {
		t.Fatalf("watchdog fired despite readings: %s", out.String())
	}

	// The next stream stalls first, then plays without a reading.
	event("core-idle", true)
	event("core-idle", false)
	event("core-idle", true)
	check(true)
	if out.Len() != 0 {
		t.Fatalf("watchdog fired while mpv was idle: %s", out.String())
	}
	event("core-idle", false)
	p.bandsAt.Store(p.watchSince.Add(-spectrumWatchdog - time.Second).UnixNano())
	check(true)
	if !strings.Contains(out.String(), `"remove"`) || !strings.Contains(out.String(), meterCandidates[1].filter) {
		t.Fatalf("watchdog did not replace the spectrum: %s", out.String())
	}
	out.Reset()
	p.handleVU(&out, map[string]any{"request_id": float64(vuRequestID + 1), "error": "success"})
	if _, _, ok := p.Spectrum(); ok || vuState(p.vu.Load()) != vuOn {
		t.Fatal("the level filter should take over")
	}
}

type fakeMeter struct {
	level            float64
	bands            [bandCount]float64
	levelOK, bandsOK bool
	at               time.Time
}

func (f fakeMeter) Level() (float64, time.Time, bool)               { return f.level, f.at, f.levelOK }
func (f fakeMeter) Spectrum() ([bandCount]float64, time.Time, bool) { return f.bands, f.at, f.bandsOK }

func TestSpectrumCard(t *testing.T) {
	applyGlyphs(false)
	defer applyGlyphs(false)
	now := time.Now()
	n := newNowPlaying(nil)
	n.update(Info{Station: "Radio", Url: "http://radio", Song: "A", Volume: 50, Bitrate: 128}, now)
	src := fakeMeter{level: 0.5, levelOK: true, bandsOK: true, at: now,
		bands: [bandCount]float64{0, 0.02, 0.1, 0.25, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 0.95, 1}}
	n.advance(now, src)

	cells := n.meter()
	if got := segsText(cells); got != "▁▁▁▂▃▄▅▆▆▇██" {
		t.Errorf("spectrum = %q", got)
	}
	for cell, want := range map[int]tcell.Color{0: colorDim, 1: colorDim, 2: colorAccent, 7: colorAccent, 11: colorAccent} {
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
	src.bands, src.at = [bandCount]float64{}, later
	n.advance(later, src)
	if math.Abs(n.bands[bandCount-1]-(1-vuFallPerSec*0.1)) > 1e-9 || n.bands[0] != 0 {
		t.Errorf("bands after 100 ms = %v", n.bands)
	}

	// Without a spectrum the same slot shows the level meter.
	src.bandsOK = false
	n.advance(later.Add(100*time.Millisecond), src)
	cells = n.meter()
	lit, _, _ := cells[5].style.Decompose()
	unlit, _, _ := cells[6].style.Decompose()
	if len(cells) != vuCells || lit != colorAccent || unlit != colorDim {
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

// On the tags page of the wide layout the stations are visible, and a click
// on one must play it although the stations page has not been drawn yet.
func TestClickStationFromTagsPage(t *testing.T) {
	a, screen := newTestApp(t)
	resize(a, screen, 120, 40)
	waitFor(t, "wide layout", func() bool { return onUI(a, func() bool { return a.wide }) })
	screen.InjectKey(tcell.KeyDown, 0, tcell.ModNone)
	waitFor(t, "preview all stations", func() bool {
		return onUI(a, func() bool { return a.tag == allStationsTag && a.frontPage() == a.pageNames[Tags] })
	})

	pos := onUI(a, func() [2]int { x, y, _, _ := a.stationsList.GetInnerRect(); return [2]int{x + 5, y + 3} })
	screen.InjectMouse(pos[0], pos[1], tcell.Button1, tcell.ModNone)
	screen.InjectMouse(pos[0], pos[1], tcell.ButtonNone, tcell.ModNone)
	waitFor(t, "station clicked", func() bool {
		return onUI(a, func() bool {
			return a.frontPage() == a.pageNames[Main] && a.stationsList.GetCurrentItem() == 3
		})
	})
}

func segsText(segs []seg) string {
	var sb strings.Builder
	for _, s := range segs {
		sb.WriteString(s.text)
	}
	return sb.String()
}
