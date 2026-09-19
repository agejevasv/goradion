package radio

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type hint struct{ key, label string }

type hintBar struct {
	*tview.Box
	app *Application
}

func newHintBar(a *Application) *hintBar {
	return &hintBar{Box: tview.NewBox(), app: a}
}

func (h *hintBar) Draw(screen tcell.Screen) {
	h.DrawForSubclass(screen, h)
	x, y, w, _ := h.GetInnerRect()
	drawSegs(screen, x+1, y, w-1, hintSegs(h.app.currentHints(), w-1))
}

// hintSegs draws the keys as keycaps on the cursor's surface; the terminal
// theme, which has none, keeps them bold.
func hintSegs(hints []hint, width int) []seg {
	gap, key := "   ", func(k string) seg { return seg{k, styleKey} }
	if colorSurface != tcell.ColorDefault {
		gap = "  "
		key = func(k string) seg { return seg{" " + k + " ", styleKey.Background(colorSurface)} }
	}
	var out []seg
	used := 0
	for i, h := range hints {
		part := []seg{key(h.key), {" " + h.label, styleDim}}
		w := segsWidth(part)
		if i > 0 {
			w += len(gap)
		}
		if used+w > width {
			break
		}
		if i > 0 {
			out = append(out, seg{gap, styleText})
		}
		out = append(out, part...)
		used += w
	}
	return out
}

func (a *Application) currentHints() []hint {
	switch a.frontPage() {
	case pageSearch:
		if a.searchOnline {
			return []hint{{"enter", "search"}, {glyphs.down, "results"}, {"^F", "search local"}, {"^B", "bookmark"}, {"esc", "close"}}
		}
		return []hint{{"enter", "show all"}, {glyphs.down, "results"}, {"^F", "search online"}, {"^B", "bookmark"}, {"esc", "close"}}
	case pageRemote:
		return []hint{{"esc", "close"}}
	case pageTheme:
		return []hint{{glyphs.up + " " + glyphs.down, "preview"}, {"enter", "apply"}, {"esc", "cancel"}}
	case pageHelp:
		return []hint{{"esc", "back"}, {glyphs.up + " " + glyphs.down, "scroll"}}
	case pageMain:
		hints := []hint{{"^B", "bookmark"}, {"^F", "search"}, {"^R", "shuffle"}, {"^Z", "sleep"}, {"^P", "phone"}, {"^T", "theme"}, {"?", "help"}}
		if !a.wide {
			hints = append(hints, hint{"esc", "tags"})
		}
		return hints
	default:
		return []hint{{"^F", "search"}, {"^R", "shuffle"}, {"^Z", "sleep"}, {"^P", "phone"}, {"^T", "theme"}, {"?", "help"}, {"esc", "quit"}}
	}
}
