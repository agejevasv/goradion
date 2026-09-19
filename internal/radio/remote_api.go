package radio

import (
	"errors"
	"fmt"
	"slices"
	"time"
)

// The remote's JSON API. The actions run on the UI goroutine, see
// Remote.action.

type remoteTag struct {
	Name   string `json:"name"`
	Kind   string `json:"kind,omitempty"`
	Online bool   `json:"online,omitempty"`
}

type remoteStation struct {
	Title      string `json:"title"`
	URL        string `json:"url"`
	Meta       string `json:"meta,omitempty"`
	Playing    bool   `json:"playing"`
	Bookmarked bool   `json:"bookmarked"`
}

type remotePlayer struct {
	Status     string `json:"status"`
	Station    string `json:"station"`
	Song       string `json:"song"`
	URL        string `json:"url"`
	Volume     int    `json:"volume"`
	Bitrate    int    `json:"bitrate"`
	Bookmarked bool   `json:"bookmarked"`
}

type remoteShuffle struct {
	Active    bool `json:"active"`
	Remaining int  `json:"remaining"`
	Interval  int  `json:"interval"`
}

type remoteSleep struct {
	Active    bool `json:"active"`
	Remaining int  `json:"remaining"`
}

type remoteState struct {
	Version  string          `json:"version"`
	Page     string          `json:"page"`
	Tag      string          `json:"tag"`
	Tags     []remoteTag     `json:"tags"`
	Stations []remoteStation `json:"stations"`
	Player   remotePlayer    `json:"player"`
	Shuffle  remoteShuffle   `json:"shuffle"`
	Sleep    remoteSleep     `json:"sleep"`
}

type actionRequest struct {
	Tag     string `json:"tag"`
	Search  bool   `json:"search"` // Tag is the last search
	URL     string `json:"url"`
	Volume  *int   `json:"volume"`
	Minutes int    `json:"minutes"`
	Query   string `json:"query"`
	Online  bool   `json:"online"`
}

// remoteState is what the TUI shows.
func (a *Application) remoteState() remoteState {
	inf := a.player.Snapshot()
	st := remoteState{
		Version:  VersionString(),
		Page:     "tags",
		Tag:      a.tag.name,
		Tags:     a.remoteTags(),
		Stations: []remoteStation{},
		Player: remotePlayer{
			Status:     inf.Status,
			Station:    inf.Station,
			Song:       inf.Song,
			URL:        inf.URL,
			Volume:     inf.Volume,
			Bitrate:    inf.Bitrate,
			Bookmarked: a.bookmarks.has(inf.URL),
		},
		Shuffle: remoteShuffle{Interval: int(a.shuffle.interval.Minutes())},
	}
	if a.sleep.active() {
		st.Sleep = remoteSleep{Active: true, Remaining: int(a.sleep.remaining(time.Now()).Seconds())}
	}
	if a.shuffle.active {
		st.Shuffle.Active = true
		st.Shuffle.Remaining = int(a.shuffle.remaining(time.Now()).Seconds())
	}
	if slices.Contains(a.pages.GetPageNames(true), string(pageMain)) {
		st.Page = "stations"
		for _, s := range a.listed {
			st.Stations = append(st.Stations, remoteStation{
				Title:      s.title,
				URL:        s.url,
				Playing:    s.url == inf.URL,
				Bookmarked: a.bookmarks.has(s.url),
			})
		}
	}
	return st
}

func (a *Application) remoteTags() []remoteTag {
	var out []remoteTag
	if !a.bookmarks.empty() {
		out = append(out, remoteTag{Name: bookmarksTag, Kind: "bookmarks"})
	}
	for _, t := range a.tags {
		out = append(out, remoteTag{Name: t})
	}
	if q := a.lastSearch.query; q != "" {
		out = append(out, remoteTag{Name: q, Kind: "search", Online: a.lastSearch.online})
	}
	return out
}

func (a *Application) remoteShowTags(actionRequest) error {
	a.tag = tagRef{}
	a.show(pageTags)
	return nil
}

func (a *Application) remoteOpenTag(q actionRequest) error {
	tag := tagRef{name: q.Tag, search: q.Search}
	if (tag == tagRef{name: bookmarksTag} && a.bookmarks.empty()) || tag == (tagRef{name: allStationsTag}) || !a.openTag(tag) {
		return fmt.Errorf("tag %q %w", q.Tag, errNotFound)
	}
	return nil
}

// remotePlay toggles a station of the list currently shown, like Enter does.
func (a *Application) remotePlay(q actionRequest) error {
	for i, s := range a.listed {
		if s.url == q.URL {
			a.selectListed(i)
			a.togglePlayManual(s)
			return nil
		}
	}
	return fmt.Errorf("station %w in the current list", errNotFound)
}

// remoteBookmark toggles the bookmark of a station of the list currently
// shown, or of the one playing when no URL is given.
func (a *Application) remoteBookmark(q actionRequest) error {
	station, ok := Station{}, false
	if i := slices.IndexFunc(a.listed, func(s Station) bool { return s.url == q.URL }); q.URL != "" && i >= 0 {
		station, ok = a.listed[i], true
	}
	if playing := a.playing; !ok && playing.url != "" && playing.url == a.player.URL() && (q.URL == "" || q.URL == playing.url) {
		station, ok = playing, true
	}
	if !ok {
		return fmt.Errorf("station %w", errNotFound)
	}
	a.bookmarks.toggle(station)
	a.reloadStations()
	a.syncBookmarksRow()
	return nil
}

func (a *Application) remoteStop(actionRequest) error {
	a.stop()
	return nil
}

func (a *Application) remoteRandom(actionRequest) error {
	a.stopShuffle()
	if _, ok := a.playRandom(); !ok {
		return errors.New("no stations to pick from")
	}
	return nil
}

func (a *Application) remoteVolume(q actionRequest) error {
	if q.Volume == nil {
		return errors.New("volume required")
	}
	a.player.SetVolume(*q.Volume)
	return nil
}

func (a *Application) remoteShuffle(q actionRequest) error {
	if q.Minutes == 0 {
		a.toggleShuffle()
		return nil
	}
	if q.Minutes < 1 || q.Minutes > 9 {
		return errors.New("minutes must be 1-9")
	}
	a.setShuffleInterval(q.Minutes)
	return nil
}

func (a *Application) remoteSleep(actionRequest) error {
	if !a.sleep.active() && a.player.URL() == "" {
		return errors.New("nothing playing")
	}
	a.cycleSleep()
	return nil
}
