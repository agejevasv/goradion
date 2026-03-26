package radio

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func (a *Application) setupBrowseModal() {
	a.browseInput = tview.NewInputField().
		SetLabel("Search: ").
		SetFieldWidth(0)

	a.browseInput.SetFieldBackgroundColor(tcell.ColorBlack)
	a.browseInput.SetBackgroundColor(tcell.ColorDefault)
	a.browseInput.SetLabelColor(tcell.ColorYellow)

	a.browseInput.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEscape:
			a.pages.HidePage(a.pageNames[Browse])
			return nil
		case tcell.KeyEnter:
			query := a.browseInput.GetText()
			if query != "" {
				go a.doBrowseSearch(query)
			}
			return nil
		case tcell.KeyDown, tcell.KeyTab:
			a.app.SetFocus(a.browseResults)
			if a.browseResults.GetItemCount() > 0 {
				a.browseResults.SetCurrentItem(0)
			}
			return nil
		}
		return event
	})

	a.browseResults = newList()
	a.browseResults.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEscape:
			a.pages.HidePage(a.pageNames[Browse])
			return nil
		case tcell.KeyUp:
			if a.browseResults.GetCurrentItem() == 0 {
				a.app.SetFocus(a.browseInput)
				return nil
			}
		}
		return event
	})

	browseContent := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(a.browseInput, 1, 0, true).
		AddItem(a.browseResults, 0, 1, false)

	browseContent.SetBorder(true).SetTitle(" Radio Browser ").SetBackgroundColor(tcell.ColorDefault)

	a.browseModal = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 5, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexColumn).
			AddItem(nil, 0, 5, false).
			AddItem(browseContent, 0, 90, true).
			AddItem(nil, 0, 5, false), 0, 90, true).
		AddItem(nil, 0, 5, false)

	a.pages.AddPage(a.pageNames[Browse], a.browseModal, true, false)
}

func (a *Application) showBrowseModal() {
	a.browseInput.SetText("")
	a.browseResults.Clear()
	a.pages.ShowPage(a.pageNames[Browse])
	a.app.SetFocus(a.browseInput)
}

func (a *Application) doBrowseSearch(query string) {
	a.app.QueueUpdateDraw(func() {
		a.browseResults.Clear()
		a.browseResults.AddItem("[yellow]Searching...[-]", "", 0, nil)
	})

	results, err := SearchRadioBrowser(query)

	a.app.QueueUpdateDraw(func() {
		a.browseResults.Clear()

		if err != nil {
			a.browseResults.AddItem(fmt.Sprintf("[red]Error: %s[-]", err.Error()), "", 0, nil)
			return
		}

		if len(results) == 0 {
			a.browseResults.AddItem("No stations found", "", rune('!'), nil)
			return
		}

		allStations := make([]Station, len(results))
		for i, r := range results {
			allStations[i] = r.station
		}

		for i, r := range results {
			displayTitle := stripBraces(r.station.title)
			meta := ""
			if r.countryCode != "" {
				meta = r.countryCode
			}
			if r.bitrate > 0 {
				if meta != "" {
					meta += ", "
				}
				meta += fmt.Sprintf("%dk", r.bitrate)
			}
			if meta != "" {
				displayTitle += fmt.Sprintf(" [gray](%s)[-]", meta)
			}

			station := r.station
			a.browseResults.AddItem(displayTitle, "", idxToRune(i), func() {
				a.selectBrowseResult(query, station, allStations)
			})
		}
	})
}

func (a *Application) selectBrowseResult(query string, selectedStation Station, allStations []Station) {
	a.tag = query
	a.lastSearchTag = a.tag
	a.lastBrowseStations = allStations
	a.setupStationsList(a.stationsList, allStations)
	a.refreshTagsPage()
	a.show(Main)
	a.pages.HidePage(a.pageNames[Browse])

	stationIndex := a.findStationIndex(selectedStation.url, allStations)
	a.stationsList.SetCurrentItem(stationIndex)
	go a.togglePlayManual(selectedStation)
}
