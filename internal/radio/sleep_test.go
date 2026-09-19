package radio

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/agejevasv/goradion/internal/mpv"
)

func TestSleepCycle(t *testing.T) {
	a, _ := newTestApp(t)
	if onUI(a, func() bool { a.cycleSleep(); return a.sleep.active() }) {
		t.Fatal("timer set with nothing playing")
	}
	onUI(a, func() bool { a.openTag(tagRef{name: allStationsTag}); a.togglePlayManual(a.listed[0]); return true })

	var got []time.Duration
	for range len(sleepSteps) + 1 {
		got = append(got, onUI(a, func() time.Duration { a.cycleSleep(); return a.sleep.total }))
	}
	want := append(append([]time.Duration{}, sleepSteps...), 0)
	if !slices.Equal(got, want) {
		t.Fatalf("steps %v, want %v", got, want)
	}

	onUI(a, func() bool { a.cycleSleep(); return true })
	label := onUI(a, func() string { return segsText(a.card.sleepLabel(time.Now())) })
	if !strings.Contains(label, clock(sleepSteps[0])) {
		t.Fatalf("label %q", label)
	}
	if onUI(a, func() bool { a.togglePlayManual(a.playing); return a.sleep.active() }) {
		t.Fatal("stopping must turn the timer off")
	}
}

func TestSleepFadesAndStops(t *testing.T) {
	a, _ := newTestApp(t)
	p := a.player
	onUI(a, func() bool {
		a.openTag(tagRef{name: allStationsTag})
		a.togglePlayManual(a.listed[0])
		a.toggleShuffle()
		return true
	})
	a.sleepLoop(context.Background(), 20*time.Millisecond, 10*time.Millisecond)
	if url := p.URL(); url != "" {
		t.Fatalf("still playing %q", url)
	}
	if v := p.Volume(); v != mpv.DefaultVolume {
		t.Fatalf("volume %d, want %d", v, mpv.DefaultVolume)
	}
	if onUI(a, func() bool { return a.shuffle.active }) {
		t.Fatal("shuffle still on")
	}
}

func TestSleepCancelRestoresVolume(t *testing.T) {
	a, _ := newTestApp(t)
	p := a.player
	onUI(a, func() bool { a.openTag(tagRef{name: allStationsTag}); a.togglePlayManual(a.listed[0]); return true })
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { a.sleepLoop(ctx, time.Second, time.Second); close(done) }()
	waitFor(t, "fade", func() bool { return p.Volume() < mpv.DefaultVolume })
	cancel()
	<-done
	if v := p.Volume(); v != mpv.DefaultVolume || p.URL() == "" {
		t.Fatalf("volume %d, url %q", v, p.URL())
	}
}

func TestSleepFromPhone(t *testing.T) {
	a, _ := newTestApp(t)
	if err := onUI(a, func() error { return a.remoteSleep(actionRequest{}) }); err == nil {
		t.Fatal("timer set with nothing playing")
	}
	st := onUI(a, func() remoteSleep {
		a.openTag(tagRef{name: allStationsTag})
		a.togglePlayManual(a.listed[0])
		a.remoteSleep(actionRequest{})
		return a.remoteState().Sleep
	})
	if !st.Active || st.Remaining < 14*60 || st.Remaining > 15*60 {
		t.Fatalf("sleep = %+v", st)
	}
}
