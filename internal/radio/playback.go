package radio

import "math/rand"

// togglePlay plays the station, or stops it when it is the one playing.
func (a *Application) togglePlay(station Station) {
	starting := station.url != "" && station.url != a.player.URL()
	if starting {
		a.favorites.track(station)
	}
	a.player.Toggle(station.title, station.url)
	if starting && a.tag == favoritesTag {
		a.showStations(a.favorites.list())
	}
}

// togglePlayManual is togglePlay for a station the user picked, which ends
// the shuffle.
func (a *Application) togglePlayManual(station Station) {
	a.stopShuffle()
	a.togglePlay(station)
}

func (a *Application) stop() {
	a.stopShuffle()
	a.player.Stop()
}

// playRandom plays a random station of the list, avoiding the one playing
// when there is another.
func (a *Application) playRandom() (Station, bool) {
	if len(a.listed) == 0 {
		return Station{}, false
	}
	current := a.player.URL()
	i := rand.Intn(len(a.listed))
	for len(a.listed) > 1 && a.listed[i].url == current {
		i = rand.Intn(len(a.listed))
	}
	station := a.listed[i]
	a.selectListed(i)
	a.togglePlay(station)
	return station, true
}
