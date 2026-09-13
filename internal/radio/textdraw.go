package radio

import (
	"fmt"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/uniseg"
)

// seg text is printed as is, not parsed for tview style tags, so stream titles
// with brackets survive.
type seg struct {
	text  string
	style tcell.Style
}

func segsWidth(segs []seg) int {
	w := 0
	for _, s := range segs {
		w += uniseg.StringWidth(s.text)
	}
	return w
}

func drawSegs(screen tcell.Screen, x, y, maxWidth int, segs []seg) int {
	used := 0
	for _, s := range segs {
		g := uniseg.NewGraphemes(s.text)
		for g.Next() {
			w := g.Width()
			if w == 0 {
				continue
			}
			if used+w > maxWidth {
				return used
			}
			for offset := w - 1; offset > 0; offset-- {
				screen.SetContent(x+used+offset, y, ' ', nil, s.style)
			}
			runes := g.Runes()
			screen.SetContent(x+used, y, runes[0], runes[1:], s.style)
			used += w
		}
	}
	return used
}

func fitSegs(segs []seg, width int) []seg {
	if width <= 0 {
		return nil
	}
	if segsWidth(segs) <= width {
		return segs
	}
	ell := uniseg.StringWidth(glyphs.ellipsis)
	budget := width - ell
	out := make([]seg, 0, len(segs))
	for _, s := range segs {
		if budget <= 0 {
			break
		}
		cut := truncateCells(s.text, budget)
		out = append(out, seg{cut, s.style})
		budget -= uniseg.StringWidth(cut)
		if cut != s.text {
			break
		}
	}
	style := styleDim
	if len(out) > 0 {
		style = out[len(out)-1].style
	}
	return append(out, seg{glyphs.ellipsis, style})
}

func truncateCells(s string, width int) string {
	used, end := 0, 0
	g := uniseg.NewGraphemes(s)
	for g.Next() {
		w := g.Width()
		if used+w > width {
			break
		}
		used += w
		_, end = g.Positions()
	}
	return s[:end]
}

// A wide character cut in half by skip or width becomes a space.
func sliceCells(s string, skip, width int) string {
	var sb strings.Builder
	pos, used := 0, 0
	g := uniseg.NewGraphemes(s)
	for g.Next() && used < width {
		w := g.Width()
		switch {
		case pos+w <= skip:
		case pos < skip:
			for i := 0; i < pos+w-skip && used < width; i++ {
				sb.WriteByte(' ')
				used++
			}
		case used+w <= width:
			sb.WriteString(g.Str())
			used += w
		default:
			for used < width {
				sb.WriteByte(' ')
				used++
			}
		}
		pos += w
	}
	return sb.String()
}

const (
	marqueePause = 2 * time.Second
	marqueeStep  = 300 * time.Millisecond
	marqueeGap   = 3
)

func marquee(text string, width int, elapsed time.Duration) string {
	textWidth := uniseg.StringWidth(text)
	if textWidth <= width {
		return text
	}
	cycle := textWidth + marqueeGap
	period := marqueePause + time.Duration(cycle)*marqueeStep
	t := elapsed % period
	offset := 0
	if t >= marqueePause {
		offset = int((t - marqueePause) / marqueeStep)
	}
	loop := text + strings.Repeat(" ", marqueeGap) + text
	return sliceCells(loop, offset, width)
}

func gauge(width int, fraction float64, fill, track tcell.Style) []seg {
	if width <= 0 {
		return nil
	}
	fraction = min(max(fraction, 0), 1)
	full, half := int(fraction*float64(width)+0.5), 0
	if glyphs.gaugeHalf != "" {
		halves := int(fraction*float64(width)*2 + 0.5)
		full, half = halves/2, halves%2
	}
	empty := width - full - half
	segs := []seg{{strings.Repeat(glyphs.gaugeFull, full), fill}}
	if half > 0 {
		segs = append(segs, seg{glyphs.gaugeHalf, fill})
	}
	return append(segs, seg{strings.Repeat(glyphs.gaugeEmpty, empty), track})
}

func clock(d time.Duration) string {
	d = max(d, 0).Truncate(time.Second)
	h, m, s := int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}
