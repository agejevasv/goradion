package radio

import "testing"

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
