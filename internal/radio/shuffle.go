package radio

import (
	"context"
	"math/rand"
	"time"
)

func (a *Application) toggleTimedRandom() {
	if a.timedRandomActive {
		if a.timedRandomCancel != nil {
			a.timedRandomCancel()
		}

		a.player.Lock()
		if a.player.fadeCancel != nil {
			a.player.fadeCancel()
		}
		a.player.Unlock()

		a.timedRandomActive = false
		a.publishShuffle()
		return
	}

	a.timedRandomActive = true
	a.shuffleIterationStartAt = time.Now()
	ctx, cancel := context.WithCancel(context.Background())
	a.timedRandomCancel = cancel

	a.publishShuffle()

	stations := a.getStationsFromCurrentView()
	if len(stations) > 0 {
		r := rand.Intn(len(stations))
		for len(stations) > 1 && a.player.info.Url == stations[r].url {
			r = rand.Intn(len(stations))
		}
		offset := a.calculateStationListOffset()
		a.stationsList.SetCurrentItem(r + offset)
		go a.togglePlay(stations[r])
	}

	go a.timedRandomLoop(ctx)
}

func (a *Application) setShuffleInterval(minutes int) {
	a.shuffleInterval = time.Duration(minutes) * time.Minute
	if !a.timedRandomActive {
		a.publishShuffle()
		return
	}

	if a.timedRandomCancel != nil {
		a.timedRandomCancel()
	}
	a.player.Lock()
	if a.player.fadeCancel != nil {
		a.player.fadeCancel()
	}
	a.player.Unlock()

	a.shuffleIterationStartAt = time.Now()
	ctx, cancel := context.WithCancel(context.Background())
	a.timedRandomCancel = cancel

	a.publishShuffle()

	go a.timedRandomLoop(ctx)
}

// publishShuffle must be called by the goroutine that changed the shuffle
// state, never by the UI goroutine.
func (a *Application) publishShuffle() {
	active, start, interval := a.timedRandomActive, a.shuffleIterationStartAt, a.shuffleInterval
	a.app.QueueUpdateDraw(func() {
		a.card.shuffleActive, a.card.shuffleStart, a.card.shuffleInterval = active, start, interval
	})
}

func (a *Application) timedRandomLoop(ctx context.Context) {
	ticker := time.NewTicker(a.shuffleInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			fadeCtx, fadeCancel := context.WithCancel(ctx)

			a.player.Lock()
			a.player.fadeCancel = fadeCancel
			savedVol := a.player.info.Volume
			a.player.Unlock()

			a.player.FadeOut(fadeCtx, fadeDuration)

			if fadeCtx.Err() != nil {
				a.player.SetVolume(savedVol)
				fadeCancel()
				return
			}

			stations := a.getStationsFromCurrentView()
			if len(stations) > 0 {
				r := rand.Intn(len(stations))
				for len(stations) > 1 && a.player.info.Url == stations[r].url {
					r = rand.Intn(len(stations))
				}
				offset := a.calculateStationListOffset()
				a.stationsList.SetCurrentItem(r + offset)

				a.waitingForPlayback = make(chan struct{})
				a.waitingForURL = stations[r].url
				go a.togglePlay(stations[r])
				a.shuffleIterationStartAt = time.Now()
				a.publishShuffle()

				select {
				case <-a.waitingForPlayback:
				case <-fadeCtx.Done():
					a.waitingForPlayback = nil
					a.waitingForURL = ""
					a.player.SetVolume(savedVol)
					fadeCancel()
					return
				case <-time.After(30 * time.Second):
				}

				a.waitingForPlayback = nil
				a.waitingForURL = ""
			}

			if fadeCtx.Err() != nil {
				a.player.SetVolume(savedVol)
				fadeCancel()
				return
			}

			a.player.FadeIn(fadeCtx, fadeDuration)

			if fadeCtx.Err() != nil {
				a.player.SetVolume(savedVol)
				fadeCancel()
				return
			}

			fadeCancel()
			a.player.Lock()
			a.player.fadeCancel = nil
			a.player.Unlock()
		}
	}
}
