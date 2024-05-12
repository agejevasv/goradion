package radio

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func NewApp(player *Player, stations, urls []string) *tview.Application {
	list := tview.NewList()
	list.ShowSecondaryText(false)
	list.SetBackgroundColor(tcell.ColorDefault)
	list.SetSelectedStyle(tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorGreen))
	list.SetMainTextStyle(tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorDefault))
	list.SetShortcutStyle(tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorDefault))

	for i := 0; i < len(stations); i++ {
		list = list.AddItem(stations[i], urls[i], idxToRune(i), func() {
			go player.Toggle(stations[i], urls[i])
		})
	}

	status := tview.NewTextView()
	status.SetBackgroundColor(tcell.ColorBlack)
	status.SetTextColor(tcell.ColorWhite)

	flex := tview.NewFlex().
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(list, 0, 100, true).
			AddItem(status, 0, 1, false), 0, 1, true)

	app := tview.NewApplication()
	app.SetRoot(flex, true).EnableMouse(true)

	go func() {
		for {
			status.SetText(<-player.OnAir)
			app.Draw()
		}
	}()

	return app
}

func idxToRune(i int) rune {
	if i+97 <= 122 {
		return rune(i + 97)
	}

	i -= 26

	if i+65 <= 90 {
		return rune(i + 65)
	}

	return 0
}
