package radio

import (
	"context"
	"fmt"
	"math/rand"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const (
	favoritesTag = "Favorites"
	fadeDuration = 2 * time.Second
)

type Option func(*Application)

// WithASCII(false) keeps the automatic choice made by detectASCII.
func WithASCII(on bool) Option {
	return func(a *Application) { a.ascii = a.ascii || on }
}

func helpText() string {
	arrows := glyphs.left + " " + glyphs.right + " - +"
	sections := []struct {
		title string
		keys  [][2]string
	}{
		{"Playing", [][2]string{
			{"a-z A-Z", "play or stop the station with that letter"},
			{"Enter Space", "play or stop the station under the cursor"},
			{"*", "play a random station from the list"},
			{arrows, "volume down or up"},
			{"Ctrl+R", "shuffle: a random station every few minutes"},
			{"Alt+1 to 9", "shuffle interval in minutes"},
		}},
		{"Finding stations", [][2]string{
			{"/ #", "tags"},
			{"~", "all stations"},
			{"Ctrl+F :", "search your stations; press again to search online"},
			{"Ctrl+S", "search online at radio-browser.info"},
			{glyphs.up + " " + glyphs.down + " PgUp PgDn", "move through the list"},
			{"Tab", "switch between tags and stations (wide terminals)"},
			{"Esc", "go back; quits from the tags list"},
		}},
		{"More", [][2]string{
			{"Ctrl+P", "control goradion from your phone"},
			{"?", "this help"},
			{"Mouse", "click a station to play it, scroll lists; scroll or click the volume gauge"},
		}},
		{"Command line", [][2]string{
			{"-ascii", "plain ASCII symbols for terminals without Unicode fonts"},
			{"-no-vu", "hide the audio level meter"},
			{"-s file|url", "stations CSV to use"},
			{"-p port", "preferred port for the phone remote"},
		}},
	}
	var sb strings.Builder
	sb.WriteString("[green::b]" + VersionString() + "[-::-]\n")
	for _, sec := range sections {
		sb.WriteString("\n[::b]" + sec.title + "[::-]\n")
		for _, k := range sec.keys {
			sb.WriteString(fmt.Sprintf("  [green]%-15s[-] %s\n", k[0], k[1]))
		}
	}
	return sb.String()
}

type Page int

const (
	Main = iota
	Help
	Tags
	Search
	RemotePage
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
	stationsPane            *listPane
	tagsPane                *listPane
	tagsFlex                *tview.Flex
	mainFlex                *tview.Flex
	helpFlex                *tview.Flex
	helpView                *tview.TextView
	card                    *nowPlaying
	hints                   *hintBar
	favorites               *Favorites
	searchModal             *tview.Flex
	searchContent           *tview.Flex
	searchInput             *tview.InputField
	searchResults           *tview.List
	searchOnline            bool
	searchGeneration        int
	lastOnlineStations      []Station
	timedRandomActive       bool
	timedRandomCancel       context.CancelFunc
	shuffleIterationStartAt time.Time
	shuffleInterval         time.Duration
	waitingForPlayback      chan struct{}
	waitingForURL           string
	remote                  *Remote
	remotePort              int
	remoteModal             *tview.Flex
	remoteQR                *tview.TextView
	remoteText              *tview.TextView

	ascii             bool
	wide              bool
	layoutReady       bool
	syncingTags       bool // suppresses the tags list's changed callback
	tagRows           []string
	stationRows       []string
	stationsBackLink  bool
	emptyStationsText string
}

// NewApp creates the TUI. remotePort is the preferred port for the remote
// control server started with Ctrl+P; a free port is used if it is taken.
func NewApp(player *Player, stations []Station, remotePort int, options ...Option) *Application {
	a := &Application{
		player:          player,
		stations:        stations,
		pageNames:       []string{"Main", "Help", "Tags", "Search", "Remote"},
		favorites:       NewFavorites(stations),
		shuffleInterval: 5 * time.Minute,
		remotePort:      remotePort,
		ascii:           detectASCII(),
	}
	for _, option := range options {
		option(a)
	}
	applyTheme(a.ascii)

	a.setupPages()
	a.card.info.Volume = player.info.Volume
	a.setupSearchModal()
	a.setupRemoteModal()

	a.app = tview.NewApplication().
		SetRoot(a.pages, true).
		EnableMouse(true).
		SetMouseCapture(a.mouseCapture).
		SetInputCapture(a.inputCapture()).
		SetBeforeDrawFunc(a.beforeDraw)

	go a.updateStatus()

	return a
}

func (a *Application) Run() error {
	defer a.stopRemote()
	stop := make(chan struct{})
	defer close(stop)
	go a.animate(stop)
	return a.app.Run()
}

func (a *Application) setupPages() {
	a.card = newNowPlaying(a)
	a.hints = newHintBar(a)

	a.tagsList = newList()
	a.tagsList.SetTitle(" Tags ")
	a.tagsPane = newListPane(a.tagsList)
	a.tagsList.SetChangedFunc(func(index int, _, _ string, _ rune) {
		if a.wide && !a.syncingTags && index < len(a.tagRows) && a.tagRows[index] != "" {
			a.loadTag(a.tagRows[index])
		}
	})
	a.setupTagsList()

	a.stationsList = newList()
	a.stationsPane = newListPane(a.stationsList)
	a.stationsPane.overlay = a.drawStationMarks
	a.setupStationsList(a.stationsList, a.stations)

	a.helpView = tview.NewTextView().SetDynamicColors(true).SetScrollable(true).SetWordWrap(true)
	a.helpView.SetText(helpText())
	a.helpView.SetBorder(true).SetBorderPadding(0, 0, 1, 1).
		SetTitle(" Help ").SetTitleAlign(tview.AlignLeft).SetTitleColor(colorAccent)

	a.tagsFlex, a.mainFlex, a.helpFlex = tview.NewFlex(), tview.NewFlex(), tview.NewFlex()
	a.buildLayout(false)

	a.pages = tview.NewPages().
		AddPage(a.pageNames[Tags], a.tagsFlex, true, true).
		AddPage(a.pageNames[Main], a.mainFlex, true, false).
		AddPage(a.pageNames[Help], a.helpFlex, true, false)
	a.pageHistory = append(a.pageHistory, Tags)
}

func (a *Application) show(page Page) {
	a.pageHistory = append(a.pageHistory, page)

	if len(a.pageHistory) > 2 {
		a.pageHistory = a.pageHistory[len(a.pageHistory)-2:]
	}

	a.pages.SwitchToPage(a.pageNames[page])

	if a.wide && page == Tags && a.tag == "" {
		a.previewTagAtCursor()
	} else if a.wide && (page == Tags || page == Main) {
		a.syncTagCursor()
	}
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
			if currentPage == a.pageNames[Search] {
				return event
			}

			if currentPage == a.pageNames[RemotePage] {
				a.hideRemoteModal()
				return nil
			}

			if currentPage == a.pageNames[Tags] {
				a.app.Stop()
				return nil
			}

			if currentPage == a.pageNames[Main] {
				if !a.wide {
					a.tag = ""
				}
				a.show(Tags)
				return nil
			}

			if !closeHelp() {
				a.show(Tags)
			}
			return nil
		case tcell.KeyTab, tcell.KeyBacktab:
			if a.wide && currentPage == a.pageNames[Tags] {
				a.show(Main)
				return nil
			}
			if a.wide && currentPage == a.pageNames[Main] {
				a.show(Tags)
				return nil
			}
			return event
		case tcell.KeyCtrlF:
			a.showSearchModal(false)
			return nil
		case tcell.KeyCtrlS:
			a.showSearchModal(true)
			return nil
		case tcell.KeyCtrlR:
			go a.toggleTimedRandom()
			return nil
		case tcell.KeyCtrlP:
			if currentPage == a.pageNames[Search] {
				return event
			}
			a.toggleRemoteModal()
			return nil
		case tcell.KeyLeft:
			a.card.flash(time.Now())
			a.player.VolumeDn()
			return nil
		case tcell.KeyRight:
			a.card.flash(time.Now())
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
				a.card.flash(time.Now())
				a.player.VolumeUp()
				return nil
			case '-', '_':
				a.card.flash(time.Now())
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
				a.openTag(allStationsTag)
				return nil
			case ':':
				if a.isSearchModalOpen() {
					return event
				}
				a.showSearchModal(false)
				return nil
			}
		}
		return event
	}
}

