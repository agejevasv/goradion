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

func hintSegs(hints []hint, width int) []seg {
	const gap = "   "
	var out []seg
	used := 0
	for i, h := range hints {
		part := []seg{{h.key, styleKey}, {" " + h.label, styleDim}}
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
	arrows := glyphs.left + " " + glyphs.right
	switch a.frontPage() {
	case pageSearch:
		if a.searchOnline {
			return []hint{{"enter", "search"}, {glyphs.down, "results"}, {"^F", "search local"}, {"esc", "close"}}
		}
		return []hint{{"enter", "show all"}, {glyphs.down, "results"}, {"^F", "search online"}, {"esc", "close"}}
	case pageRemote:
		return []hint{{"esc", "close"}}
	case pageTheme:
		return []hint{{glyphs.up + " " + glyphs.down, "preview"}, {"enter", "apply"}, {"esc", "cancel"}}
	case pageHelp:
		return []hint{{"esc", "back"}, {glyphs.up + " " + glyphs.down, "scroll"}}
	case pageMain:
		tags := hint{"esc", "tags"}
		if a.wide {
			tags = hint{"tab", "tags"}
		}
		return []hint{{"a-z", "play"}, {"*", "random"}, {arrows, "volume"}, tags,
			{"^R", "shuffle"}, {"^F", "search"}, {"^T", "theme"}, {"^P", "phone"}, {"?", "help"}}
	default:
		hints := []hint{{"a-z", "open"}, {"~", "all"}, {arrows, "volume"}}
		if a.wide {
			hints = append(hints, hint{"tab", "stations"})
		}
		return append(hints, hint{"^R", "shuffle"}, hint{"^F", "search"}, hint{"^T", "theme"},
			hint{"^P", "phone"}, hint{"esc", "quit"}, hint{"?", "help"})
	}
}
