package radio

import (
	"time"

	"github.com/agejevasv/goradion/internal/mpv"
	"github.com/gdamore/tcell/v2"
)

func (a *Application) handleKey(event *tcell.EventKey) *tcell.EventKey {
	front := a.frontPage()
	if front == pageTheme {
		if event.Key() == tcell.KeyCtrlT {
			a.cancelThemeModal()
			return nil
		}
		return event
	}
	if front == pageSearch {
		// Editing keys belong to the search field, and Esc closes it.
		switch event.Key() {
		case tcell.KeyRune, tcell.KeyLeft, tcell.KeyRight, tcell.KeyEscape, tcell.KeyCtrlP:
			return event
		}
	}

	switch event.Key() {
	case tcell.KeyEscape:
		switch front {
		case pageRemote:
			a.hideRemoteModal()
		case pageTags:
			a.app.Stop()
		case pageMain:
			if !a.wide {
				a.tag = ""
			}
			a.show(pageTags)
		default:
			if !a.closeHelp() {
				a.show(pageTags)
			}
		}
	case tcell.KeyTab, tcell.KeyBacktab:
		switch {
		case a.wide && front == pageTags:
			a.show(pageMain)
		case a.wide && front == pageMain:
			a.show(pageTags)
		default:
			return event
		}
	case tcell.KeyCtrlF:
		a.showSearchModal(false)
	case tcell.KeyCtrlS:
		a.showSearchModal(true)
	case tcell.KeyCtrlR:
		a.toggleShuffle()
	case tcell.KeyCtrlT:
		a.showThemeModal()
	case tcell.KeyCtrlP:
		a.toggleRemoteModal()
	case tcell.KeyLeft:
		a.changeVolume(-mpv.VolumeStep)
	case tcell.KeyRight:
		a.changeVolume(mpv.VolumeStep)
	case tcell.KeyRune:
		return a.handleRune(event)
	default:
		return event
	}
	return nil
}

func (a *Application) handleRune(event *tcell.EventKey) *tcell.EventKey {
	r := event.Rune()
	if event.Modifiers()&tcell.ModAlt != 0 && r >= '1' && r <= '9' {
		a.setShuffleInterval(int(r - '0'))
		return nil
	}
	switch r {
	case '=', '+':
		a.changeVolume(mpv.VolumeStep)
	case '-', '_':
		a.changeVolume(-mpv.VolumeStep)
	case '/', '#':
		a.show(pageTags)
	case '?':
		if !a.closeHelp() {
			a.show(pageHelp)
		}
	case '~':
		a.openTag(allStationsTag)
	case ':':
		a.showSearchModal(false)
	default:
		return event
	}
	return nil
}

// changeVolume flashes the gauge even when the volume is at its limit.
func (a *Application) changeVolume(delta int) {
	a.card.flash(time.Now())
	a.player.ChangeVolume(delta)
}
