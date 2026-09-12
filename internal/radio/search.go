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

	a.searchInput.SetFieldBackgroundColor(tcell.ColorBlack)
	a.searchInput.SetBackgroundColor(tcell.ColorDefault)

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
		AddItem(a.searchResults, 0, 1, false)

	a.searchContent.SetBorder(true).SetBackgroundColor(tcell.ColorDefault)
	a.applySearchMode()

	a.searchModal = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 5, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexColumn).
			AddItem(nil, 0, 5, false).
			AddItem(a.searchContent, 0, 90, true).
			AddItem(nil, 0, 5, false), 0, 90, true).
		AddItem(nil, 0, 5, false)

	a.pages.AddPage(a.pageNames[Search], a.searchModal, true, false)
}

// showSearchModal opens the search modal in the given mode. If the modal is
// already open, the mode is flipped instead and the current query is re-run.
func (a *Application) showSearchModal(online bool) {
	if a.isSearchModalOpen() {
		a.setSearchMode(!a.searchOnline)
		return
	}

	a.searchOnline = online
	a.searchGeneration++
	a.searchInput.SetText("")
	a.applySearchMode()
	a.searchResults.Clear()
	a.showSearchPlaceholder()
	a.pages.ShowPage(a.pageNames[Search])
	a.app.SetFocus(a.searchInput)
}

func (a *Application) hideSearchModal() {
	a.searchGeneration++
	a.pages.HidePage(a.pageNames[Search])
}

func (a *Application) isSearchModalOpen() bool {
	return a.pages.GetPageNames(true)[0] == a.pageNames[Search]
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

// applySearchMode updates the modal title and label colour to reflect the mode.
func (a *Application) applySearchMode() {
	if a.searchOnline {
		a.searchInput.SetLabelColor(tcell.ColorYellow)
		a.searchContent.SetTitle(" Station Search: Local [black:yellow:b] Online [-:-:-] ")
	} else {
		a.searchInput.SetLabelColor(tcell.ColorGreen)
		a.searchContent.SetTitle(" Station Search: [black:green:b] Local [-:-:-] Online ")
	}
}

func (a *Application) showSearchPlaceholder() {
	if a.searchOnline {
		a.searchResults.AddItem(fmt.Sprintf("[gray]%s[-]", onlineSearchHint), "", 0, nil)
	}
}

func (a *Application) filterStations(query string) []Station {
	if query == "" {
		return nil
	}

	queryWords := strings.Fields(strings.ToLower(query))
	var matchedStations []Station

	for _, station := range a.stations {
		if fuzzyMatch(station, queryWords) {
			matchedStations = append(matchedStations, station)
		}
	}

	return matchedStations
}

// updateSearchResults performs a live local search as the user types.
func (a *Application) updateSearchResults(query string) {
	results := make([]searchResult, 0)
	for _, station := range a.filterStations(query) {
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
	a.searchResults.AddItem("[yellow]Searching...[-]", "", 0, nil)

	go func() {
		found, err := SearchRadioBrowser(query)

		a.app.QueueUpdateDraw(func() {
			if generation != a.searchGeneration {
				return
			}

			if err != nil {
				a.searchResults.Clear()
				a.searchResults.AddItem(fmt.Sprintf("[red]Error: %s[-]", err.Error()), "", 0, nil)
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

func onlineMeta(r RadioBrowserResult) string {
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
			a.searchResults.AddItem("No stations found", "", rune('!'), nil)
		}
		return
	}

	stations := make([]Station, len(results))
	for i, r := range results {
		stations[i] = r.station
	}

	for i, r := range results {
		title := r.station.title
		shortcut := rune(0)
		if online {
			title = stripBraces(title)
			shortcut = idxToRune(i)
		}
		if r.meta != "" {
			title += fmt.Sprintf(" [gray](%s)[-]", r.meta)
		}

		station := r.station
		a.searchResults.AddItem(title, "", shortcut, func() {
			a.selectSearchResult(query, stations, station, online)
		})
	}
}

func (a *Application) selectSearchResult(query string, stations []Station, selected Station, online bool) {
	a.openSearchInMain(query, stations, online)

	stationIndex := a.findStationIndex(selected.url, stations)
	a.stationsList.SetCurrentItem(stationIndex)
	go a.togglePlayManual(selected)
}

// search is triggered by Enter in local mode: shows the matching stations in
// the main view without starting playback.
func (a *Application) search(query string) {
	matchedStations := a.filterStations(query)

	if len(matchedStations) > 0 {
		a.openSearchInMain(query, matchedStations, false)
	}
}

func (a *Application) openSearchInMain(query string, stations []Station, online bool) {
	a.tag = query
	a.lastSearchTag = a.tag
	if online {
		a.lastOnlineStations = stations
	} else {
		a.lastOnlineStations = nil
	}
	a.setupStationsList(a.stationsList, stations)
	a.refreshTagsPage()
	a.show(Main)
	a.hideSearchModal()
}

func fuzzyMatch(station Station, queryWords []string) bool {
	stationTitle := strings.ToLower(station.title)

	for _, word := range queryWords {
		wordFound := false

		if strings.Contains(stationTitle, word) {
			wordFound = true
		}

		if !wordFound {
			for _, tag := range station.tags {
				if strings.Contains(strings.ToLower(tag), word) {
					wordFound = true
					break
				}
			}
		}

		if !wordFound {
			return false
		}
	}

	return true
}
