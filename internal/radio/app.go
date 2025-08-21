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

	[green]*[default]
		Toggle playing a random station.

	[green]#[default] or [green]/[default]
		Show tag selection screen.

	[green]~[default]
		Show all stations (ignore tags).

	[green]a[default]-[green]z[default] and [green]A[default]-[green]Z[default]
		Toggle playing a station marked with a given letter (or select a tag).

	[green]Enter[default] and [green]Space[default]
		Toggle playing currently selected station.

	[green]Left[default] and [green]Right[default], [green]-[default] and [green]+[default]
		Change the volume in increments of 5.

	[green]Up[default] and [green]Down[default]
		Cycle through the radio station list.

	[green]PgUp[default] and [green]PgDown[default]
		Jump to a beginning/end of a station list.

	[green]Esc[default]
		Close current window.

	[green]?[default]
		Show help screen.`
)

type Page int

const (
	Main = iota
	Help
	Tags
)

type Application struct {
	pageNames    []string
	stations     []Station
	player       *Player
	tag          string
	app          *tview.Application
	pages        *tview.Pages
	stationsList *tview.List
	tagsList     *tview.List
	status       *tview.TextView
	volume       *tview.TextView
	favorites    *Favorites
}

func NewApp(player *Player, stations []Station) *Application {
	a := &Application{
		player:    player,
		stations:  stations,
		pageNames: []string{"Main", "Help", "Tags"},
		favorites: NewFavorites(stations),
	}

	a.setupPages()

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

	tagsFlex := tview.NewFlex().
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(a.setupTagsList(), 0, 100, true).
			AddItem(statusFlex, 0, 1, true), 0, 1, true)

	help := tview.NewTextView().
		SetDynamicColors(true).
		SetText(fmt.Sprintf("[green]%s\n\n[default]%s", VersionString(), helpString))
	help.SetBackgroundColor(tcell.ColorDefault)

	a.pages = tview.NewPages().
		AddPage(a.pageNames[Tags], tagsFlex, true, true).
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

	skip := 1

	if a.tag != "" {
		list = list.AddItem(fmt.Sprintf("[red:black:]%s", a.tag), "", rune('#'), func() {
			a.show(Tags)
		})
		skip++
	}

	list.AddItem("Random", "", rune('*'), func() {
		r := rand.Intn(len(stations))

		for len(stations) > 1 && a.player.info.Url == stations[r].url {
			r = rand.Intn(len(stations))
		}

		list.SetCurrentItem(r + skip)
		go a.togglePlay(stations[r])
	})

	for i := 0; i < len(stations); i++ {
		list = list.AddItem(stations[i].title, "", idxToRune(i), func() {
			go a.togglePlay(stations[i])
		})

		if a.player.info.Url == stations[i].url {
			list.SetCurrentItem(i + skip)
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
		// Refresh favorites display if currently showing favorites
		if a.tag == favoritesTag {
			a.filterStationsForSelectedTag()
			// Find and select the station that was just played
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
				// Account for (#) Favorites and (*) Random top items
				a.stationsList.SetCurrentItem(i + 2)
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
