package radio

import (
	"fmt"
	"math/rand"
	"sort"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const (
	favoritesTag = "Favorites"
	helpString   = `Keyboard Control

	[green]*[-]
		Toggle playing a random station.

	[green]#[-] or [green]/[-]
		Show tag selection screen.

	[green]~[-]
		Show all stations (ignore tags).

	[green]Ctrl+F[-] or [green]:[-]
		Show search to find stations.

	[green]a[-]-[green]z[-] and [green]A[-]-[green]Z[-]
		Toggle playing a station marked with a given letter (or select a tag).

	[green]Enter[-] and [green]Space[-]
		Toggle playing currently selected station.

	[green]Left[-] and [green]Right[-], [green]-[-] and [green]+[-]
		Change the volume in increments of 5.

	[green]Up[-] and [green]Down[-]
		Cycle through the radio station list.

	[green]PgUp[-] and [green]PgDown[-]
		Jump to a beginning/end of a station list.

	[green]Esc[-]
		Close current window.

	[green]?[-]
		Show help screen.`
)

type Page int

const (
	Main = iota
	Help
	Tags
	Search
)

type Application struct {
	pageNames     []string
	stations      []Station
	player        *Player
	tag           string
	lastSearchTag string
	app           *tview.Application
	pages         *tview.Pages
	stationsList  *tview.List
	tagsList      *tview.List
	tagsFlex      *tview.Flex
	status        *tview.TextView
	volume        *tview.TextView
	favorites     *Favorites
	searchModal   *tview.Flex
	searchInput   *tview.InputField
	searchResults *tview.List
}

func NewApp(player *Player, stations []Station) *Application {
	a := &Application{
		player:    player,
		stations:  stations,
		pageNames: []string{"Main", "Help", "Tags", "Search"},
		favorites: NewFavorites(stations),
	}

	a.setupPages()
	a.setupSearchModal()

	a.app = tview.NewApplication().
		SetRoot(a.pages, true).
		EnableMouse(true).
		SetMouseCapture(devNullMouse()).
		SetInputCapture(a.inputCapture())

	go a.updateStatus()

	return a
}

func (a *Application) Run() error {
	return a.app.Run()
}

func (a *Application) setupPages() {
	a.stationsList = a.setupStationsList(newList(), a.stations)
	a.tagsList = a.setupTagsList()

	a.status = tview.NewTextView().
		SetTextColor(tcell.ColorLightGray).
		SetDynamicColors(true).
		SetText("Ready [gray]| [green]Press ? for help")

	a.volume = tview.NewTextView().
		SetDynamicColors(true).
		SetTextColor(tcell.ColorLightGray).
		SetTextAlign(tview.AlignRight)

	statusFlex := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(a.status, 0, 100, true).
		AddItem(a.volume, 0, 25, false)

	flex := tview.NewFlex().
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(a.stationsList, 0, 100, true).
			AddItem(statusFlex, 0, 1, true), 0, 1, true)

	a.tagsFlex = tview.NewFlex().
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(a.tagsList, 0, 100, true).
			AddItem(statusFlex, 0, 1, true), 0, 1, true)

	help := tview.NewTextView().
		SetDynamicColors(true).
		SetText(fmt.Sprintf("[green]%s\n\n[default]%s", VersionString(), helpString))
	help.SetBackgroundColor(tcell.ColorDefault)

	a.pages = tview.NewPages().
		AddPage(a.pageNames[Tags], a.tagsFlex, true, true).
		AddPage(a.pageNames[Main], flex, true, false).
		AddPage(a.pageNames[Help], help, true, false)
}

func (a *Application) show(page Page) {
	a.pages.SwitchToPage(a.pageNames[page])
}

func (a *Application) toggle(page Page) {
	if a.pages.GetPageNames(true)[0] == a.pageNames[page] {
		a.show(Tags)
	} else {
		a.show(page)
	}
}

