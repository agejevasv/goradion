package radio

import "github.com/rivo/tview"

type page string

const (
	pageTags   page = "Tags"
	pageMain   page = "Main"
	pageHelp   page = "Help"
	pageSearch page = "Search"
	pageRemote page = "Remote"
	pageTheme  page = "Theme"
)

func (a *Application) addPage(p page, item tview.Primitive, visible bool) {
	a.pages.AddPage(string(p), item, true, visible)
}

func (a *Application) frontPage() page {
	return page(a.pages.GetPageNames(true)[0])
}

func (a *Application) isFront(p page) bool {
	return a.frontPage() == p
}

func (a *Application) showModal(p page) {
	a.pages.ShowPage(string(p))
}

func (a *Application) hideModal(p page) {
	a.pages.HidePage(string(p))
}

// show switches to a full-screen page, closing any modal.
func (a *Application) show(p page) {
	if a.isFront(pageTheme) {
		a.cancelThemeModal()
	}
	a.pageHistory = append(a.pageHistory, p)
	if len(a.pageHistory) > 2 {
		a.pageHistory = a.pageHistory[len(a.pageHistory)-2:]
	}

	a.pages.SwitchToPage(string(p))

	if a.wide && p == pageTags && a.tag == "" {
		a.previewTagAtCursor()
	} else if a.wide && (p == pageTags || p == pageMain) {
		a.syncTagCursor()
	}
}

// closeHelp goes back to the page the help was opened from.
func (a *Application) closeHelp() bool {
	if !a.isFront(pageHelp) || len(a.pageHistory) < 2 {
		return false
	}
	previous := a.pageHistory[len(a.pageHistory)-2]
	if previous == pageHelp {
		return false
	}
	a.show(previous)
	return true
}
