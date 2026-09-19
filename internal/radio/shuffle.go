package radio

import (
	"context"
	"sync"
	"time"
)

const (
	defaultShuffleInterval = 5 * time.Minute
	shuffleFade            = 2 * time.Second
	playbackTimeout        = 30 * time.Second
)

// shuffle plays a random station of the list every interval, fading between
// them. Its fields belong to the UI goroutine.
type shuffle struct {
	active   bool
	interval time.Duration
	start    time.Time // when the current interval began
	fade     time.Duration
	cancel   context.CancelFunc
	loops    sync.WaitGroup
	done     chan struct{} // closed when the latest loop returns
}

func (s *shuffle) remaining(now time.Time) time.Duration {
	return max(s.interval-now.Sub(s.start), 0)
}

func (a *Application) toggleShuffle() {
	if a.shuffle.active {
		a.stopShuffle()
		return
	}
	a.shuffle.active = true
	a.playRandom()
	a.restartShuffle()
}

func (a *Application) stopShuffle() {
	if !a.shuffle.active {
		return
	}
	a.shuffle.active = false
	a.shuffle.cancel()
}

func (a *Application) setShuffleInterval(minutes int) {
	a.shuffle.interval = time.Duration(minutes) * time.Minute
	if a.shuffle.active {
		a.restartShuffle()
	}
}

func (a *Application) restartShuffle() {
	if a.shuffle.cancel != nil {
		a.shuffle.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.shuffle.cancel = cancel
	a.shuffle.start = time.Now()
	done := make(chan struct{})
	a.shuffle.done = done
	a.shuffle.loops.Add(1)
	go func() {
		defer a.shuffle.loops.Done()
		defer close(done)
		a.shuffleLoop(ctx, a.shuffle.interval, a.shuffle.fade)
	}()
}

func (a *Application) shuffleLoop(ctx context.Context, interval, fade time.Duration) {
	timer := time.NewTimer(interval)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		if ctx.Err() != nil {
			return
		}
		picked := a.shuffleNext(ctx, fade, playbackTimeout)
		timer.Reset(interval - time.Since(picked))
	}
}

// shuffleNext fades out, plays a random station and fades back in once it
// plays, or after timeout. It returns when the station was picked. Cancelling
// ctx restores the volume at once.
func (a *Application) shuffleNext(ctx context.Context, fade, timeout time.Duration) time.Time {
	volume := a.player.Volume()
	defer func() {
		if ctx.Err() != nil {
			a.player.SetVolume(volume)
		}
	}()

	a.player.Fade(ctx, 0, fade)
	picked := time.Now()
	var url string
	a.queueUpdateDraw(func() {
		if ctx.Err() != nil {
			return
		}
		picked = time.Now()
		a.shuffle.start = picked
		if s, ok := a.playRandom(); ok {
			url = s.url
		}
	})
	if url != "" {
		a.player.WaitPlaying(ctx, url, timeout)
	}
	a.player.Fade(ctx, volume, fade)
	return picked
}
