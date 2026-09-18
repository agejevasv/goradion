package radio

import (
	"fmt"
	"slices"

	"github.com/rivo/tview"
)

const (
	favoritesTag   = "Favorites"
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

// showTags fills the tags list: favourites, all stations, the tags of the
// stations and the last search.
func (a *Application) showTags() {
	a.syncingTags = true
	defer func() { a.syncingTags = false }()

	list := a.tagsList
	cursor := list.GetCurrentItem()
	list.Clear()
	a.tagRows = a.tagRows[:0]
	add := func(label string, shortcut rune, tag string) {
		list.AddItem(label, "", shortcut, func() { a.openTag(tag) })
		a.tagRows = append(a.tagRows, tag)
	}

	add(favoritesTag, '$', favoritesTag)
	add(allStationsTag, '~', allStationsTag)
	for i, tag := range a.tags {
		add(tview.Escape(tag), idxToRune(i), tag)
	}
	if q := a.lastSearch.query; q != "" {
		label := tview.Escape(q)
		if a.lastSearch.online {
			label += " " + fgTag(colorDim) + "(online)[-]"
		}
		add(label, '^', q)
	}
	list.SetCurrentItem(cursor)
}

func (a *Application) refreshTags() {
	a.showTags()
	if a.wide {
		a.syncTagCursor()
	}
}

// stationsForTag is what the stations list shows for a tag or search.
func (a *Application) stationsForTag(tag string) []Station {
	switch {
	case tag == "" || tag == allStationsTag:
		return a.stations
	case tag == a.lastSearch.query:
		return a.lastSearch.stations
	case tag == favoritesTag:
		return a.favorites.list()
	}
	var match []Station
	for _, s := range a.stations {
		if slices.Contains(s.tags, tag) {
			match = append(match, s)
		}
	}
	return match
}

// loadTag is openTag without switching pages.
func (a *Application) loadTag(tag string) bool {
	if tag == "" || (tag != favoritesTag && tag != allStationsTag && tag != a.lastSearch.query &&
		!slices.Contains(a.tags, tag)) {
		return false
	}
	a.tag = tag
	a.showStations(a.stationsForTag(tag))
	return true
}

func (a *Application) openTag(tag string) bool {
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
	a.stationsBackLink = a.tag != "" && !a.wide

	if a.stationsBackLink {
		list.AddItem(glyphs.back+" "+tview.Escape(a.tag), "", '#', func() {
			a.tag = ""
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
		list.AddItem("  "+stationLabel(s), "", idxToRune(i), func() { a.togglePlayManual(s) })
		a.stationRows = append(a.stationRows, s.url)
	}

	list.SetCurrentItem(0)
	a.selectStation(a.player.URL())

	a.emptyStationsText = ""
	if len(stations) == 0 {
		a.emptyStationsText = "No stations"
		if a.tag == favoritesTag {
			a.emptyStationsText = "No favourites yet " + glyphs.dot + " stations you play land here"
		}
	}
	a.setStationsTitle(len(stations))
}

func stationLabel(s Station) string {
	label := tview.Escape(s.title)
	if s.plays > 0 {
		label += fmt.Sprintf(" %s(%d)[-]", fgTag(colorDim), s.plays)
	}
	return label
}

func (a *Application) setStationsTitle(count int) {
	name, icon := a.tag, glyphs.notes
	switch a.tag {
	case "":
		name = allStationsTag
	case favoritesTag:
		icon = glyphs.star
	}
	if a.tag != "" && a.tag == a.lastSearch.query && a.lastSearch.online {
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
