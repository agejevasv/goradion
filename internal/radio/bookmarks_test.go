package radio

import (
	"slices"
	"testing"

	"github.com/gdamore/tcell/v2"
)

func TestBookmarks(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	stations := []Station{{title: "One", url: "http://1"}, {title: "Two", url: "http://2"}}
	b := NewBookmarks(stations)
	if !b.empty() {
		t.Fatal("new bookmarks should be empty")
	}

	online := Station{title: "Online", url: "http://online"}
	if !b.toggle(stations[1]) || !b.toggle(online) || !b.toggle(stations[0]) {
		t.Fatal("toggle should add")
	}
	if b.toggle(Station{title: "No URL"}) {
		t.Fatal("a station without a URL can't be bookmarked")
	}
	if list := b.list(); len(list) != 3 || list[0].url != "http://2" || list[1].url != online.url || list[2].url != "http://1" {
		t.Fatalf("list = %+v, want the order added", list)
	}
	if b.toggle(online) || b.has(online.url) || len(b.list()) != 2 {
		t.Fatal("toggle should remove")
	}
	b.toggle(online)

	// The file survives a restart; the stations list's titles win.
	stations[1].title = "Renamed"
	list := NewBookmarks(stations).list()
	if len(list) != 3 || list[0].title != "Renamed" || list[2].title != "Online" {
		t.Fatalf("reloaded list = %+v", list)
	}
}

func hasBookmarksRow(a *Application) bool {
	return onUI(a, func() bool { return slices.Contains(a.tagRows, tagRef{name: bookmarksTag}) })
}

func TestBookmarksRowHiddenWhenEmpty(t *testing.T) {
	a, screen := newTestApp(t)
	resize(a, screen, 80, 30)
	if hasBookmarksRow(a) || onUI(a, func() tagRef { return a.tagRows[a.tagsList.GetCurrentItem()] }) != (tagRef{name: a.tags[0]}) {
		t.Fatal("empty bookmarks shown, or cursor not on the first tag")
	}

	jazz := tagRef{name: "Jazz"}
	onUI(a, func() bool { a.openTag(jazz); a.selectListed(0); a.toggleBookmark(); return true })
	if !hasBookmarksRow(a) {
		t.Fatal("row missing after the first bookmark")
	}

	// Removing the last bookmark from its own list keeps the row until leaving.
	onUI(a, func() bool { a.openTag(tagRef{name: bookmarksTag}); a.selectListed(0); a.toggleBookmark(); return true })
	if !hasBookmarksRow(a) {
		t.Fatal("row vanished while shown")
	}
	screen.InjectKey(tcell.KeyEscape, 0, tcell.ModNone)
	waitFor(t, "row hidden", func() bool { return !hasBookmarksRow(a) })
}

func TestBookmarksRowWideLeavesOnMove(t *testing.T) {
	a, screen := newTestApp(t)
	resize(a, screen, 120, 30)
	onUI(a, func() bool {
		a.openTag(tagRef{name: "Jazz"})
		a.selectListed(0)
		a.toggleBookmark()
		a.show(pageTags)
		a.tagsList.SetCurrentItem(0)
		a.show(pageMain)
		a.selectListed(0)
		a.toggleBookmark()
		a.show(pageTags)
		return true
	})
	if !hasBookmarksRow(a) || onUI(a, func() string { return a.tag.name }) != bookmarksTag {
		t.Fatal("previewed empty bookmarks lost their row")
	}
	screen.InjectKey(tcell.KeyDown, 0, tcell.ModNone)
	waitFor(t, "row hidden", func() bool { return !hasBookmarksRow(a) })
	got := onUI(a, func() []tagRef { return []tagRef{a.tag, a.tagRows[a.tagsList.GetCurrentItem()]} })
	if got[0] != got[1] || got[0].name == bookmarksTag {
		t.Fatalf("tag %v, cursor on %v", got[0], got[1])
	}
}

func TestBookmarksRowFromPhone(t *testing.T) {
	a, _ := newTestApp(t)
	onUI(a, func() error {
		a.openTag(tagRef{name: "Jazz"})
		return a.remoteBookmark(actionRequest{URL: a.listed[0].url})
	})
	if !hasBookmarksRow(a) {
		t.Fatal("row missing after a bookmark from the phone")
	}
}
