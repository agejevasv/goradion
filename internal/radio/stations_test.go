package radio

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestParseStations(t *testing.T) {
	csv := "  Radio One , http://one ,Jazz; Rock ;;\n" +
		"No URL\n" +
		"Blank URL, ,Jazz\n" +
		`"Two, Quoted",http://two` + "\n"
	stations, err := parseStations(strings.NewReader(csv))
	if err != nil {
		t.Fatal(err)
	}
	if len(stations) != 2 {
		t.Fatalf("stations = %+v", stations)
	}
	if s := stations[0]; s.title != "Radio One" || s.url != "http://one" || !slices.Equal(s.tags, []string{"Jazz", "Rock"}) {
		t.Errorf("first = %+v", s)
	}
	if s := stations[1]; s.title != "Two, Quoted" || s.url != "http://two" || s.tags != nil {
		t.Errorf("second = %+v", s)
	}

	if _, err := parseStations(strings.NewReader(`"unterminated,http://x`)); err == nil {
		t.Error("broken CSV accepted")
	}
}

func TestLoadStations(t *testing.T) {
	builtIn, err := LoadStations("")
	if err != nil || len(builtIn) < 100 {
		t.Fatalf("built-in list: %d stations, %v", len(builtIn), err)
	}
	seen := map[string]bool{}
	for _, s := range builtIn {
		if s.title == "" || len(s.tags) == 0 || seen[s.url] {
			t.Errorf("built-in station %+v is untitled, untagged or repeated", s)
		}
		seen[s.url] = true
	}

	path := filepath.Join(t.TempDir(), "stations.csv")
	os.WriteFile(path, []byte("Mine,http://mine,Tag\n"), 0644)
	if stations, err := LoadStations(path); err != nil || len(stations) != 1 || stations[0].title != "Mine" {
		t.Errorf("file: %+v, %v", stations, err)
	}
	if _, err := LoadStations(filepath.Join(t.TempDir(), "missing.csv")); err == nil {
		t.Error("missing file accepted")
	}
}

func TestCollectTagsAndShortcuts(t *testing.T) {
	stations := []Station{{tags: []string{"Rock", "Jazz"}}, {tags: []string{"Jazz", "Ambient"}}, {}}
	if got := collectTags(stations); !slices.Equal(got, []string{"Ambient", "Jazz", "Rock"}) {
		t.Errorf("tags = %q", got)
	}
	for i, want := range map[int]rune{0: 'a', 25: 'z', 26: 'A', 51: 'Z', 52: '1', 60: '9', 61: 0} {
		if got := idxToRune(i); got != want {
			t.Errorf("idxToRune(%d) = %q, want %q", i, got, want)
		}
	}
}

func TestSearchStations(t *testing.T) {
	stations := []Station{
		{title: "SomaFM: Groove Salad", tags: []string{"Downtempo", "Electronic"}},
		{title: "SomaFM: Bossa Beyond", tags: []string{"Jazz", "Lounge"}},
		{title: "Linn Jazz", tags: []string{"Jazz"}},
	}
	titles := func(ss []Station) (out []string) {
		for _, s := range ss {
			out = append(out, s.title)
		}
		return out
	}
	for query, want := range map[string][]string{
		"soma":         {"SomaFM: Groove Salad", "SomaFM: Bossa Beyond"},
		"SOMA jazz":    {"SomaFM: Bossa Beyond"},
		"jazz":         {"SomaFM: Bossa Beyond", "Linn Jazz"},
		"electro soma": {"SomaFM: Groove Salad"},
		"polka":        nil,
		"  ":           nil,
	} {
		if got := titles(searchStations(stations, query)); !slices.Equal(got, want) {
			t.Errorf("search %q = %q, want %q", query, got, want)
		}
	}
}

func TestStationLabel(t *testing.T) {
	// Named colours such as gray/grey print either way; hex ones don't.
	defer useTheme(themes[0])
	nord, _ := findTheme("nord")
	useTheme(nord)
	if got := stationLabel(Station{title: "Radio [live]"}, false); got != "Radio [live[]" {
		t.Errorf("label = %q, want tview tags escaped", got)
	}
	if got := stationLabel(Station{title: "Radio"}, true); got != "Radio [#88C0D0]"+glyphs.star+"[-]" {
		t.Errorf("bookmarked label = %q", got)
	}
}
