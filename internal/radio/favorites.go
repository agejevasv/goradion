package radio

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"time"
)

const minPlays = 1
const maxFavs = int('z' - 'a' + 1)

func getFavoritesFile() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}

	var configDir string
	if runtime.GOOS == "windows" {
		configDir = filepath.Join(home, "AppData", "Roaming", "goradion")
	} else if runtime.GOOS == "darwin" {
		configDir = filepath.Join(home, "Library", "Application Support", "goradion")
	} else {
		configDir = filepath.Join(home, ".config", "goradion")
	}

	return filepath.Join(configDir, "favorites.json")
}

type FavoriteStation struct {
	URL        string    `json:"url"`
	Title      string    `json:"title"`
	PlayCount  int       `json:"play_count"`
	LastPlayed time.Time `json:"last_played"`
}

type Favorites struct {
	Stations          map[string]*FavoriteStation `json:"stations"`
	availableStations map[string]bool
	stationsByURL     map[string]Station
}

func NewFavorites(stations []Station) *Favorites {
	availableStations := make(map[string]bool)
	stationsByURL := make(map[string]Station)
	for _, station := range stations {
		availableStations[station.url] = true
		stationsByURL[station.url] = station
	}

	favorites := &Favorites{
		Stations:          make(map[string]*FavoriteStation),
		availableStations: availableStations,
		stationsByURL:     stationsByURL,
	}

	data, err := os.ReadFile(getFavoritesFile())
	if err != nil {
		return favorites
	}

	if err := json.Unmarshal(data, favorites); err != nil {
		log.Printf("Failed to unmarshal favorites: %v", err)
	}
	return favorites
}

func (f *Favorites) save() error {
	favFile := getFavoritesFile()
	os.MkdirAll(filepath.Dir(favFile), 0755)

	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(favFile, data, 0644)
}

func (f *Favorites) track(station Station) {
	if station.url == "" {
		return
	}

	title := station.title
	if currentStation, ok := f.stationsByURL[station.url]; ok {
		title = currentStation.title
	}

	if f.Stations[station.url] == nil {
		f.Stations[station.url] = &FavoriteStation{
			URL:   station.url,
			Title: title,
		}
	}

	f.Stations[station.url].Title = title
	f.Stations[station.url].PlayCount++
	f.Stations[station.url].LastPlayed = time.Now()
	if err := f.save(); err != nil {
		log.Printf("Failed to save favorites: %v", err)
	}
}

func (f *Favorites) getFavoriteStations() []Station {
	if len(f.Stations) == 0 {
		return nil
	}

	var favStations []*FavoriteStation
	for _, fav := range f.Stations {
		if fav.PlayCount >= minPlays && f.availableStations[fav.URL] {
			favStations = append(favStations, fav)
		}
	}

	if len(favStations) == 0 {
		return nil
	}

	sort.Slice(favStations, func(i, j int) bool {
		if favStations[i].PlayCount == favStations[j].PlayCount {
			return favStations[i].LastPlayed.After(favStations[j].LastPlayed)
		}
		return favStations[i].PlayCount > favStations[j].PlayCount
	})

	var stations []Station

	for i, fav := range favStations {
		if i >= maxFavs {
			break
		}

		title := fav.Title
		if currentStation, ok := f.stationsByURL[fav.URL]; ok {
			title = currentStation.title
		}

		stations = append(stations, Station{
			title: fmt.Sprintf("%s [gray](%d)[-]", title, fav.PlayCount),
			url:   fav.URL,
			tags:  []string{favoritesTag},
		})
	}

	return stations
}

func (f *Favorites) hasFavorites() bool {
	for _, fav := range f.Stations {
		if fav.PlayCount >= minPlays {
			return true
		}
	}
	return false
}