func (a *Application) inputCapture() func(event *tcell.EventKey) *tcell.EventKey {
	return func(event *tcell.EventKey) *tcell.EventKey {
		switch key := event.Key(); key {
		case tcell.KeyEscape:
			if a.pages.GetPageNames(true)[0] != a.pageNames[Tags] {
				a.show(Tags)
				return nil
			} else {
				a.app.Stop()
			}
		case tcell.KeyCtrlF:
			a.showSearchModal()
			return nil
		case tcell.KeyLeft:
			a.player.VolumeDn()
			return nil
		case tcell.KeyRight:
			a.player.VolumeUp()
			return nil
		case tcell.KeyRune:
			switch event.Rune() {
			case '=', '+':
				a.player.VolumeUp()
				return nil
			case '-', '_':
				a.player.VolumeDn()
				return nil
			case '/', '#':
				a.show(Tags)
				return nil
			case '?':
				a.toggle(Help)
				return nil
			case '~':
				a.tag = "All Stations"
				a.filterStationsForSelectedTag()
				a.show(Main)
				return nil
			case ':':
				a.showSearchModal()
				return nil
			}
		}
		return event
	}
}

func (a *Application) updateStatus() {
	for inf := range a.player.Info {
		if inf.Song == "" && inf.Status == "" {
			a.status.SetText(inf.Station)
		} else if inf.Song == "" {
			a.status.SetText(fmt.Sprintf("%s [gray]| [green]%s", inf.Station, inf.Status))
		} else {
			a.status.SetText(fmt.Sprintf("%s [gray]| [green]%s", inf.Station, stripBraces(inf.Song)))
		}

		if inf.Bitrate > 0 {
			a.volume.SetText(fmt.Sprintf("%d kb/s [gray]|[lightgray] %d%%", inf.Bitrate, inf.Volume))
		} else {
			a.volume.SetText(fmt.Sprintf("%d%%", inf.Volume))
		}

		a.app.Draw()
	}
}

func (a *Application) setupStationsList(list *tview.List, stations []Station) *tview.List {
	list.Clear()
	list.SetCurrentItem(0)

	offset := a.calculateStationListOffset()

	if a.tag != "" {
		list = list.AddItem(fmt.Sprintf("[red:black:]%s", a.tag), "", rune('#'), func() {
			a.show(Tags)
		})
	}

	list.AddItem("Random", "", rune('*'), func() {
		r := rand.Intn(len(stations))

		for len(stations) > 1 && a.player.info.Url == stations[r].url {
			r = rand.Intn(len(stations))
		}

		list.SetCurrentItem(r + offset)
		go a.togglePlay(stations[r])
	})

	for i := 0; i < len(stations); i++ {
		list = list.AddItem(stations[i].title, "", idxToRune(i), func() {
			go a.togglePlay(stations[i])
		})

		if a.player.info.Url == stations[i].url {
			list.SetCurrentItem(i + offset)
		}
	}

	return list
}

func (a *Application) setupTagsList() *tview.List {
	tagsList := newList()

	tags := tags(a.stations)

	if a.favorites.hasFavorites() {
		tagsList = tagsList.AddItem(favoritesTag, "", rune('$'), func() {
			a.tag = favoritesTag
			a.filterStationsForSelectedTag()
			a.show(Main)
		})
	}

	for i := 0; i < len(tags); i++ {
		tagsList = tagsList.AddItem(tags[i], "", idxToRune(i), func() {
			a.tag = tags[i]
			a.filterStationsForSelectedTag()
			a.show(Main)
		})
	}

	if a.lastSearchTag != "" {
		tagsList = tagsList.AddItem(a.lastSearchTag, "", rune('^'), func() {
			matchedStations := a.filterStations(a.lastSearchTag)
			a.tag = a.lastSearchTag
			a.setupStationsList(a.stationsList, matchedStations)
			a.show(Main)
		})
	}

	if len(tags) == 0 && !a.favorites.hasFavorites() {
		tagsList = tagsList.AddItem("No tags were found", "", rune('#'), func() {
			a.show(Main)
		})
	}

	return tagsList
}

func (a *Application) togglePlay(station Station) {
	if station.url != "" && station.url != a.player.info.Url {
		a.favorites.track(station)
		if a.tag == favoritesTag {
			a.filterStationsForSelectedTag()
			a.findAndSelectStation(station.url)
		}
	}
	a.player.Toggle(station)
}

