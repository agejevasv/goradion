package radio

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"time"
)

func getFavoritesFile() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}

	var configDir string
	if runtime.GOOS == "windows" {
		configDir = filepath.Join(home, "AppData", "Roaming", "goradion")
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
}

func NewFavorites(stations []Station) *Favorites {
	availableStations := make(map[string]bool)
	for _, station := range stations {
		availableStations[station.url] = true
	}

	favorites := &Favorites{
		Stations:          make(map[string]*FavoriteStation),
		availableStations: availableStations,
	}

	data, err := os.ReadFile(getFavoritesFile())
	if err != nil {
		return favorites
	}

	json.Unmarshal(data, favorites)
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

	if f.Stations[station.url] == nil {
		f.Stations[station.url] = &FavoriteStation{
			URL:   station.url,
			Title: station.title,
		}
	}

	f.Stations[station.url].PlayCount++
	f.Stations[station.url].LastPlayed = time.Now()
	f.save()
}

func (f *Favorites) getFavoriteStations(minPlayCount int) []Station {
	if len(f.Stations) == 0 {
		return nil
	}

	var favStations []*FavoriteStation
	for _, fav := range f.Stations {
		if fav.PlayCount >= minPlayCount && f.availableStations[fav.URL] {
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
	maxFavorites := 10
	for i, fav := range favStations {
		if i >= maxFavorites {
			break
		}
		stations = append(stations, Station{
			title: fav.Title,
			url:   fav.URL,
			tags:  []string{favoritesTag},
		})
	}

	return stations
}

func (f *Favorites) hasFavorites() bool {
	for _, fav := range f.Stations {
		if fav.PlayCount >= 1 {
			return true
		}
	}
	return false
}
