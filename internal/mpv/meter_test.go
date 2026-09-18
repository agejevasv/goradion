package mpv

import (
	"bytes"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"
)

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
	for band := range BandCount {
		meta[fmt.Sprintf("lavfi.astats.%d.RMS_level", band+3)] = fmt.Sprint(-20 - band)
	}
	if r := readMeter(meta); !r.hasLevel || r.level != -9.25 || !r.hasBands || r.bands[0] != -20 || r.bands[BandCount-1] != -31 {
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
		if want := math.Round(40 * math.Pow(300, float64(i)/(BandCount-1))); float64(f) != want {
			t.Errorf("band %d is %d Hz, want %v", i, f, want)
		}
	}
}

func TestBandLevel(t *testing.T) {
	// Pink noise at -17 dBFS puts every band near -24.5 dBFS.
	bass, mid, treble := bandLevel(0, -24.5, 44100), bandLevel(5, -24.5, 44100), bandLevel(BandCount-1, -24.5, 44100)
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
		{BandCount - 1, 0, 44100, 1},
		{BandCount - 1, -10, 24000, 0},
		{BandCount - 1, -10, 22050, 0},
		{BandCount - 2, 0, 22050, 1},
		{BandCount - 1, 0, 0, 1},
	} {
		if got := bandLevel(tc.band, tc.db, tc.sampleRate); got != tc.want {
			t.Errorf("bandLevel(%d, %v, %d) = %v, want %v", tc.band, tc.db, tc.sampleRate, got, tc.want)
		}
	}
}

func TestMeterHandshake(t *testing.T) {
	p := New()
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

	q := New()
	q.DisableVU()
	out.Reset()
	q.installVU(&out)
	if _, _, ok := q.Spectrum(); ok || out.Len() != 0 {
		t.Fatal("disabled meter installed a filter")
	}
}

func TestSpectrumWatchdog(t *testing.T) {
	p := New()
	defer p.disarmWatch()
	var out bytes.Buffer
	event := func(name string, data any) {
		t.Helper()
		if !p.handleVU(&out, map[string]any{"event": "property-change", "name": name, "data": data}) {
			t.Fatalf("%s not consumed", name)
		}
	}
	reading := map[string]any{"lavfi.astats.1.RMS_level": "-16.0", "lavfi.astats.2.RMS_level": "-8.0"}
	for band := range BandCount {
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
	if bands[0] != bandLevel(0, -24.5, 22050) || bands[BandCount-2] == 0 || bands[BandCount-1] != 0 {
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
