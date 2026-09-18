package radio

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/agejevasv/goradion/internal/logging"
)

// maxFavorites keeps every favourite on a letter shortcut.
const maxFavorites = int('z' - 'a' + 1)

func favoritesFile() string {
	return filepath.Join(configDir(), "favorites.json")
}

type FavoriteStation struct {
	URL        string    `json:"url"`
	Title      string    `json:"title"`
	PlayCount  int       `json:"play_count"`
	LastPlayed time.Time `json:"last_played"`
}

// Favorites are the stations played most, among the ones in the stations
// list. Its methods are safe for concurrent use.
type Favorites struct {
	mu       sync.Mutex
	path     string
	Stations map[string]*FavoriteStation `json:"stations"`
	known    map[string]Station          // the stations list by URL
}

func NewFavorites(stations []Station) *Favorites {
	f := &Favorites{
		path:     favoritesFile(),
		Stations: make(map[string]*FavoriteStation),
		known:    make(map[string]Station, len(stations)),
	}
	for _, s := range stations {
		f.known[s.url] = s
	}

	data, err := os.ReadFile(f.path)
	if err != nil {
		if !os.IsNotExist(err) {
			logging.Printf("favorites: %v", err)
		}
		return f
	}
	if err := json.Unmarshal(data, f); err != nil {
		logging.Printf("favorites: %v", err)
	}
	for url, fav := range f.Stations {
		if fav == nil {
			delete(f.Stations, url)
		}
	}
	return f
}

// track counts a play and saves the file.
func (f *Favorites) track(station Station) {
	if station.url == "" {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()

	fav := f.Stations[station.url]
	if fav == nil {
		fav = &FavoriteStation{URL: station.url}
		f.Stations[station.url] = fav
	}
	fav.Title = f.titleLocked(fav.URL, station.title)
	fav.PlayCount++
	fav.LastPlayed = time.Now()

	data, err := json.MarshalIndent(f, "", "  ")
	if err == nil {
		err = writeFile(f.path, data)
	}
	if err != nil {
		logging.Printf("favorites: %v", err)
	}
}

// titleLocked prefers the title in the stations list, which may have changed
// since the station was played.
func (f *Favorites) titleLocked(url, fallback string) string {
	if s, ok := f.known[url]; ok {
		return s.title
	}
	return fallback
}

// list returns the favourites most played first, then most recently played.
func (f *Favorites) list() []Station {
	f.mu.Lock()
	defer f.mu.Unlock()

	favs := make([]*FavoriteStation, 0, len(f.Stations))
	for _, fav := range f.Stations {
		if _, ok := f.known[fav.URL]; ok && fav.PlayCount > 0 {
			favs = append(favs, fav)
		}
	}
	slices.SortFunc(favs, func(a, b *FavoriteStation) int {
		if a.PlayCount != b.PlayCount {
			return b.PlayCount - a.PlayCount
		}
		return b.LastPlayed.Compare(a.LastPlayed)
	})

	stations := make([]Station, 0, min(len(favs), maxFavorites))
	for _, fav := range favs[:min(len(favs), maxFavorites)] {
		stations = append(stations, Station{
			title: f.titleLocked(fav.URL, fav.Title),
			url:   fav.URL,
			tags:  []string{favoritesTag},
			plays: fav.PlayCount,
		})
	}
	return stations
}

func (f *Favorites) empty() bool {
	return len(f.list()) == 0
}
