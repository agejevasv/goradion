package radio

import (
	"fmt"

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
	status.SetTextColor(tcell.ColorLightGray)
	status.SetDynamicColors(true)
	status.SetText("Ready")

	volume := tview.NewTextView()
	volume.SetBackgroundColor(tcell.ColorBlack)
	volume.SetTextColor(tcell.ColorLightGray)
	volume.SetTextAlign(tview.AlignRight)
	volume.SetText(fmt.Sprintf("%d%%", player.status.Volume))

	flex := tview.NewFlex().
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(list, 0, 100, true).
			AddItem(tview.NewFlex().SetDirection(tview.FlexColumn).
				AddItem(status, 0, 100, true).
				AddItem(volume, 0, 5, false), 0, 1, true), 0, 1, true)

	app := tview.NewApplication()

	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch key := event.Key(); key {
		case tcell.KeyEscape:
			app.Stop()
		case tcell.KeyLeft:
			player.VolumeDn()
			return nil
		case tcell.KeyRight:
			player.VolumeUp()
			return nil
		case tcell.KeyRune:
			switch event.Rune() {
			case '=', '+':
				player.VolumeUp()
				return nil
			case '-', '_':
				player.VolumeDn()
				return nil
			}
		}
		return event
	})

	app.SetRoot(flex, true).EnableMouse(true)

	go func() {
		for inf := range player.Info {
			if inf.Song == "" {
				status.SetText(inf.Status)
			} else {
				status.SetText(fmt.Sprintf("%s [gray]| [green]%s", inf.Status, inf.Song))
			}
			volume.SetText(fmt.Sprintf("%d%%", inf.Volume))
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
