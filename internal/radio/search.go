package radio

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const onlineSearchHint = "Press Enter to search radio-browser.info"

type searchResult struct {
	station Station
	meta    string
}

func (a *Application) setupSearchModal() {
	a.searchInput = tview.NewInputField().
		SetLabel("Search: ").
		SetFieldWidth(0).
		SetChangedFunc(func(text string) {
			if !a.searchOnline {
				a.updateSearchResults(text)
			}
		})

	a.searchInput.SetPlaceholder("station name or tag")

	a.searchInput.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEscape:
			a.hideSearchModal()
			return nil
		case tcell.KeyEnter:
			text := a.searchInput.GetText()
			if text == "" {
				return nil
			}
			if a.searchOnline {
				a.startOnlineSearch(text)
			} else {
				a.search(text)
			}
			return nil
		case tcell.KeyDown, tcell.KeyTab:
			a.app.SetFocus(a.searchResults)
			if a.searchResults.GetItemCount() > 0 {
				a.searchResults.SetCurrentItem(0)
			}
			return nil
		}
		return event
	})

	a.searchResults = newList()
	a.searchResults.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEscape:
			a.hideSearchModal()
			return nil
		case tcell.KeyUp:
			if a.searchResults.GetCurrentItem() == 0 {
				a.app.SetFocus(a.searchInput)
				return nil
			}
		}
		return event
	})

	a.searchContent = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(a.searchInput, 1, 0, true).
		AddItem(wheelList{a.searchResults}, 0, 1, false)

	// Pad the children, not the frame: a Flex does not clear its background.
	a.searchInput.SetBorderPadding(0, 0, 1, 1)
	a.searchResults.SetBorderPadding(0, 0, 1, 1)
	a.searchContent.SetBorder(true).SetTitleAlign(tview.AlignLeft)
	a.applySearchColors()

	a.searchModal = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 5, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexColumn).
			AddItem(nil, 0, 5, false).
			AddItem(a.searchContent, 0, 90, true).
			AddItem(nil, 0, 5, false), 0, 90, true).
		AddItem(nil, 0, 5, false)

	a.addPage(pageSearch, a.searchModal, false)
}

// showSearchModal opens the search modal in the given mode. If the modal is
// already open, the mode is flipped instead and the current query is re-run.
func (a *Application) showSearchModal(online bool) {
	if a.isSearchModalOpen() {
		a.setSearchMode(!a.searchOnline)
		return
	}

	a.hideRemoteModal()
	a.searchOnline = online
	a.searchGeneration++
	a.searchInput.SetText("")
	a.applySearchMode()
	a.searchResults.Clear()
	a.showSearchPlaceholder()
	a.showModal(pageSearch)
	a.app.SetFocus(a.searchInput)
}

func (a *Application) hideSearchModal() {
	a.searchGeneration++
	a.hideModal(pageSearch)
}

func (a *Application) isSearchModalOpen() bool {
	return a.isFront(pageSearch)
}

func (a *Application) setSearchMode(online bool) {
	a.searchOnline = online
	a.searchGeneration++
	a.applySearchMode()
	a.app.SetFocus(a.searchInput)

	text := a.searchInput.GetText()
	if online {
		if text != "" {
			a.startOnlineSearch(text)
		} else {
			a.searchResults.Clear()
			a.showSearchPlaceholder()
		}
		return
	}
	a.updateSearchResults(text)
}

func (a *Application) applySearchColors() {
	a.searchInput.SetBackgroundColor(colorBg)
	a.searchInput.SetFieldBackgroundColor(colorBg)
	a.searchInput.SetFieldTextColor(colorText)
	a.searchInput.SetPlaceholderStyle(styleDim)
	a.searchContent.SetBackgroundColor(colorBg)
	a.searchContent.SetBorderColor(colorDim)
	a.searchContent.SetTitleColor(colorText)
	a.applySearchMode()
}

func (a *Application) applySearchMode() {
	// Box titles are printed over the default style, so the tags spell out the
	// background instead of resetting it with "-".
	badge := func(label string, bg tcell.Color) string {
		return fmt.Sprintf("[%s:%s:b]%s[%s:%s:-]", colorTag(colorOnAccent), colorTag(bg), label, colorTag(colorText), colorTag(colorBg))
	}
	if a.searchOnline {
		a.searchInput.SetLabelColor(colorWarn)
		a.searchContent.SetTitle(" Station Search: Local " + badge(" Online ", colorWarn) + " ")
	} else {
		a.searchInput.SetLabelColor(colorAccent)
		a.searchContent.SetTitle(" Station Search: " + badge(" Local ", colorAccent) + " Online ")
	}
}

