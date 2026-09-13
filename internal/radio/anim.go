package radio

import "time"

const frameInterval = 50 * time.Millisecond

func (a *Application) animate(stop <-chan struct{}) {
	ticker := time.NewTicker(frameInterval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
		}
		a.app.QueueUpdate(func() {
			now := time.Now()
			a.card.advance(now, a.player)
			if a.card.animating(now) && a.card.changed(now) {
				a.app.ForceDraw()
			}
		})
	}
}
