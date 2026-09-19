package radio

import (
	"fmt"
	"slices"

	"github.com/rivo/tview"
)

const (
	bookmarksTag   = "Bookmarks"
	allStationsTag = "All Stations"
)

func collectTags(stations []Station) []string {
	var tags []string
	for _, s := range stations {
		tags = append(tags, s.tags...)
	}
	slices.Sort(tags)
	return slices.Compact(tags)
}

// idxToRune is the list shortcut of the i-th item: a-z, A-Z, then 1-9.
func idxToRune(i int) rune {
	switch {
	case i < 26:
		return 'a' + rune(i)
	case i < 52:
		return 'A' + rune(i-26)
	case i < 61:
		return '1' + rune(i-52)
	}
	return 0
}

// showTags fills the tags list: bookmarks, the tags of the stations and the
// last search. All stations has no row; ~ opens it.
func (a *Application) showTags() {
	a.syncingTags = true
	defer func() { a.syncingTags = false }()

	list := a.tagsList
	cursor := list.GetCurrentItem()
	var cursorTag tagRef
	if cursor >= 0 && cursor < len(a.tagRows) {
		cursorTag = a.tagRows[cursor]
	}
	list.Clear()
	a.tagRows = a.tagRows[:0]
	add := func(label string, shortcut rune, tag tagRef) {
		list.AddItem(label, "", shortcut, func() { a.openTag(tag) })
		a.tagRows = append(a.tagRows, tag)
	}

	a.countTags()
	if a.wantsBookmarksRow() {
		add(bookmarksTag, '$', tagRef{name: bookmarksTag})
	}
	for i, tag := range a.tags {
		add(tview.Escape(tag), idxToRune(i), tagRef{name: tag})
	}
	if q := a.lastSearch.query; q != "" {
		label := tview.Escape(q)
		if a.lastSearch.online {
			label += " " + fgTag(colorDim) + "(online)[-]"
		}
		add(label, '^', tagRef{name: q, search: true})
	}
	if i := slices.Index(a.tagRows, cursorTag); i >= 0 {
		cursor = i
	}
	list.SetCurrentItem(cursor)
}

// wantsBookmarksRow keeps the row while its emptied list is shown, so that it
// doesn't vanish under the cursor.
func (a *Application) wantsBookmarksRow() bool {
	return !a.bookmarks.empty() || a.tag == tagRef{name: bookmarksTag}
}

func (a *Application) syncBookmarksRow() {
	shown := len(a.tagRows) > 0 && a.tagRows[0] == tagRef{name: bookmarksTag}
	if shown != a.wantsBookmarksRow() {
		a.refreshTags()
	}
}

func (a *Application) refreshTags() {
	a.showTags()
	if a.wide {
		a.syncTagCursor()
	}
}

// stationsForTag is what the stations list shows for a tag or search.
func (a *Application) stationsForTag(tag tagRef) []Station {
	switch {
	case tag.search:
		return a.lastSearch.stations
	case tag.name == "" || tag.name == allStationsTag:
		return a.stations
	case tag.name == bookmarksTag:
		return a.bookmarks.list()
	}
	var match []Station
	for _, s := range a.stations {
		if slices.Contains(s.tags, tag.name) {
			match = append(match, s)
		}
	}
	return match
}

// loadTag is openTag without switching pages.
func (a *Application) loadTag(tag tagRef) bool {
	if !a.tagExists(tag) {
		return false
	}
	a.tag = tag
	a.showStations(a.stationsForTag(tag))
	a.syncBookmarksRow()
	return true
}

func (a *Application) tagExists(tag tagRef) bool {
	if tag.search {
		return tag.name != "" && tag.name == a.lastSearch.query
	}
	return tag.name == bookmarksTag || tag.name == allStationsTag || slices.Contains(a.tags, tag.name)
}

func (a *Application) openTag(tag tagRef) bool {
	if !a.loadTag(tag) {
		return false
	}
	a.show(pageMain)
	return true
}

// showStations fills the stations list: in the narrow layout a link back to
// the tags, then "Random", then the stations. The cursor goes to the station
// playing.
func (a *Application) showStations(stations []Station) {
	list := a.stationsList
	list.Clear()
	a.listed = stations
	a.stationRows = a.stationRows[:0]
	a.stationsBackLink = a.tag.name != "" && !a.wide

	if a.stationsBackLink {
		list.AddItem(glyphs.back+" "+tview.Escape(a.tag.name), "", '#', func() {
			a.tag = tagRef{}
			a.show(pageTags)
		})
		a.stationRows = append(a.stationRows, "")
	}
	if len(stations) > 0 {
		list.AddItem("  Random", "", '*', func() {
			a.stopShuffle()
			a.playRandom()
		})
		a.stationRows = append(a.stationRows, "")
	}
	for i, s := range stations {
		list.AddItem("  "+stationLabel(s, a.bookmarks.has(s.url)), "", idxToRune(i), func() { a.togglePlayManual(s) })
		a.stationRows = append(a.stationRows, s.url)
	}

	list.SetCurrentItem(0)
	a.selectStation(a.player.URL())

	a.emptyStationsText = ""
	if len(stations) == 0 {
		a.emptyStationsText = "No stations"
		if a.tag == (tagRef{name: bookmarksTag}) {
			a.emptyStationsText = "No bookmarks yet " + glyphs.dot + " press Ctrl+B on a station to add it"
		}
	}
	a.setStationsTitle(len(stations))
}

func stationLabel(s Station, bookmarked bool) string {
	label := tview.Escape(s.title)
	if bookmarked {
		label += " " + fgTag(colorDim) + glyphs.star + "[-]"
	}
	return label
}

func (a *Application) setStationsTitle(count int) {
	name, icon := a.tag.name, fgTag(colorSong)+glyphs.notes+"[-]"
	if name == "" && !a.tag.search {
		name = allStationsTag
	}
	if a.tag.search && a.lastSearch.online {
		name += " (online)"
	}
	unit := "stations"
	if count == 1 {
		unit = "station"
	}
	a.stationsList.SetTitle(fmt.Sprintf(" %s %s %s %d %s ", icon, tview.Escape(name), glyphs.dot, count, unit))
}

// stationOffset is the number of rows above the first station.
func (a *Application) stationOffset() int {
	return len(a.stationRows) - len(a.listed)
}

// selectListed moves the cursor to the i-th station listed.
func (a *Application) selectListed(i int) {
	a.stationsList.SetCurrentItem(i + a.stationOffset())
}

func (a *Application) selectStation(url string) bool {
	if url == "" {
		return false
	}
	for i, rowURL := range a.stationRows {
		if rowURL == url {
			a.stationsList.SetCurrentItem(i)
			return true
		}
	}
	return false
}

// reloadStations rebuilds the stations list, keeping the cursor.
func (a *Application) reloadStations() {
	cursor := a.stationsList.GetCurrentItem() - a.stationOffset()
	a.showStations(a.stationsForTag(a.tag))
	if cursor >= 0 {
		a.selectListed(cursor)
	}
}
