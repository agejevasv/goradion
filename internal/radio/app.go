package radio

import (
	"context"
	"fmt"
	"math/rand"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const (
	favoritesTag = "Favorites"
	fadeDuration = 2 * time.Second
	helpString   = `Keyboard Control

	[green]*[-]
		Toggle playing a random station.

	[green]#[-] or [green]/[-]
		Show tag selection screen.

	[green]~[-]
		Show all stations (ignore tags).

	[green]a[-]-[green]z[-] and [green]A[-]-[green]Z[-]
		Toggle playing a station marked with a given letter (or select a tag).

	[green]Ctrl+F[-] or [green]:[-]
		Show search to find stations.

	[green]Ctrl+S[-]
		Search online via radio-browser.info.

	[green]Ctrl+R[-]
		Toggle shuffle mode (plays a random station at timed intervals).

	[green]Alt+1[-] to [green]Alt+9[-]
		Set shuffle interval to 1-9 minutes and reset timer.

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
	Browse
)

type Application struct {
	pageNames               []string
	stations                []Station
	player                  *Player
	tag                     string
	lastSearchTag           string
	pageHistory             []Page
	app                     *tview.Application
	pages                   *tview.Pages
	stationsList            *tview.List
	tagsList                *tview.List
	tagsFlex                *tview.Flex
	mainFlex                *tview.Flex
	status                  *tview.TextView
	volume                  *tview.TextView
	favorites               *Favorites
	searchModal             *tview.Flex
	searchInput             *tview.InputField
	searchResults           *tview.List
	browseModal             *tview.Flex
	browseInput             *tview.InputField
	browseResults           *tview.List
	lastBrowseStations      []Station
	timedRandomActive       bool
	timedRandomCancel       context.CancelFunc
	shuffleIterationStartAt time.Time
	shuffleInterval         time.Duration
	waitingForPlayback      chan struct{}
	waitingForURL           string
}

func NewApp(player *Player, stations []Station) *Application {
	a := &Application{
		player:          player,
		stations:        stations,
		pageNames:       []string{"Main", "Help", "Tags", "Search", "Browse"},
		favorites:       NewFavorites(stations),
		shuffleInterval: 5 * time.Minute,
	}

	a.setupPages()
	a.setupSearchModal()
	a.setupBrowseModal()

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

	a.mainFlex = tview.NewFlex().
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
		AddPage(a.pageNames[Main], a.mainFlex, true, false).
		AddPage(a.pageNames[Help], help, true, false)
	a.pageHistory = append(a.pageHistory, Tags)
}

func (a *Application) show(page Page) {
	a.pageHistory = append(a.pageHistory, page)

	if len(a.pageHistory) > 2 {
		a.pageHistory = a.pageHistory[len(a.pageHistory)-2:]
	}

	a.pages.SwitchToPage(a.pageNames[page])
}

func (a *Application) inputCapture() func(event *tcell.EventKey) *tcell.EventKey {
	return func(event *tcell.EventKey) *tcell.EventKey {
		var currentPage = a.pages.GetPageNames(true)[0]

		closeHelp := func() bool {
			if currentPage == a.pageNames[Help] && len(a.pageHistory) > 1 {
				previous := a.pageHistory[len(a.pageHistory)-2]
				if previous != Help {
					a.show(previous)
					return true
				}
			}
			return false
		}

		switch key := event.Key(); key {
		case tcell.KeyEscape:
			if currentPage == a.pageNames[Search] || currentPage == a.pageNames[Browse] {
				return event
			}

			if currentPage == a.pageNames[Tags] {
				a.app.Stop()
				return nil
			}

			if currentPage == a.pageNames[Main] {
				a.tag = ""
				a.show(Tags)
				return nil
			}

			if !closeHelp() {
				a.show(Tags)
			}
			return nil
		case tcell.KeyCtrlF:
			a.showSearchModal()
			return nil
		case tcell.KeyCtrlS:
			a.showBrowseModal()
			return nil
		case tcell.KeyCtrlR:
			go a.toggleTimedRandom()
			return nil
		case tcell.KeyLeft:
			a.player.VolumeDn()
			return nil
		case tcell.KeyRight:
			a.player.VolumeUp()
			return nil
		case tcell.KeyRune:
			if event.Modifiers()&tcell.ModAlt != 0 {
				r := event.Rune()
				if r >= '1' && r <= '9' {
					minutes := int(r - '0')
					go a.setShuffleInterval(minutes)
					return nil
				}
			}
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
				if !closeHelp() {
					a.show(Help)
				}
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
		stationName := stripPlayCount(inf.Station)
		if inf.Song == "" && inf.Status == "" {
			a.status.SetText(stationName)
		} else if inf.Song == "" {
			a.status.SetText(fmt.Sprintf("%s [gray]| [green]%s", stationName, inf.Status))
		} else {
			a.status.SetText(fmt.Sprintf("%s [gray]| [green]%s", stationName, stripBraces(inf.Song)))
		}

		if inf.Bitrate > 0 {
			a.volume.SetText(fmt.Sprintf("%d kb/s [gray]|[lightgray] %d%%", inf.Bitrate, inf.Volume))
		} else {
			a.volume.SetText(fmt.Sprintf("%d%%", inf.Volume))
		}

		if a.waitingForPlayback != nil && inf.Url == a.waitingForURL && (inf.Status == "Playing" || inf.Song != "") {
			close(a.waitingForPlayback)
			a.waitingForPlayback = nil
			a.waitingForURL = ""
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
			a.tag = ""
			a.show(Tags)
		})
	}

	list.AddItem("Random", "", rune('*'), func() {
		r := rand.Intn(len(stations))

		for len(stations) > 1 && a.player.info.Url == stations[r].url {
			r = rand.Intn(len(stations))
		}

		list.SetCurrentItem(r + offset)
		go a.togglePlayManual(stations[r])
	})

	for i := range stations {
		list = list.AddItem(stations[i].title, "", idxToRune(i), func() {
			go a.togglePlayManual(stations[i])
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

	for i := range tags {
		tagsList = tagsList.AddItem(tags[i], "", idxToRune(i), func() {
			a.tag = tags[i]
			a.filterStationsForSelectedTag()
			a.show(Main)
		})
	}

	if a.lastSearchTag != "" {
		tagsList = tagsList.AddItem(a.lastSearchTag, "", rune('^'), func() {
			var matchedStations []Station
			if a.lastBrowseStations != nil {
				matchedStations = a.lastBrowseStations
			} else {
				matchedStations = a.filterStations(a.lastSearchTag)
			}
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

func (a *Application) togglePlayManual(station Station) {
	if a.timedRandomActive {
		a.toggleTimedRandom()
	}
	a.togglePlay(station)
}

func (a *Application) getStationsFromCurrentView() []Station {
	if a.tag == "" {
		return a.stations
	}

	if a.tag == favoritesTag {
		return a.favorites.getFavoriteStations()
	}

	if a.tag == "All Stations" {
		return a.stations
	}

	if a.tag == a.lastSearchTag && a.lastSearchTag != "" {
		if a.lastBrowseStations != nil {
			return a.lastBrowseStations
		}
		return a.filterStations(a.tag)
	}

	match := make([]Station, 0)
	for i := 0; i < len(a.stations); i++ {
		if slices.Contains(a.stations[i].tags, a.tag) {
			match = append(match, a.stations[i])
		}
	}
	return match
}

func (a *Application) filterStationsForSelectedTag() {
	match := make([]Station, 0)

	if a.tag == favoritesTag {
		match = a.favorites.getFavoriteStations()
	} else {
		for i := 0; i < len(a.stations); i++ {
			if a.tag == "All Stations" || slices.Contains(a.stations[i].tags, a.tag) {
				match = append(match, a.stations[i])
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

func stripPlayCount(s string) string {
	re := regexp.MustCompile(` \[gray\]\(\d+\)\[-\]$`)
	return re.ReplaceAllString(s, "")
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