// The player sends on Info while holding its lock, so only the latest
// snapshot is queued for the UI loop and the player never waits for it.
func (a *Application) updateStatus() {
	var mu sync.Mutex
	var latest Info
	pending := make(chan struct{}, 1)

	go func() {
		for range pending {
			mu.Lock()
			inf := latest
			mu.Unlock()
			a.app.QueueUpdateDraw(func() {
				a.card.update(inf, time.Now())
			})
		}
	}()

	for inf := range a.player.Info {
		if a.waitingForPlayback != nil && inf.Url == a.waitingForURL && (inf.Status == "Playing" || inf.Song != "") {
			close(a.waitingForPlayback)
			a.waitingForPlayback = nil
			a.waitingForURL = ""
		}

		mu.Lock()
		latest = inf
		mu.Unlock()
		select {
		case pending <- struct{}{}:
		default:
		}
	}
}

func (a *Application) setupStationsList(list *tview.List, stations []Station) *tview.List {
	list.Clear()
	list.SetCurrentItem(0)

	a.stationsBackLink = a.tag != "" && !a.wide
	offset := a.calculateStationListOffset()
	rows := make([]string, 0, len(stations)+2)

	if a.stationsBackLink {
		list.AddItem(glyphs.back+" "+tview.Escape(a.tag), "", rune('#'), func() {
			a.tag = ""
			a.show(Tags)
		})
		rows = append(rows, "")
	}

	if len(stations) > 0 {
		list.AddItem("  Random", "", rune('*'), func() {
			r := a.randomIndex(stations)
			list.SetCurrentItem(r + offset)
			go a.togglePlayManual(stations[r])
		})
		rows = append(rows, "")
	}

	for i := range stations {
		list.AddItem("  "+stations[i].title, "", idxToRune(i), func() {
			go a.togglePlayManual(stations[i])
		})
		rows = append(rows, stations[i].url)

		if a.player.info.Url == stations[i].url {
			list.SetCurrentItem(i + offset)
		}
	}

	a.emptyStationsText = ""
	if len(stations) == 0 {
		a.emptyStationsText = "No stations"
		if a.tag == favoritesTag {
			a.emptyStationsText = "No favourites yet " + glyphs.dot + " stations you play land here"
		}
	}

	a.stationRows = rows
	a.setStationsTitle(len(stations))
	return list
}

