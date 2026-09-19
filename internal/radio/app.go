package radio

import (
	"time"

	"github.com/agejevasv/goradion/internal/logging"
	"github.com/agejevasv/goradion/internal/mpv"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type Option func(*Application)

// WithASCII(false) keeps the automatic choice made by detectASCII.
func WithASCII(on bool) Option {
	return func(a *Application) { a.ascii = a.ascii || on }
}

// WithRemote starts the remote control server together with the TUI. A nil
// key keeps the configured one, an empty one means no code at all.
func WithRemote(key *string) Option {
	return func(a *Application) {
		a.remoteAutostart = true
		if key != nil {
			a.remoteKey = key
		}
	}
}

// WithRemotePort overrides the configured preferred port; a free port is used
// if it is taken.
func WithRemotePort(port int) Option {
	return func(a *Application) { a.remotePort = port }
}

// Application is the TUI. Its fields belong to the UI goroutine: other
// goroutines reach them through queueUpdate or queueUpdateDraw.
type Application struct {
	player    *mpv.Player
	stations  []Station // the stations list, never modified
	tags      []string  // of the stations, sorted
	bookmarks *Bookmarks
	config    *config

	app         *tview.Application
	stopped     chan struct{} // closed once Run's event loop is over
	pages       *tview.Pages
	pageHistory []page
	tagsFlex    *tview.Flex
	tagsRow     *tview.Flex // the panes of tagsFlex when wide
	mainRow     *tview.Flex // and of mainFlex
	mainFlex    *tview.Flex
	helpFlex    *tview.Flex
	helpView    *tview.TextView
	card        *nowPlaying
	hints       *hintBar
	ascii       bool
	wide        bool
	layoutReady bool
	termBg      tcell.Color // the terminal default background set by syncTermBg

	tagsList    *tview.List
	tagsPane    *listPane
	tagRows     []tagRef // what each row opens
	tagCounts   map[tagRef]int
	syncingTags bool // suppresses the tags list's changed callback

	tag               tagRef    // the tag or search shown, zero for none
	listed            []Station // the stations in the stations list
	playing           Station   // the station last started
	playingTag        tagRef
	stationsList      *tview.List
	stationsPane      *listPane
	stationRows       []string // the station URL of each row, empty for the others
	stationsBackLink  bool
	emptyStationsText string

	searchModal      *tview.Flex
	searchContent    *tview.Flex
	searchInput      *tview.InputField
	searchResults    *tview.List
	searchShown      searchShown // what the results list shows
	searchOnline     bool
	searchGeneration int
	lastSearch       searchView

	shuffle shuffle
	sleep   sleepTimer

	remote          *Remote
	remotePort      int
	remoteAutostart bool
	remoteKey       *string // nil: a random code per run
	remoteModal     *tview.Flex
	remoteQR        *tview.TextView
	remoteText      *tview.TextView

	themeList *tview.List
}

// tagRef names a list of stations: a tag, All Stations, Bookmarks, or the last
// search, which may be named like any of them.
type tagRef struct {
	name   string
	search bool
}

// searchView is the last search shown in the stations list.
type searchView struct {
	query    string
	stations []Station
	online   bool
}

func NewApp(player *mpv.Player, stations []Station, options ...Option) *Application {
	a := &Application{
		player:    player,
		stations:  stations,
		tags:      collectTags(stations),
		bookmarks: NewBookmarks(stations),
		config:    loadConfig(),
		shuffle:   shuffle{interval: defaultShuffleInterval, fade: shuffleFade},
		sleep:     sleepTimer{fade: sleepFade},
		ascii:     detectASCII(),
		stopped:   make(chan struct{}),
	}
	a.remoteAutostart, a.remotePort, a.remoteKey = a.config.Remote.Autostart, a.config.Remote.Port, a.config.Remote.Key
	for _, option := range options {
		option(a)
	}
	applyGlyphs(a.ascii)
	t, ok := findTheme(a.config.Theme)
	if !ok {
		logging.Printf("config: unknown theme %q, using %q", a.config.Theme, t.name)
	}
	useTheme(t)

	a.setupPages()
	a.setupSearchModal()
	a.setupRemoteModal()
	a.setupThemeModal()
	a.restoreSession()

	a.app = tview.NewApplication().
		SetRoot(a.pages, true).
		EnableMouse(true).
		SetMouseCapture(a.mouseCapture).
		SetInputCapture(a.handleKey).
		SetBeforeDrawFunc(a.beforeDraw)
	return a
}

func (a *Application) Run() error {
	defer a.stopRemote()
	// Deferred first to run after stopShuffle, which restores a faded volume.
	defer a.saveSession()
	if a.remoteAutostart {
		if _, err := a.startRemote(); err != nil {
			// The modal retries and shows the error.
			a.showRemoteModal()
		}
	}
	defer a.restoreTermBg()
	defer a.stopShuffle()
	defer a.cancelSleep()

	defer close(a.stopped)
	go a.stopOnSignal()
	go a.animate()
	go a.followPlayer()
	return a.app.Run()
}

// queueUpdate is tview's QueueUpdate, except that it reports false rather
// than block for good once the TUI has exited, when nothing runs queued
// updates. It reports whether f ran.
func (a *Application) queueUpdate(f func()) bool {
	return a.queue(a.app.QueueUpdate, f)
}

// queueUpdateDraw is queueUpdate followed by a draw.
func (a *Application) queueUpdateDraw(f func()) bool {
	return a.queue(a.app.QueueUpdateDraw, f)
}

func (a *Application) queue(enqueue func(func()) *tview.Application, f func()) bool {
	done := make(chan struct{})
	// After the exit the enqueueing goroutine is stuck instead of the caller.
	go enqueue(func() {
		f()
		close(done)
	})
	select {
	case <-done:
		return true
	case <-a.stopped:
		return false
	}
}

// followPlayer shows every player state change on the card.
func (a *Application) followPlayer() {
	for {
		changed := a.player.Changed()
		inf := a.player.Snapshot()
		if !a.queueUpdateDraw(func() { a.card.update(inf, time.Now()) }) {
			return
		}
		select {
		case <-changed:
		case <-a.stopped:
			return
		}
	}
}

func (a *Application) setupPages() {
	a.card = newNowPlaying(a.player, &a.shuffle)
	a.card.bookmarked = a.bookmarks.has
	a.card.sleep = &a.sleep
	a.card.cancelSleep = a.cancelSleep
	a.hints = newHintBar(a)

	a.tagsList = newList()
	a.tagsList.SetTitle(" Tags ")
	a.tagsPane = newListPane(a.tagsList)
	a.tagsPane.overlay = a.drawTagCounts
	a.tagsList.SetChangedFunc(func(index int, _, _ string, _ rune) {
		if a.wide && !a.syncingTags && index < len(a.tagRows) {
			a.loadTag(a.tagRows[index])
		}
	})
	a.showTags()

	a.stationsList = newList()
	a.stationsPane = newListPane(a.stationsList)
	a.stationsPane.overlay = a.drawStationMarks
	a.showStations(a.stations)

	a.helpView = tview.NewTextView().SetDynamicColors(true).SetScrollable(true).SetWordWrap(true)
	a.helpView.SetText(helpText())
	a.helpView.SetBorder(true).SetBorderPadding(0, 0, 1, 1).
		SetTitle(" Help ").SetTitleAlign(tview.AlignLeft).SetTitleColor(colorText)

	a.tagsFlex, a.mainFlex, a.helpFlex = tview.NewFlex(), tview.NewFlex(), tview.NewFlex()
	a.buildLayout(false)

	a.pages = tview.NewPages()
	a.addPage(pageTags, a.tagsFlex, true)
	a.addPage(pageMain, a.mainFlex, false)
	a.addPage(pageHelp, a.helpFlex, false)
	a.pageHistory = append(a.pageHistory, pageTags)
}

func newList() *tview.List {
	list := tview.NewList()
	list.ShowSecondaryText(false)
	colorList(list)
	return list
}
