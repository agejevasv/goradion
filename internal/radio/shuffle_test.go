package radio

import (
	"context"
	"testing"
	"time"

	"github.com/agejevasv/goradion/internal/mpv"
)

func TestShuffle(t *testing.T) {
	a, _ := newTestApp(t)
	p := a.player

	first := onUI(a, func() string {
		a.openTag(allStationsTag)
		a.toggleShuffle()
		return p.URL()
	})
	if first == "" || !onUI(a, func() bool { return a.shuffle.active }) {
		t.Fatal("shuffle should start with a random station")
	}
	if row := onUI(a, func() string { return a.stationRows[a.stationsList.GetCurrentItem()] }); row != first {
		t.Fatalf("cursor on %q, playing %q", row, first)
	}

	// Without mpv the next station never plays, so it fades in after the
	// timeout.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	next := make(chan time.Time)
	go func() { next <- a.shuffleNext(ctx, 10*time.Millisecond, 300*time.Millisecond) }()
	waitFor(t, "next station", func() bool { return p.URL() != first })
	if v := p.Volume(); v != 0 {
		t.Fatalf("volume %d while tuning in, want 0", v)
	}
	picked := <-next
	if v := p.Volume(); v != mpv.DefaultVolume {
		t.Fatalf("volume %d after the fade in, want %d", v, mpv.DefaultVolume)
	}
	if start := onUI(a, func() time.Time { return a.shuffle.start }); !start.Equal(picked) {
		t.Fatalf("interval restarted at %v, picked at %v", start, picked)
	}

	// Picking a station ends the shuffle and restores the volume at once.
	second := p.URL()
	go func() { next <- a.shuffleNext(ctx, 10*time.Millisecond, time.Hour) }()
	waitFor(t, "third station", func() bool { return p.URL() != second })
	a.app.QueueUpdateDraw(func() { a.togglePlayManual(a.listed[0]) })
	<-next
	if v := p.Volume(); v != mpv.DefaultVolume {
		t.Fatalf("volume %d after cancelling, want %d", v, mpv.DefaultVolume)
	}
	if onUI(a, func() bool { return a.shuffle.active }) {
		t.Fatal("playing a station must end the shuffle")
	}
}

func TestShuffleInterval(t *testing.T) {
	a, _ := newTestApp(t)
	st := onUI(a, func() remoteShuffle {
		a.setShuffleInterval(3)
		a.toggleShuffle()
		return a.remoteState().Shuffle
	})
	if !st.Active || st.Interval != 3 || st.Remaining < 170 || st.Remaining > 180 {
		t.Fatalf("shuffle = %+v", st)
	}
	st = onUI(a, func() remoteShuffle {
		a.toggleShuffle()
		return a.remoteState().Shuffle
	})
	if st.Active {
		t.Fatalf("shuffle = %+v", st)
	}
}