func (a *Application) showSearchPlaceholder() {
	if a.searchOnline {
		a.searchResults.AddItem(fgTag(colorDim)+onlineSearchHint+"[-]", "", 0, nil)
	}
}

// searchStations returns the stations whose title or tags contain every word
// of the query, ignoring case.
func searchStations(stations []Station, query string) []Station {
	words := strings.Fields(strings.ToLower(query))
	if len(words) == 0 {
		return nil
	}
	var match []Station
	for _, s := range stations {
		if matchesAll(s, words) {
			match = append(match, s)
		}
	}
	return match
}

func matchesAll(s Station, words []string) bool {
	title := strings.ToLower(s.title)
	for _, word := range words {
		found := strings.Contains(title, word)
		for _, tag := range s.tags {
			if found {
				break
			}
			found = strings.Contains(strings.ToLower(tag), word)
		}
		if !found {
			return false
		}
	}
	return true
}

// updateSearchResults runs the local search as the user types.
func (a *Application) updateSearchResults(query string) {
	var results []searchResult
	for _, station := range searchStations(a.stations, query) {
		results = append(results, searchResult{station: station})
	}
	a.renderSearchResults(query, results, false)
}

// startOnlineSearch kicks off an asynchronous radio-browser.info search.
// Results are discarded if the mode changed or a newer search was started.
func (a *Application) startOnlineSearch(query string) {
	a.searchGeneration++
	generation := a.searchGeneration

	a.searchResults.Clear()
	a.searchResults.AddItem(fgTag(colorWarn)+"Searching...[-]", "", 0, nil)

	go func() {
		found, err := searchRadioBrowser(query)

		a.app.QueueUpdateDraw(func() {
			if generation != a.searchGeneration {
				return
			}

			if err != nil {
				a.searchResults.Clear()
				a.searchResults.AddItem(fmt.Sprintf("%sError: %s[-]", fgTag(colorDanger), tview.Escape(err.Error())), "", 0, nil)
				return
			}

			results := make([]searchResult, len(found))
			for i, r := range found {
				results[i] = searchResult{station: r.station, meta: onlineMeta(r)}
			}
			a.renderSearchResults(query, results, true)
		})
	}()
}

func onlineMeta(r onlineStation) string {
	parts := make([]string, 0, 2)
	if r.countryCode != "" {
		parts = append(parts, r.countryCode)
	}
	if r.bitrate > 0 {
		parts = append(parts, fmt.Sprintf("%dk", r.bitrate))
	}
	return strings.Join(parts, ", ")
}

func (a *Application) renderSearchResults(query string, results []searchResult, online bool) {
	a.searchResults.Clear()

	if len(results) == 0 {
		if query != "" {
			a.searchResults.AddItem(fgTag(colorDim)+"No stations match[-]", "", 0, nil)
		}
		return
	}

	stations := make([]Station, len(results))
	for i, r := range results {
		stations[i] = r.station
	}

	for i, r := range results {
		title := tview.Escape(r.station.title)
		shortcut := rune(0)
		if online {
			shortcut = idxToRune(i)
		}
		if r.meta != "" {
			title += fmt.Sprintf(" %s(%s)[-]", fgTag(colorDim), r.meta)
		}

		station := r.station
		a.searchResults.AddItem(title, "", shortcut, func() {
			a.selectSearchResult(query, stations, station, online)
		})
	}
}

func (a *Application) selectSearchResult(query string, stations []Station, selected Station, online bool) {
	a.openSearch(query, stations, online)
	a.selectStation(selected.url)
	a.togglePlayManual(selected)
}

// search is triggered by Enter in local mode: shows the matching stations in
// the main view without starting playback.
func (a *Application) search(query string) {
	if stations := searchStations(a.stations, query); len(stations) > 0 {
		a.openSearch(query, stations, false)
	}
}

// openSearch shows search results in the stations list, and adds the search
// to the tags.
func (a *Application) openSearch(query string, stations []Station, online bool) {
	a.tag = query
	a.lastSearch = searchView{query: query, stations: stations, online: online}
	a.showStations(stations)
	a.refreshTags()
	a.show(pageMain)
	a.hideSearchModal()
}
