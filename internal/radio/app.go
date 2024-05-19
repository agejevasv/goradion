package radio

import (
	"fmt"
	"math/rand"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const helpString = `Keyboard Control

	[green]*[default]
		Toggle playing a random station.

	[green]a[default]-[green]z[default]
		Toggle playing a station marked with a given letter.

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
		Toggle help.`

func NewApp(player *Player, stations, urls []string) *tview.Application {
	list := tview.NewList()
	list.ShowSecondaryText(false)
	list.SetBackgroundColor(tcell.ColorDefault)
	list.SetSelectedStyle(tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorGreen))
	list.SetMainTextStyle(tcell.StyleDefault.Foreground(tcell.ColorDefault).Background(tcell.ColorDefault))
	list.SetShortcutStyle(tcell.StyleDefault.Foreground(tcell.ColorDefault).Background(tcell.ColorDefault))

	list.AddItem("Random", "", rune('*'), func() {
		r := rand.Intn(len(stations))
		for list.GetCurrentItem() == r+1 {
			r = rand.Intn(len(stations))
		}
		list.SetCurrentItem(r + 1)
		go player.Toggle(stations[r], urls[r])
	})

	for i := 0; i < len(stations); i++ {
		list = list.AddItem(stations[i], "", idxToRune(i), func() {
			go player.Toggle(stations[i], urls[i])
		})
	}

	status := tview.NewTextView()
	status.SetTextColor(tcell.ColorLightGray)
	status.SetDynamicColors(true)
	status.SetText("Ready [gray]| [green]Press ? for help")

	volume := tview.NewTextView()
	volume.SetTextColor(tcell.ColorLightGray)
	volume.SetTextAlign(tview.AlignRight)

	statusFlex := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(status, 0, 100, true).
		AddItem(volume, 0, 5, false)

	flex := tview.NewFlex().
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(list, 0, 100, true).
			AddItem(statusFlex, 0, 1, true), 0, 1, true)

	help := tview.NewTextView()
	help.SetDynamicColors(true)
	help.SetBackgroundColor(tcell.ColorDefault)
	help.SetText(fmt.Sprintf("[green]%s\n\n[default]%s", VersionString(), helpString))

	currentPage := "Main"
	pages := tview.NewPages()
	pages.AddPage("Main", flex, true, true)
	pages.AddPage("Help", help, true, true)
	pages.SwitchToPage(currentPage)

	app := tview.NewApplication()
	app.SetRoot(pages, true)

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
			if inf.Song == "" && inf.Status == "" {
				status.SetText(inf.Station)
			} else if inf.Song == "" {
				status.SetText(fmt.Sprintf("%s [gray]| [green]%s", inf.Station, inf.Status))
			} else {
				status.SetText(fmt.Sprintf("%s [gray]| [green]%s", inf.Station, stripBraces(inf.Song)))
			}
			volume.SetText(fmt.Sprintf("%d%%", inf.Volume))
			app.Draw()
		}
	}()

	return app
}

func stripBraces(s string) string {
	s = strings.ReplaceAll(s, "[", "(")
	return strings.ReplaceAll(s, "]", ")")
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
