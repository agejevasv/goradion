package radio

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"

	"github.com/agejevasv/goradion/internal/logging"
)

func bookmarksFile() string {
	return filepath.Join(configDir(), "bookmarks.json")
}

type bookmark struct {
	URL   string `json:"url"`
	Title string `json:"title"`
}

// Bookmarks are the stations the user marked, in the order added. They may
// come from the stations list or from an online search. They belong to the
// UI goroutine.
type Bookmarks struct {
	path  string
	items []bookmark
	known map[string]Station // the stations list by URL
}

func NewBookmarks(stations []Station) *Bookmarks {
	b := &Bookmarks{path: bookmarksFile(), known: make(map[string]Station, len(stations))}
	for _, s := range stations {
		b.known[s.url] = s
	}

	data, err := os.ReadFile(b.path)
	switch {
	case os.IsNotExist(err):
	case err != nil:
		logging.Printf("bookmarks: %v", err)
	default:
		if err := json.Unmarshal(data, &b.items); err != nil {
			logging.Printf("bookmarks: %v", err)
		}
	}
	b.items = slices.DeleteFunc(b.items, func(bm bookmark) bool { return bm.URL == "" })
	return b
}

func (b *Bookmarks) has(url string) bool {
	return url != "" && slices.ContainsFunc(b.items, func(bm bookmark) bool { return bm.URL == url })
}

// toggle adds the station, or removes it when bookmarked, and saves the file.
// It reports whether the station is bookmarked now.
func (b *Bookmarks) toggle(s Station) bool {
	if s.url == "" {
		return false
	}
	n := len(b.items)
	b.items = slices.DeleteFunc(b.items, func(bm bookmark) bool { return bm.URL == s.url })
	added := len(b.items) == n
	if added {
		b.items = append(b.items, bookmark{URL: s.url, Title: s.title})
	}
	b.save()
	return added
}

func (b *Bookmarks) save() {
	data, err := json.MarshalIndent(b.items, "", "  ")
	if err == nil {
		err = writeFile(b.path, data)
	}
	if err != nil {
		logging.Printf("bookmarks: %v", err)
	}
}

// list returns the bookmarked stations. Those in the stations list show its
// title, which may have changed since they were added.
func (b *Bookmarks) list() []Station {
	stations := make([]Station, len(b.items))
	for i, bm := range b.items {
		if s, ok := b.known[bm.URL]; ok {
			stations[i] = s
			continue
		}
		stations[i] = Station{title: bm.Title, url: bm.URL}
	}
	return stations
}

func (b *Bookmarks) empty() bool {
	return len(b.items) == 0
}

// toggleBookmark bookmarks the station under the cursor, else the one
// playing, or removes its bookmark.
func (a *Application) toggleBookmark() {
	s, ok := a.bookmarkTarget()
	if !ok {
		return
	}
	a.bookmarks.toggle(s)
	a.reloadStations()
	if a.isSearchModalOpen() {
		a.rerenderSearchResults()
	}
}

func (a *Application) bookmarkTarget() (Station, bool) {
	switch a.frontPage() {
	case pageSearch:
		if i := a.searchResults.GetCurrentItem(); a.searchResults.HasFocus() && i < len(a.searchShown.results) {
			return a.searchShown.results[i].station, true
		}
	case pageMain:
		if i := a.stationsList.GetCurrentItem() - a.stationOffset(); i >= 0 && i < len(a.listed) {
			return a.listed[i], true
		}
	}
	if url := a.player.URL(); url != "" && url == a.playing.url {
		return a.playing, true
	}
	return Station{}, false
}
