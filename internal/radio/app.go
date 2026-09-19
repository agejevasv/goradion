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
// key means a random access code, an empty one means no code at all.
func WithRemote(key *string) Option {
	return func(a *Application) {
		a.remoteAutostart = true
		a.remoteKey = key
	}
}

// Application is the TUI. Its fields belong to the UI goroutine: other
// goroutines reach them through app.QueueUpdate or app.QueueUpdateDraw.
type Application struct {
	player    *mpv.Player
	stations  []Station // the stations list, never modified
	tags      []string  // of the stations, sorted
	favorites *Favorites
	config    *config

	app         *tview.Application
	pages       *tview.Pages
	pageHistory []page
	tagsFlex    *tview.Flex
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
	tagRows     []string // the tag of each row
	syncingTags bool     // suppresses the tags list's changed callback

	tag               string    // the tag or search shown, empty for none
	listed            []Station // the stations in the stations list
	stationsList      *tview.List
	stationsPane      *listPane
	stationRows       []string // the station URL of each row, empty for the others
	stationsBackLink  bool
	emptyStationsText string

	searchModal      *tview.Flex
	searchContent    *tview.Flex
	searchInput      *tview.InputField
	searchResults    *tview.List
	searchOnline     bool
	searchGeneration int
	lastSearch       searchView

	shuffle shuffle

	remote          *Remote
	remotePort      int
	remoteAutostart bool
	remoteKey       *string // nil: a random code per run
	remoteModal     *tview.Flex
	remoteQR        *tview.TextView
	remoteText      *tview.TextView

	themeList *tview.List
}

// searchView is the last search shown in the stations list.
type searchView struct {
	query    string
	stations []Station
	online   bool
}

// NewApp creates the TUI. remotePort is the preferred port for the remote
// control server started with Ctrl+P; a free port is used if it is taken.
func NewApp(player *mpv.Player, stations []Station, remotePort int, options ...Option) *Application {
	a := &Application{
		player:     player,
		stations:   stations,
		tags:       collectTags(stations),
		favorites:  NewFavorites(stations),
		config:     loadConfig(),
		shuffle:    shuffle{interval: defaultShuffleInterval, fade: shuffleFade},
		remotePort: remotePort,
		ascii:      detectASCII(),
	}
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
	if a.remoteAutostart {
		if _, err := a.startRemote(); err != nil {
			// The modal retries and shows the error.
			a.showRemoteModal()
		}
	}
	defer a.restoreTermBg()
	defer a.stopShuffle()

	stop := make(chan struct{})
	defer close(stop)
	go a.animate(stop)
	go a.followPlayer(stop)
	return a.app.Run()
}

// followPlayer shows every player state change on the card.
func (a *Application) followPlayer(stop <-chan struct{}) {
	for {
		changed := a.player.Changed()
		inf := a.player.Snapshot()
		a.app.QueueUpdateDraw(func() { a.card.update(inf, time.Now()) })
		select {
		case <-changed:
		case <-stop:
			return
		}
	}
}

func (a *Application) setupPages() {
	a.card = newNowPlaying(a.player, &a.shuffle)
	a.hints = newHintBar(a)

	a.tagsList = newList()
	a.tagsList.SetTitle(" Tags ")
	a.tagsPane = newListPane(a.tagsList)
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
