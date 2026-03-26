package radio

import (
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func (a *Application) setupSearchModal() {
	a.searchInput = tview.NewInputField().
		SetLabel("Search: ").
		SetFieldWidth(0).
		SetChangedFunc(a.updateSearchResults)

	a.searchInput.SetFieldBackgroundColor(tcell.ColorBlack)
	a.searchInput.SetBackgroundColor(tcell.ColorDefault)
	a.searchInput.SetLabelColor(tcell.ColorGreen)

	a.searchInput.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEscape:
			a.pages.HidePage(a.pageNames[Search])
			return nil
		case tcell.KeyEnter:
			text := a.searchInput.GetText()
			if text != "" {
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
			a.pages.HidePage(a.pageNames[Search])
			return nil
		case tcell.KeyUp:
			if a.searchResults.GetCurrentItem() == 0 {
				a.app.SetFocus(a.searchInput)
				return nil
			}
		}
		return event
	})

	searchContent := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(a.searchInput, 1, 0, true).
		AddItem(a.searchResults, 0, 1, false)

	searchContent.SetBorder(true).SetTitle(" Station Search ").SetBackgroundColor(tcell.ColorDefault)

	a.searchModal = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 5, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexColumn).
			AddItem(nil, 0, 5, false).
			AddItem(searchContent, 0, 90, true).
			AddItem(nil, 0, 5, false), 0, 90, true).
		AddItem(nil, 0, 5, false)

	a.pages.AddPage(a.pageNames[Search], a.searchModal, true, false)
}

func (a *Application) showSearchModal() {
	a.searchInput.SetText("")
	a.searchResults.Clear()
	a.updateSearchResults("")
	a.pages.ShowPage(a.pageNames[Search])
	a.app.SetFocus(a.searchInput)
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

func (a *Application) updateSearchResults(query string) {
	a.searchResults.Clear()

	matchedStations := a.filterStations(query)

	for _, station := range matchedStations {
		currentStation := station
		a.searchResults.AddItem(station.title, "", 0, func() {
			a.selectFoundResults(query, matchedStations, currentStation)
		})
	}

	if len(matchedStations) == 0 && query != "" {
		a.searchResults.AddItem("No stations found", "", rune('!'), nil)
	}
}

func (a *Application) selectFoundResults(query string, stations []Station, selectedStation Station) {
	a.tag = query
	a.lastSearchTag = a.tag
	a.lastBrowseStations = nil
	a.setupStationsList(a.stationsList, stations)
	a.refreshTagsPage()
	a.show(Main)
	a.pages.HidePage(a.pageNames[Search])

	stationIndex := a.findStationIndex(selectedStation.url, stations)
	a.stationsList.SetCurrentItem(stationIndex)
	go a.togglePlayManual(selectedStation)
}

func (a *Application) search(query string) {
	matchedStations := a.filterStations(query)

	if len(matchedStations) > 0 {
		a.tag = query
		a.lastSearchTag = a.tag
		a.lastBrowseStations = nil
		a.setupStationsList(a.stationsList, matchedStations)
		a.refreshTagsPage()
		a.show(Main)
		a.pages.HidePage(a.pageNames[Search])
	}
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
