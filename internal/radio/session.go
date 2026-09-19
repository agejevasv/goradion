package radio

import (
	"os"
	"os/signal"
	"slices"
	"syscall"

	"github.com/agejevasv/goradion/internal/logging"
)

// restoreSession leaves the default view when the tag or station is gone,
// e.g. after an update of the built-in stations.
func (a *Application) restoreSession() {
	c := a.config
	a.player.SetVolume(c.Volume)
	tag := tagRef{name: c.Tag}
	station, ok := a.findStation(tag, c.Station)
	if !ok || tag.name == allStationsTag {
		return
	}
	a.openTag(tag)
	a.selectStation(station.url)
	if c.Autoplay {
		a.togglePlay(station)
	}
}

// saveSession falls back to the station's own tag or Bookmarks for a station
// started from a search or All Stations, which have no row in the tags list.
func (a *Application) saveSession() {
	a.sleep.loops.Wait()
	a.shuffle.loops.Wait()
	c := a.config
	c.Volume = a.player.Volume()
	keys := []string{"volume"}
	if url := a.playing.url; url != "" {
		keys = append(keys, "tag", "station")
		c.Tag, c.Station = "", ""
		candidates := []tagRef{a.playingTag}
		if s, ok := a.findStation(tagRef{name: allStationsTag}, url); ok && len(s.tags) > 0 {
			candidates = append(candidates, tagRef{name: s.tags[0]})
		}
		candidates = append(candidates, tagRef{name: bookmarksTag})
		for _, tag := range candidates {
			if tag.name == allStationsTag {
				continue
			}
			if _, ok := a.findStation(tag, url); ok {
				c.Tag, c.Station = tag.name, url
				break
			}
		}
	}
	if err := c.save(keys...); err != nil {
		logging.Printf("config: %v", err)
	}
}

func (a *Application) findStation(tag tagRef, url string) (Station, bool) {
	if url == "" || tag.name == "" || tag.search || !a.tagExists(tag) {
		return Station{}, false
	}
	stations := a.stationsForTag(tag)
	i := slices.IndexFunc(stations, func(s Station) bool { return s.url == url })
	if i < 0 {
		return Station{}, false
	}
	return stations[i], true
}

// stopOnSignal saves the session when the terminal window is closed.
func (a *Application) stopOnSignal() {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGHUP, syscall.SIGTERM)
	defer signal.Stop(signals)
	select {
	case <-signals:
		a.app.Stop()
	case <-a.stopped:
	}
}
