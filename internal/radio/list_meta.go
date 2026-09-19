package radio

import (
	"slices"
	"strconv"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// The tags list shows how many stations each tag holds, in dim text on the
// right of the row.

// drawAside right-aligns text in a list row that ends at right, if it clears
// the row's label by two cells. The row keeps its background, so the text sits
// on the cursor bar too.
func drawAside(screen tcell.Screen, labelEnd, right, y int, text string) {
	width := tview.TaggedStringWidth(text)
	start := right - width
	if text == "" || start < labelEnd+2 {
		return
	}
	for i, r := range []rune(text) {
		_, _, cell, _ := screen.GetContent(start+i, y)
		if _, _, attrs := cell.Decompose(); attrs&tcell.AttrReverse == 0 {
			cell = cell.Foreground(colorDim).Bold(false)
		}
		screen.SetContent(start+i, y, r, nil, cell)
	}
}

// rowLabelEnd is the column after the label of a list's index-th item.
func rowLabelEnd(list *tview.List, index, textX int) int {
	main, _ := list.GetItemText(index)
	return textX + tview.TaggedStringWidth(main)
}

func (a *Application) countTags() {
	a.tagCounts = map[tagRef]int{}
	for _, s := range a.stations {
		for i, t := range s.tags {
			if !slices.Contains(s.tags[:i], t) { // a tag repeated in the CSV lists the station once
				a.tagCounts[tagRef{name: t}]++
			}
		}
	}
	if q := a.lastSearch.query; q != "" {
		a.tagCounts[tagRef{name: q, search: true}] = len(a.lastSearch.stations)
	}
}

func (a *Application) drawTagCounts(screen tcell.Screen) {
	list := a.tagsList
	x, y, width, height := list.GetInnerRect()
	offset, _ := list.GetOffset()
	for row := range height {
		i := offset + row
		if i >= len(a.tagRows) {
			return
		}
		count, ok := a.tagCounts[a.tagRows[i]]
		if a.tagRows[i] == (tagRef{name: bookmarksTag}) { // changes with Ctrl+B
			count, ok = len(a.bookmarks.list()), true
		}
		if !ok {
			continue
		}
		drawAside(screen, rowLabelEnd(list, i, x+4), x+width, y+row, strconv.Itoa(count))
	}
}
