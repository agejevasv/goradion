package radio

import (
	"time"

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

func (p *listPane) Draw(screen tcell.Screen) {
	if p.HasFocus() {
		p.SetBorderStyle(styleText)
		p.SetTitleColor(colorAccent)
		p.SetSelectedStyle(styleSelected)
	} else {
		p.SetBorderStyle(styleDim)
		p.SetTitleColor(colorDim)
		p.SetSelectedStyle(styleSelectedUnfocused)
	}
	p.List.Draw(screen)
	if p.overlay != nil {
		p.overlay(screen)
	}
}

// centerIn fills outer with p, centred at a fixed size.
func centerIn(outer *tview.Flex, p tview.Primitive, width, height int) *tview.Flex {
	return outer.Clear().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().
			AddItem(nil, 0, 1, false).
			AddItem(p, width, 0, true).
			AddItem(nil, 0, 1, false), height, 0, true).
		AddItem(nil, 0, 1, false)
}

func (a *Application) frontPage() string {
	return a.pages.GetPageNames(true)[0]
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
			if a.frontPage() == a.pageNames[Tags] || a.tag == "" {
				a.previewTagAtCursor()
			} else {
				a.syncTagCursor()
			}
		}
	}
	return false
}

func (a *Application) reloadStations() {
	cursor := a.stationsList.GetCurrentItem() - a.calculateStationListOffset()
	a.setupStationsList(a.stationsList, a.getStationsFromCurrentView())
	if cursor >= 0 {
		a.stationsList.SetCurrentItem(cursor + a.calculateStationListOffset())
	}
}

func (a *Application) previewTagAtCursor() {
	if i := a.tagsList.GetCurrentItem(); i >= 0 && i < len(a.tagRows) && a.tagRows[i] != "" {
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
		underCursor := index == list.GetCurrentItem()
		if glyph != "" {
			_, _, cell, _ := screen.GetContent(textX, y+row)
			_, bg, _ := cell.Decompose()
			mark := style.Background(bg)
			if underCursor && focused {
				mark = mark.Foreground(colorOnAccent)
			}
			screen.SetContent(textX, y+row, []rune(glyph)[0], nil, mark)
		}
		if underCursor {
			return
		}
		fg, _, _ := style.Decompose()
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
		case stateBuffering:
			frame := spinnerFrame(time.Now())
			recolor(i, frame, styleText.Foreground(colorWarn))
		case stateFailed:
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
	case a.pageNames[Search]:
		if !a.searchContent.InRect(x, y) {
			return event, tview.MouseMove
		}
	case a.pageNames[RemotePage]:
		return event, tview.MouseMove
	case a.pageNames[ThemePage]:
		if !a.themeList.InRect(x, y) {
			return event, tview.MouseMove
		}
	case a.pageNames[Tags], a.pageNames[Main]:
		if action != tview.MouseLeftClick || !a.wide {
			break
		}
		pane, to := a.stationsPane, Page(Main)
		if a.tagsPane.InRect(x, y) {
			pane, to = a.tagsPane, Tags
		}
		if pane.InRect(x, y) && page != a.pageNames[to] {
			a.show(to)
			// The page just shown is laid out only when drawn, so it would miss
			// the click; the pane is shared and already in place.
			pane.MouseHandler()(action, event, func(p tview.Primitive) { a.app.SetFocus(p) })
			return nil, action
		}
	}
	return event, action
}
