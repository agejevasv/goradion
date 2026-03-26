package radio

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/gdamore/tcell/v2"
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
		a.updateShuffleBorder()
		return
	}

	a.timedRandomActive = true
	a.shuffleIterationStartAt = time.Now()
	ctx, cancel := context.WithCancel(context.Background())
	a.timedRandomCancel = cancel

	a.updateShuffleBorder()

	go a.updateCountdown(ctx)

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
	if a.timedRandomActive {
		if a.timedRandomCancel != nil {
			a.timedRandomCancel()
		}
		a.player.Lock()
		if a.player.fadeCancel != nil {
			a.player.fadeCancel()
		}
		a.player.Unlock()

		a.timedRandomActive = true
		a.shuffleIterationStartAt = time.Now()
		ctx, cancel := context.WithCancel(context.Background())
		a.timedRandomCancel = cancel

		a.updateShuffleBorder()

		go a.updateCountdown(ctx)
		go a.timedRandomLoop(ctx)
	}
}

func (a *Application) updateShuffleBorder() {
	a.app.QueueUpdateDraw(func() {
		if a.timedRandomActive {
			elapsed := time.Since(a.shuffleIterationStartAt)
			remaining := max(a.shuffleInterval-elapsed, 0)
			minutes := int(remaining.Minutes())
			seconds := int(remaining.Seconds()) % 60
			countdown := fmt.Sprintf("%02d:%02d", minutes, seconds)

			title := fmt.Sprintf(" [red]🔀[-] Shuffle %s ", countdown)
			a.mainFlex.SetBorder(true).SetBorderColor(tcell.ColorWhite).SetBackgroundColor(tcell.ColorDefault).SetTitle(title)
			a.tagsFlex.SetBorder(true).SetBorderColor(tcell.ColorWhite).SetBackgroundColor(tcell.ColorDefault).SetTitle(title)
		} else {
			a.mainFlex.SetBorder(false).SetBackgroundColor(tcell.ColorBlack)
			a.tagsFlex.SetBorder(false).SetBackgroundColor(tcell.ColorBlack)
		}
	})
}

func (a *Application) updateCountdown(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.updateShuffleBorder()
		}
	}
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
