package radio

import (
	"context"
	"sync"
	"time"
)

var sleepSteps = []time.Duration{15 * time.Minute, 30 * time.Minute, 45 * time.Minute, time.Hour, 90 * time.Minute}

const sleepFade = time.Minute

// sleepTimer's fields belong to the UI goroutine.
type sleepTimer struct {
	total  time.Duration // zero when off
	end    time.Time
	fade   time.Duration
	cancel context.CancelFunc
	loops  sync.WaitGroup
}

func (s *sleepTimer) active() bool {
	return s.total > 0
}

func (s *sleepTimer) remaining(now time.Time) time.Duration {
	return max(s.end.Sub(now), 0)
}

func nextSleep(current time.Duration) time.Duration {
	for _, d := range sleepSteps {
		if d > current {
			return d
		}
	}
	return 0
}

// cycleSleep steps through sleepSteps, then off. It does nothing while
// nothing plays.
func (a *Application) cycleSleep() {
	next := nextSleep(a.sleep.total)
	if !a.sleep.active() && a.player.URL() == "" {
		return
	}
	a.cancelSleep()
	a.card.flashSleep(time.Now())
	if next == 0 {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.sleep.total, a.sleep.end, a.sleep.cancel = next, time.Now().Add(next), cancel
	a.sleep.loops.Add(1)
	go func() {
		defer a.sleep.loops.Done()
		a.sleepLoop(ctx, next, a.sleep.fade)
	}()
}

func (a *Application) cancelSleep() {
	if a.sleep.cancel != nil {
		a.sleep.cancel()
	}
	a.sleep.total, a.sleep.end, a.sleep.cancel = 0, time.Time{}, nil
}

func (a *Application) sleepLoop(ctx context.Context, total, fade time.Duration) {
	timer := time.NewTimer(max(total-fade, 0))
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return
	case <-timer.C:
	}

	// A shuffle fade in progress restores its volume when stopped, so wait
	// for it before taking the volume to put back. Not loops.Wait: Ctrl+R
	// may start a new loop meanwhile.
	var shuffleDone chan struct{}
	if !a.queueUpdate(func() { shuffleDone = a.shuffle.done; a.stopShuffle() }) {
		return
	}
	if shuffleDone != nil {
		<-shuffleDone
	}
	volume := a.player.Volume()
	defer a.player.SetVolume(volume)

	a.player.Fade(ctx, 0, fade)
	a.queueUpdateDraw(func() {
		if ctx.Err() == nil {
			a.stop()
		}
	})
}
