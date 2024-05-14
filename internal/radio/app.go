package radio

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const helpString = `Keyboard Control

	[green]a[white]-[green]z[white]
		Toggle playing a station marked with a given letter.

	[green]Enter[white] and [green]Space[white]
		Toggle playing currently selected station.

	[green]Left[white] and [green]Right[white], [green]-[white] and [green]+[white]
		Change the volume in increments of 5.

	[green]Up[white] and [green]Down[white]
		Cycle through the radio station list.

	[green]PgUp[white] and [green]PgDown[white]
		Jump to a beginning/end of a station list.

	[green]Esc[white][white]
		Close current window.

	[green]?[white][white]
		Toggle help.`

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
	status.SetText("Ready [gray]| [green]Press ? for help")

	volume := tview.NewTextView()
	volume.SetBackgroundColor(tcell.ColorBlack)
	volume.SetTextColor(tcell.ColorLightGray)
	volume.SetTextAlign(tview.AlignRight)

	flex := tview.NewFlex().
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(list, 0, 100, true).
			AddItem(tview.NewFlex().SetDirection(tview.FlexColumn).
				AddItem(status, 0, 100, true).
				AddItem(volume, 0, 5, false), 0, 1, true), 0, 1, true)

	help := tview.NewTextView()
	help.SetDynamicColors(true)
	help.SetBackgroundColor(tcell.ColorDefault)
	help.SetText(fmt.Sprintf("[green]%s\n\n[white]%s", VersionString(), helpString))

	currentPage := "Main"
	pages := tview.NewPages()
	pages.AddPage("Main", flex, true, true)
	pages.AddPage("Help", help, true, true)
	pages.SwitchToPage(currentPage)

	app := tview.NewApplication()
	app.SetRoot(pages, true).EnableMouse(true)

	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch key := event.Key(); key {
		case tcell.KeyEscape:
			if currentPage == "Help" {
				pages.SwitchToPage("Main")
				currentPage = "Main"
				return nil
			}
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
			case '?':
				if currentPage == "Help" {
					pages.SwitchToPage("Main")
					currentPage = "Main"
				} else {
					pages.SwitchToPage("Help")
					currentPage = "Help"
				}
				return nil
			}
		}
		return event
	})

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
