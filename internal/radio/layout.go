package radio

import (
	"time"

	"github.com/agejevasv/goradion/internal/mpv"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const (
	wideMinWidth  = 100
	tagsPaneWidth = 26
)

type listPane struct {
	*tview.List
	overlay func(screen tcell.Screen)
}

func newListPane(list *tview.List) *listPane {
	list.SetBorder(true).SetBorderPadding(0, 0, 1, 1)
	list.SetTitleAlign(tview.AlignLeft)
	list.SetHighlightFullLine(true)
	return &listPane{List: list}
}

func (p *listPane) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (bool, tview.Primitive) {
	return wheelList{p.List}.MouseHandler()
}

// wheelList keeps the wheel scrolling past the cursor. A list draws with its
// cursor in view, so the cursor is dragged along at the edges.
type wheelList struct{ *tview.List }

func (l wheelList) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (bool, tview.Primitive) {
	handle := l.List.MouseHandler()
	return func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (bool, tview.Primitive) {
		consumed, capture := handle(action, event, setFocus)
		if consumed && (action == tview.MouseScrollUp || action == tview.MouseScrollDown) {
			offset, _ := l.GetOffset()
			_, _, _, height := l.GetInnerRect()
			current := l.GetCurrentItem()
			l.SetCurrentItem(min(max(current, offset), offset+height-1))
		}
		return consumed, capture
	}
}

func (p *listPane) Draw(screen tcell.Screen) {
	if p.HasFocus() {
		p.SetBorderStyle(styleText.Foreground(colorBorder))
		p.SetTitleColor(colorText)
	} else {
		p.SetBorderStyle(styleDim)
		p.SetTitleColor(colorDim)
	}
	wheelList{p.List}.Draw(screen)
	if p.overlay != nil {
		p.overlay(screen)
	}
}

func (l wheelList) Draw(screen tcell.Screen) {
	focused := l.HasFocus()
	if focused {
		l.SetSelectedStyle(styleSelected)
	} else {
		l.SetSelectedStyle(styleSelectedUnfocused)
	}
	l.List.Draw(screen)
	drawCursorBar(screen, l.List, focused)
}

// drawCursorBar widens the cursor over the shortcut and the side padding, and
// gives it an edge: the accent while focused, dim otherwise.
func drawCursorBar(screen tcell.Screen, list *tview.List, focused bool) {
	x, y, width, height := list.GetInnerRect()
	offset, _ := list.GetOffset()
	row := list.GetCurrentItem() - offset
	if list.GetItemCount() == 0 || row < 0 || row >= height {
		return
	}
	y += row
	if focused {
		for cx := x - 1; cx <= x+width; cx++ {
			mainc, comb, cell, _ := screen.GetContent(cx, y)
			if colorSurface == tcell.ColorDefault {
				// Reversed, a dim shortcut would be a grey block.
				_, _, attrs := cell.Decompose()
				cell = styleText.Reverse(true).Bold(attrs&tcell.AttrBold != 0)
			} else {
				cell = cell.Background(colorSurface)
			}
			screen.SetContent(cx, y, mainc, comb, cell)
		}
	}
	if glyphs.edge == "" {
		return
	}
	_, _, cell, _ := screen.GetContent(x-1, y)
	edge := cell.Reverse(false).Foreground(colorDim)
	if focused {
		edge = edge.Foreground(colorAccent)
	}
	screen.SetContent(x-1, y, []rune(glyphs.edge)[0], nil, edge)
}

// centerIn fills outer with p, centred at a fixed size. On a shorter screen p
// gets the screen's height, so a list scrolls rather than running off it.
func centerIn(outer *tview.Flex, p tview.Primitive, width, height int) *tview.Flex {
	row := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(p, width, 0, true).
		AddItem(nil, 0, 1, false)
	outer.Clear().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(row, height, 0, true).
		AddItem(nil, 0, 1, false)
	outer.SetDrawFunc(func(_ tcell.Screen, x, y, w, h int) (int, int, int, int) {
		outer.ResizeItem(row, min(height, h), 0)
		return x, y, w, h
	})
	return outer
}

// When wide, both list pages hold the same panes and differ only in focus.
func (a *Application) buildLayout(wide bool) {
	a.wide = wide
	fill := func(page *tview.Flex, body tview.Primitive) {
		page.Clear().SetDirection(tview.FlexRow).
			AddItem(body, 0, 1, true).
			AddItem(a.card, cardHeight, 0, false).
			AddItem(a.hints, 1, 0, false)
	}
	if wide {
		fill(a.tagsFlex, tview.NewFlex().
			AddItem(a.tagsPane, tagsPaneWidth, 0, true).
			AddItem(a.stationsPane, 0, 1, false))
		fill(a.mainFlex, tview.NewFlex().
			AddItem(a.tagsPane, tagsPaneWidth, 0, false).
			AddItem(a.stationsPane, 0, 1, true))
	} else {
		fill(a.tagsFlex, a.tagsPane)
		fill(a.mainFlex, a.stationsPane)
	}
	fill(a.helpFlex, a.helpView)
}

func (a *Application) beforeDraw(screen tcell.Screen) bool {
	a.syncTermBg(screen)
	if colorBg != tcell.ColorDefault {
		// Flexes don't clear, so gaps between primitives would show the terminal.
		screen.Fill(' ', styleText)
	}
	width, _ := screen.Size()
	if wide := width >= wideMinWidth; wide != a.wide || !a.layoutReady {
		a.layoutReady = true
		a.buildLayout(wide)
		a.reloadStations()
		if wide {
			if a.isFront(pageTags) || a.tag == "" {
				a.previewTagAtCursor()
			} else {
				a.syncTagCursor()
			}
		}
	}
	return false
}

func (a *Application) previewTagAtCursor() {
	if i := a.tagsList.GetCurrentItem(); i >= 0 && i < len(a.tagRows) {
		a.loadTag(a.tagRows[i])
	}
}

func (a *Application) syncTagCursor() {
	tag := a.tag
	if tag == "" {
		tag = allStationsTag
	}
	for i, t := range a.tagRows {
		if t == tag {
			a.syncingTags = true
			a.tagsList.SetCurrentItem(i)
			a.syncingTags = false
			return
		}
	}
}

// drawStationMarks paints over the drawn list: colour tags in item texts would
// vanish under the cursor, and the spinner would need a rebuild every frame.
func (a *Application) drawStationMarks(screen tcell.Screen) {
	list := a.stationsList
	x, y, width, height := list.GetInnerRect()
	offset, _ := list.GetOffset()
	focused := a.stationsPane.HasFocus()
	textX := x + 4 // after the "(a) " shortcut column

	recolor := func(index int, glyph string, style tcell.Style) {
		row := index - offset
		if row < 0 || row >= height {
			return
		}
		// A reverse-video cursor would turn coloured text into a coloured bar.
		reversed := index == list.GetCurrentItem() && focused && colorSurface == tcell.ColorDefault
		fg, _, _ := style.Decompose()
		if glyph != "" {
			_, _, cell, _ := screen.GetContent(textX, y+row)
			if !reversed {
				cell = cell.Foreground(fg)
			}
			screen.SetContent(textX, y+row, []rune(glyph)[0], nil, cell)
		}
		if reversed {
			return
		}
		for cx := textX + 2; cx < x+width; cx++ {
			mainc, comb, cell, _ := screen.GetContent(cx, y+row)
			if cellFg, _, _ := cell.Decompose(); cellFg == colorText {
				screen.SetContent(cx, y+row, mainc, comb, cell.Foreground(fg))
			}
		}
	}

	if a.stationsBackLink {
		recolor(0, "", styleAccent)
	}
	if a.emptyStationsText != "" {
		if row := list.GetItemCount() - offset; row >= 0 && row < height {
			drawSegs(screen, textX+2, y+row, x+width-textX-2, []seg{{a.emptyStationsText, styleDim}})
		}
	}

	url, state := a.card.playingURL()
	if url == "" {
		return
	}
	for i, rowURL := range a.stationRows {
		if rowURL != url {
			continue
		}
		switch state {
		case mpv.Buffering:
			frame := spinnerFrame(time.Now())
			recolor(i, frame, styleText.Foreground(colorWarn))
		case mpv.Failed:
			recolor(i, glyphs.fail, styleText.Foreground(colorDanger))
		default:
			recolor(i, glyphs.play, styleAccent)
		}
		return
	}
}

// Capturing the mouse keeps terminals from turning the wheel into arrow keys.
// Unwanted actions become moves rather than nil: tview derives several actions
// from one terminal event, and a nil would cancel the ones after it.
func (a *Application) mouseCapture(event *tcell.EventMouse, action tview.MouseAction) (*tcell.EventMouse, tview.MouseAction) {
	if event == nil {
		return nil, action
	}
	switch action {
	case tview.MouseLeftClick, tview.MouseScrollUp, tview.MouseScrollDown:
	default:
		return event, tview.MouseMove
	}
	x, y := event.Position()
	switch page := a.frontPage(); page {
	case pageSearch:
		if !a.searchContent.InRect(x, y) {
			return event, tview.MouseMove
		}
	case pageRemote:
		return event, tview.MouseMove
	case pageTheme:
		if !a.themeList.InRect(x, y) {
			return event, tview.MouseMove
		}
	case pageTags, pageMain:
		if action != tview.MouseLeftClick || !a.wide {
			break
		}
		pane, to := a.stationsPane, pageMain
		if a.tagsPane.InRect(x, y) {
			pane, to = a.tagsPane, pageTags
		}
		if pane.InRect(x, y) && page != to {
			a.show(to)
			// The page just shown is laid out only when drawn, so it would miss
			// the click; the pane is shared and already in place.
			pane.MouseHandler()(action, event, func(p tview.Primitive) { a.app.SetFocus(p) })
			return nil, action
		}
	}
	return event, action
}
