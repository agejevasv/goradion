package radio

import (
	"os"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, text string) {
	t.Helper()
	if err := writeFile(configFile(), []byte(text)); err != nil {
		t.Fatal(err)
	}
}

func TestSessionSaved(t *testing.T) {
	a, _ := newTestApp(t)
	jazz := onUI(a, func() Station {
		a.openTag(tagRef{name: "Jazz"})
		a.player.SetVolume(35)
		a.togglePlayManual(a.listed[1])
		return a.listed[1]
	})
	stopTestApp(a)

	c := loadConfig()
	if c.Volume != 35 || c.Tag != "Jazz" || c.Station != jazz.url || !c.Autoplay {
		t.Fatalf("saved %+v", c)
	}
}

func TestAutoplayKeptWhenOff(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeConfig(t, "autoplay: false # quiet\n")
	a, _ := newTestAppInHome(t, home)
	stopTestApp(a)
	data, _ := os.ReadFile(configFile())
	if !strings.Contains(string(data), "autoplay: false # quiet\n") {
		t.Fatalf("config:\n%s", data)
	}
}

func TestSessionFromSearchFallsBack(t *testing.T) {
	a, _ := newTestApp(t)
	s := onUI(a, func() Station {
		s := a.stations[0]
		a.openSearch("x", []Station{s}, false)
		a.togglePlayManual(s)
		return s
	})
	stopTestApp(a)
	if c := loadConfig(); c.Tag != s.tags[0] || c.Station != s.url {
		t.Fatalf("saved %+v", c)
	}
}

func TestSessionFromAllStationsUsesOwnTag(t *testing.T) {
	a, _ := newTestApp(t)
	s := onUI(a, func() Station {
		a.openTag(tagRef{name: allStationsTag})
		a.togglePlayManual(a.listed[2])
		return a.listed[2]
	})
	stopTestApp(a)
	if c := loadConfig(); c.Tag != s.tags[0] || c.Station != s.url {
		t.Fatalf("saved %+v", c)
	}
}

func TestSessionRestored(t *testing.T) {
	for _, autoplay := range []bool{false, true} {
		t.Run(map[bool]string{false: "cursor", true: "autoplay"}[autoplay], func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			stations, _ := LoadStations("")
			jazz := stationsWithTag(stations, "Jazz")[1]
			text := "theme: nord\nvolume: 40\ntag: Jazz\nstation: " + jazz.url + "\n"
			if !autoplay {
				text += "autoplay: false\n"
			}
			writeConfig(t, text)

			a, _ := newTestAppInHome(t, home)
			got := onUI(a, func() []any {
				row := a.stationsList.GetCurrentItem()
				return []any{a.tag.name, a.frontPage(), a.stationRows[row], a.player.Volume(), a.playing.url}
			})
			want := []any{"Jazz", pageMain, jazz.url, 40, ""}
			if autoplay {
				want[4] = jazz.url
			}
			for i := range want {
				if got[i] != want[i] {
					t.Fatalf("got %v, want %v", got, want)
				}
			}
		})
	}
}

func TestSessionGoneShowsDefault(t *testing.T) {
	for name, text := range map[string]string{
		"tag":     "tag: No Such Tag\nstation: https://example.com/\n",
		"all":     "tag: All Stations\nstation: https://streams.fluxfm.de/xjazz/mp3-320/audio/\n",
		"station": "tag: Jazz\nstation: https://example.com/gone\n",
		"other":   "tag: Jazz\nstation: https://streams.fluxfm.de/chillout/mp3-320/streams.fluxfm.de/play.pls\n",
	} {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			writeConfig(t, text)
			a, _ := newTestAppInHome(t, home)
			got := onUI(a, func() []any { return []any{a.frontPage(), a.playing.url} })
			if got[0] != pageTags || got[1] != "" {
				t.Fatalf("got %v", got)
			}
		})
	}
}

func stationsWithTag(stations []Station, tag string) []Station {
	var out []Station
	for _, s := range stations {
		for _, t := range s.tags {
			if t == tag {
				out = append(out, s)
			}
		}
	}
	return out
}

func TestSessionWithoutPlayingKeepsStation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeConfig(t, "tag: Gone\nstation: https://example.com/\n")
	a, _ := newTestAppInHome(t, home)
	stopTestApp(a)
	data, _ := os.ReadFile(configFile())
	if !strings.HasPrefix(string(data), "tag: Gone\nstation: https://example.com/\n") {
		t.Fatalf("config:\n%s", data)
	}
}
