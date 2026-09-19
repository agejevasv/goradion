package radio

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/agejevasv/goradion/internal/mpv"
	"github.com/gdamore/tcell/v2"
)

func TestScreenLayouts(t *testing.T) {
	a, screen := newTestApp(t)
	resize(a, screen, 120, 40)
	waitFor(t, "wide layout", func() bool {
		txt := screenText(a, screen)
		return strings.Contains(txt, "Tags") && strings.Contains(txt, "Now playing") && strings.Contains(txt, "? help")
	})
	if !onUI(a, func() bool { return a.wide }) {
		t.Fatal("120 columns should be wide")
	}

	stations := onUI(a, func() []Station {
		a.openTag(tagRef{name: "Jazz"})
		return a.listed
	})
	now := time.Now()
	a.app.QueueUpdateDraw(func() {
		a.card.update(mpv.Info{State: mpv.Playing, Station: stations[1].title, URL: stations[1].url, Song: "Lena Hart Trio - Autumn Leaves",
			Volume: 80, Bitrate: 192, History: []mpv.Track{
				{Time: now, Station: "Some Station", Song: "Moss & Ember - Kettle Song"},
				{Time: now, Station: "Some Station", Song: "Juniper Fields - Warm Static"},
				{Time: now, Station: stations[1].title, Song: "Lena Hart Trio - Autumn Leaves"},
			}}, now)
	})
	waitFor(t, "card and marks", func() bool {
		txt := screenText(a, screen)
		return strings.Contains(txt, "Autumn Leaves") && strings.Contains(txt, "192 kb/s") &&
			strings.Contains(txt, "(b) "+glyphs.play+" "+stations[1].title) &&
			strings.Contains(txt, "earlier  Juniper Fields - Warm Static")
	})

	screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)
	waitFor(t, "tab to tags", func() bool { return onUI(a, a.frontPage) == pageTags })
	if onUI(a, func() string { return a.tag.name }) != "Jazz" {
		t.Fatal("tab must keep the tag")
	}
	screen.InjectKey(tcell.KeyDown, 0, tcell.ModNone)
	waitFor(t, "preview next tag", func() bool { return onUI(a, func() string { return a.tag.name }) != "Jazz" })
	screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)
	waitFor(t, "tab to stations", func() bool { return onUI(a, a.frontPage) == pageMain })

	resize(a, screen, 80, 24)
	waitFor(t, "narrow layout", func() bool {
		txt := screenText(a, screen)
		return !strings.Contains(txt, "Favorites") && strings.Contains(txt, glyphs.back+" ") && strings.Contains(txt, "earlier")
	})
	if onUI(a, a.stationOffset) != 2 {
		t.Fatal("narrow station list should start with a back link")
	}
	screen.InjectKey(tcell.KeyEscape, 0, tcell.ModNone)
	waitFor(t, "esc to tags", func() bool {
		return onUI(a, a.frontPage) == pageTags && strings.Contains(screenText(a, screen), " Tags ")
	})
}

// On the tags page of the wide layout the stations are visible, and a click
// on one must play it although the stations page has not been drawn yet.
func TestClickStationFromTagsPage(t *testing.T) {
	a, screen := newTestApp(t)
	resize(a, screen, 120, 40)
	waitFor(t, "wide layout", func() bool { return onUI(a, func() bool { return a.wide }) })
	screen.InjectKey(tcell.KeyDown, 0, tcell.ModNone)
	waitFor(t, "preview the first tag", func() bool {
		return onUI(a, func() bool { return a.tag.name == a.tags[0] && a.frontPage() == pageTags })
	})

	pos := onUI(a, func() [2]int { x, y, _, _ := a.stationsList.GetInnerRect(); return [2]int{x + 5, y + 3} })
	screen.InjectMouse(pos[0], pos[1], tcell.Button1, tcell.ModNone)
	screen.InjectMouse(pos[0], pos[1], tcell.ButtonNone, tcell.ModNone)
	waitFor(t, "station clicked", func() bool {
		return onUI(a, func() bool {
			return a.frontPage() == pageMain && a.stationsList.GetCurrentItem() == 3
		})
	})
}

// A list draws with its cursor in view, which used to stop the wheel once the
// cursor reached the edge.
func TestWheelScrollsPastCursor(t *testing.T) {
	a, screen := newTestApp(t)
	resize(a, screen, 120, 40)
	a.app.QueueUpdateDraw(func() { a.openTag(tagRef{name: allStationsTag}) })
	pos := onUI(a, func() [2]int { x, y, _, _ := a.stationsList.GetInnerRect(); return [2]int{x + 5, y + 3} })
	offset := func() int { return onUI(a, func() int { o, _ := a.stationsList.GetOffset(); return o }) }
	scroll := func(wheel tcell.ButtonMask, want int) {
		for i := 0; offset() != want; i++ {
			if i == 200 {
				t.Fatalf("offset %d, want %d", offset(), want)
			}
			screen.InjectMouse(pos[0], pos[1], wheel, tcell.ModNone)
			screen.InjectMouse(pos[0], pos[1], tcell.ButtonNone, tcell.ModNone)
			time.Sleep(5 * time.Millisecond)
		}
	}
	scroll(tcell.WheelDown, 60)
	scroll(tcell.WheelUp, 0)
}

// A search named like a tag gets a row of its own, and each row opens its own
// stations.
func TestSearchNamedLikeTag(t *testing.T) {
	a, _ := newTestApp(t)
	found := []Station{{title: "Found", url: "http://found"}}
	onUI(a, func() bool { a.openSearch("Jazz", found, true); return true })

	rows := onUI(a, func() []tagRef { return slices.Clone(a.tagRows) })
	if !slices.Contains(rows, tagRef{name: "Jazz"}) || !slices.Contains(rows, tagRef{name: "Jazz", search: true}) {
		t.Fatalf("tag rows %v", rows)
	}

	listed := func(tag tagRef) []Station {
		return onUI(a, func() []Station { a.openTag(tag); return a.listed })
	}
	if got := listed(tagRef{name: "Jazz"}); len(got) < 2 || slices.ContainsFunc(got, func(s Station) bool { return s.url == "http://found" }) {
		t.Fatalf("the Jazz tag lists %v", got)
	}
	if got := listed(tagRef{name: "Jazz", search: true}); len(got) != 1 || got[0].url != "http://found" {
		t.Fatalf("the Jazz search lists %v", got)
	}
	if got := onUI(a, a.stationsList.GetTitle); !strings.Contains(got, "Jazz (online)") {
		t.Fatalf("title %q", got)
	}
}

// Once the TUI has exited, queued updates report so instead of blocking.
func TestQueueUpdateAfterExit(t *testing.T) {
	a, _ := newTestApp(t)
	if !a.queueUpdate(func() {}) {
		t.Fatal("update did not run")
	}
	stopTestApp(a)
	done := make(chan bool)
	go func() { done <- a.queueUpdateDraw(func() {}) }()
	select {
	case ran := <-done:
		if ran {
			t.Fatal("update ran after exit")
		}
	case <-time.After(time.Second):
		t.Fatal("update blocked after exit")
	}
}