func (a *Application) filterStationsForSelectedTag() {
	match := make([]Station, 0)

	if a.tag == favoritesTag {
		match = a.favorites.getFavoriteStations()
	} else {
		for i := 0; i < len(a.stations); i++ {
			for _, t := range a.stations[i].tags {
				if a.tag == "All Stations" || t == a.tag {
					match = append(match, a.stations[i])
					break
				}
			}
		}
	}
	a.setupStationsList(a.stationsList, match)
}

func (a *Application) findAndSelectStation(stationURL string) {
	if a.tag == favoritesTag {
		favStations := a.favorites.getFavoriteStations()
		for i, station := range favStations {
			if station.url == stationURL {
				a.stationsList.SetCurrentItem(i + a.calculateStationListOffset())
				break
			}
		}
	}
}

func newList() *tview.List {
	list := tview.NewList()
	list.ShowSecondaryText(false)
	list.SetBackgroundColor(tcell.ColorDefault)
	list.SetSelectedStyle(tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorGreen).Bold(true))
	list.SetMainTextStyle(tcell.StyleDefault.Foreground(tcell.ColorDefault).Background(tcell.ColorDefault))
	list.SetShortcutStyle(tcell.StyleDefault.Foreground(tcell.ColorDefault).Background(tcell.ColorDefault))
	return list
}

func devNullMouse() func(event *tcell.EventMouse, action tview.MouseAction) (*tcell.EventMouse, tview.MouseAction) {
	return func(event *tcell.EventMouse, action tview.MouseAction) (*tcell.EventMouse, tview.MouseAction) {
		// do nothing, please
		return nil, action
	}
}

func tags(stations []Station) []string {
	tagsMap := make(map[string]bool)

	for _, s := range stations {
		for _, t := range s.tags {
			tagsMap[t] = true
		}
	}

	tags := make([]string, 0, len(tagsMap))

	for tag := range tagsMap {
		tags = append(tags, tag)
	}

	sort.Sort(sort.StringSlice(tags))
	return tags
}

func stripBraces(s string) string {
	s = strings.ReplaceAll(s, "[", "(")
	return strings.ReplaceAll(s, "]", ")")
}

func idxToRune(i int) rune {
	if i+97 <= 122 {
		return rune(i + 97)
	}

	// A-Z
	if i -= 26; i+65 <= 90 {
		return rune(i + 65)
	}

	// 1-9
	if i -= 26; i+49 <= 57 {
		return rune(i + 49)
	}

	return 0
}

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
		if a.fuzzyMatch(station, queryWords) {
			matchedStations = append(matchedStations, station)
		}
	}

	return matchedStations
}

func (a *Application) calculateStationListOffset() int {
	offset := 1

	if a.tag != "" {
		offset++
	}

	return offset
}

func (a *Application) findStationIndex(stationURL string, stations []Station) int {
	offset := a.calculateStationListOffset()

	for i, station := range stations {
		if station.url == stationURL {
			return i + offset
		}
	}
	return offset
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
	a.setupStationsList(a.stationsList, stations)
	a.refreshTagsPage()
	a.show(Main)
	a.pages.HidePage(a.pageNames[Search])

	stationIndex := a.findStationIndex(selectedStation.url, stations)
	a.stationsList.SetCurrentItem(stationIndex)
	go a.togglePlay(selectedStation)
}

func (a *Application) fuzzyMatch(station Station, queryWords []string) bool {
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

func (a *Application) search(query string) {
	matchedStations := a.filterStations(query)

	if len(matchedStations) > 0 {
		a.tag = query
		a.lastSearchTag = a.tag
		a.setupStationsList(a.stationsList, matchedStations)
		a.refreshTagsPage()
		a.show(Main)
		a.pages.HidePage(a.pageNames[Search])
	}
}

func (a *Application) refreshTagsPage() {
	a.tagsList = a.setupTagsList()
	a.tagsFlex.Clear()
	statusFlex := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(a.status, 0, 100, true).
		AddItem(a.volume, 0, 25, false)
	a.tagsFlex.AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(a.tagsList, 0, 100, true).
		AddItem(statusFlex, 0, 1, true), 0, 1, true)
}
