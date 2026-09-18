package radio

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// The theme modal previews the theme under the cursor on the whole UI. Enter
// saves it, Esc goes back to the saved one.

const themeNameWidth = 16

func (a *Application) setupThemeModal() {
	a.themeList = newList()
	a.themeList.SetBorder(true).SetBorderPadding(0, 0, 1, 1).
		SetTitle(" Theme ").SetTitleAlign(tview.AlignLeft)
	a.themeList.SetHighlightFullLine(true).SetWrapAround(false)
	for i, t := range themes {
		a.themeList.AddItem(t.name, "", idxToRune(i), func() { a.saveTheme(t.name) })
	}
	a.themeList.SetChangedFunc(func(index int, _, _ string, _ rune) {
		a.setTheme(themes[index])
	})
	a.themeList.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEscape {
			a.cancelThemeModal()
			return nil
		}
		return event
	})

	width := 4 + 2 + themeNameWidth + 2 + 4*2 + 4 // shortcut, mark, name, gap, swatches, frame
	modal := centerIn(tview.NewFlex(), a.themeList, width, len(themes)+2)
	a.pages.AddPage(a.pageNames[ThemePage], modal, true, false)
}

// themeLabel shows the theme's own colours, so themes can be compared without
// previewing each one.
func themeLabel(t theme, saved bool) string {
	mark := "  "
	if saved {
		mark = glyphs.check + " "
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s%-*s  ", mark, themeNameWidth, t.name)
	for _, c := range []tcell.Color{t.accent, t.bright, t.warn, t.danger} {
		sb.WriteString(fgTag(c) + glyphs.swatch)
	}
	sb.WriteString("[-]")
	return sb.String()
}

func (a *Application) showThemeModal() {
	a.hideSearchModal()
	a.hideRemoteModal()
	current := 0
	for i, t := range themes {
		saved := t.name == a.config.Theme
		if saved {
			current = i
		}
		a.themeList.SetItemText(i, themeLabel(t, saved), "")
	}
	a.themeList.SetCurrentItem(current)
	a.pages.ShowPage(a.pageNames[ThemePage])
	a.app.SetFocus(a.themeList)
}

func (a *Application) hideThemeModal() {
	a.pages.HidePage(a.pageNames[ThemePage])
}

func (a *Application) cancelThemeModal() {
	t, _ := findTheme(a.config.Theme)
	a.setTheme(t)
	a.hideThemeModal()
}

func (a *Application) saveTheme(name string) {
	a.config.Theme = name
	if err := a.config.save(); err != nil {
		log.Printf("config: %v", err)
	}
	a.hideThemeModal()
}

func (a *Application) isThemeModalOpen() bool {
	return a.frontPage() == a.pageNames[ThemePage]
}
