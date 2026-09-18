package radio

import (
	"fmt"
	"testing"
)

func TestFavorites(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var stations []Station
	for i := range maxFavorites + 5 {
		stations = append(stations, Station{title: fmt.Sprint("Radio ", i), url: fmt.Sprint("http://", i)})
	}
	f := NewFavorites(stations)
	if !f.empty() {
		t.Fatal("new favourites should be empty")
	}

	f.track(stations[1])
	f.track(stations[2])
	f.track(stations[2])
	f.track(Station{title: "Online", url: "http://online"})
	f.track(Station{title: "No URL"})
	list := f.list()
	if len(list) != 2 || list[0].url != stations[2].url || list[0].plays != 2 || list[1].url != stations[1].url {
		t.Fatalf("list = %+v", list)
	}
	if list[0].title != stations[2].title || list[0].tags[0] != favoritesTag {
		t.Fatalf("favourite = %+v", list[0])
	}

	// Ties go to the most recent play.
	f.track(stations[1])
	if list := f.list(); list[0].url != stations[1].url {
		t.Fatalf("most recent of equals should come first: %+v", list)
	}

	// A renamed station shows its new name; the file survives a restart.
	stations[1].title = "Renamed"
	f = NewFavorites(stations)
	if list := f.list(); len(list) != 2 || list[0].title != "Renamed" {
		t.Fatalf("reloaded list = %+v", list)
	}
	if list := NewFavorites(stations[3:]).list(); len(list) != 0 {
		t.Fatalf("stations missing from the list must not show: %+v", list)
	}

	for _, s := range stations {
		f.track(s)
	}
	if n := len(f.list()); n != maxFavorites {
		t.Fatalf("%d favourites, want at most %d", n, maxFavorites)
	}
}