func (a *Application) setStationsTitle(count int) {
	name, icon := a.tag, glyphs.notes
	switch a.tag {
	case "":
		name = allStationsTag
	case favoritesTag:
		icon = glyphs.star
	}
	if a.tag != "" && a.tag == a.lastSearchTag && a.lastOnlineStations != nil {
		name += " (online)"
	}
	unit := "stations"
	if count == 1 {
		unit = "station"
	}
	a.stationsList.SetTitle(fmt.Sprintf(" %s %s %s %d %s ", icon, tview.Escape(name), glyphs.dot, count, unit))
}

func (a *Application) setupTagsList() {
	a.syncingTags = true
	defer func() { a.syncingTags = false }()

	list := a.tagsList
	cursor := list.GetCurrentItem()
	list.Clear()
	rows := make([]string, 0)
	add := func(label string, shortcut rune, tag string) {
		list.AddItem(label, "", shortcut, func() {
			a.openTag(tag)
		})
		rows = append(rows, tag)
	}

	add(favoritesTag, '$', favoritesTag)
	add(allStationsTag, '~', allStationsTag)
	for i, tag := range tags(a.stations) {
		add(tview.Escape(tag), idxToRune(i), tag)
	}
	if a.lastSearchTag != "" {
		label := tview.Escape(a.lastSearchTag)
		if a.lastOnlineStations != nil {
			label += " [gray](online)[-]"
		}
		add(label, '^', a.lastSearchTag)
	}

	a.tagRows = rows
	list.SetCurrentItem(cursor)
}

// loadTag is openTag without switching pages.
func (a *Application) loadTag(tag string) bool {
	switch {
	case tag == a.lastSearchTag && tag != "":
		var matchedStations []Station
		if a.lastOnlineStations != nil {
			matchedStations = a.lastOnlineStations
		} else {
			matchedStations = a.filterStations(a.lastSearchTag)
		}
		a.tag = a.lastSearchTag
		a.setupStationsList(a.stationsList, matchedStations)
	case tag == favoritesTag, tag == allStationsTag, slices.Contains(tags(a.stations), tag):
		a.tag = tag
		a.filterStationsForSelectedTag()
	default:
		return false
	}
	return true
}

func (a *Application) openTag(tag string) bool {
	if !a.loadTag(tag) {
		return false
	}
	a.show(Main)
	return true
}

// randomIndex picks a station index, avoiding the one playing when possible.
func (a *Application) randomIndex(stations []Station) int {
	r := rand.Intn(len(stations))
	for len(stations) > 1 && a.player.info.Url == stations[r].url {
		r = rand.Intn(len(stations))
	}
	return r
}

func (a *Application) togglePlay(station Station) {
	if station.url != "" && station.url != a.player.info.Url {
		a.favorites.track(station)
		if a.tag == favoritesTag {
			// togglePlay runs off the UI goroutine.
			a.app.QueueUpdateDraw(func() {
				a.filterStationsForSelectedTag()
				a.findAndSelectStation(station.url)
			})
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

	if a.tag == allStationsTag {
		return a.stations
	}

	if a.tag == a.lastSearchTag && a.lastSearchTag != "" {
		if a.lastOnlineStations != nil {
			return a.lastOnlineStations
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
			if a.tag == allStationsTag || slices.Contains(a.stations[i].tags, a.tag) {
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
	if a.stationsBackLink {
		return 2
	}
	return 1
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
	a.setupTagsList()
	if a.wide {
		a.syncTagCursor()
	}
}

func newList() *tview.List {
	list := tview.NewList()
	list.ShowSecondaryText(false)
	list.SetBackgroundColor(tcell.ColorDefault)
	list.SetSelectedStyle(styleSelected)
	list.SetMainTextStyle(styleText)
	list.SetShortcutStyle(styleDim)
	return list
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

	sort.Strings(tags)
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
