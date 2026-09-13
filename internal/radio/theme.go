package radio

import (
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// Palette colours and default backgrounds let the terminal's theme show through.
const (
	colorAccent       = tcell.ColorGreen
	colorAccentBright = tcell.ColorLime
	colorWarn         = tcell.ColorYellow
	colorDanger       = tcell.ColorRed
	colorDim          = tcell.ColorGray
	colorText         = tcell.ColorDefault
)

var (
	styleText              = tcell.StyleDefault.Foreground(colorText).Background(tcell.ColorDefault)
	styleDim               = styleText.Foreground(colorDim)
	styleAccent            = styleText.Foreground(colorAccent)
	styleKey               = styleText.Bold(true)
	styleSelected          = tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(colorAccent).Bold(true)
	styleSelectedUnfocused = styleText.Foreground(colorAccent).Bold(true)
)

type glyphSet struct {
	play, stop, fail, song, notes, back, dot, shuffle, star string
	left, right, up, down                                   string
	ellipsis                                                string

	spinner []string

	gaugeFull, gaugeHalf, gaugeEmpty string

	vuLit, vuUnlit []string
}

var unicodeGlyphs = glyphSet{
	play: "▶", stop: "■", fail: "!", song: "♪", notes: "♫", back: "‹", dot: "·", shuffle: "🔀", star: "★",
	left: "←", right: "→", up: "↑", down: "↓",
	ellipsis:  "…",
	spinner:   []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
	gaugeFull: "━", gaugeHalf: "╸", gaugeEmpty: "─",
	vuLit:   strings.Split("▁▂▂▃▃▄▅▅▆▆▇█", ""),
	vuUnlit: strings.Split("▁▂▂▃▃▄▅▅▆▆▇█", ""),
}

var asciiGlyphs = glyphSet{
	play: ">", stop: "#", fail: "!", song: "~", notes: "*", back: "<", dot: "-", shuffle: "", star: "*",
	left: "<-", right: "->", up: "^", down: "v",
	ellipsis:  "~",
	spinner:   []string{"|", "/", "-", "\\"},
	gaugeFull: "=", gaugeHalf: "", gaugeEmpty: "-",
	vuLit:   strings.Split("||||||||||||", ""),
	vuUnlit: strings.Split("............", ""),
}

var glyphs = unicodeGlyphs

func applyTheme(ascii bool) {
	if ascii {
		glyphs = asciiGlyphs
	} else {
		glyphs = unicodeGlyphs
	}

	b := &tview.Borders
	if ascii {
		b.Horizontal, b.Vertical = '-', '|'
		b.TopLeft, b.TopRight, b.BottomLeft, b.BottomRight = '+', '+', '+', '+'
	} else {
		b.Horizontal, b.Vertical = '─', '│'
		b.TopLeft, b.TopRight, b.BottomLeft, b.BottomRight = '╭', '╮', '╰', '╯'
	}
	// Focus is shown with colour, not with tview's double lines.
	b.HorizontalFocus, b.VerticalFocus = b.Horizontal, b.Vertical
	b.TopLeftFocus, b.TopRightFocus = b.TopLeft, b.TopRight
	b.BottomLeftFocus, b.BottomRightFocus = b.BottomLeft, b.BottomRight

	s := &tview.Styles
	s.PrimitiveBackgroundColor = tcell.ColorDefault
	s.ContrastBackgroundColor = tcell.ColorDefault
	s.MoreContrastBackgroundColor = tcell.ColorDefault
	s.BorderColor = colorDim
	s.TitleColor = colorText
	s.GraphicsColor = colorDim
	s.PrimaryTextColor = colorText
}

// detectASCII follows tcell's charset rules: where tcell falls back to ASCII,
// Unicode symbols would print as question marks.
func detectASCII() bool {
	if runtime.GOOS == "windows" {
		return false
	}
	if os.Getenv("TERM") == "linux" {
		return true
	}
	locale := os.Getenv("LC_ALL")
	if locale == "" {
		locale = os.Getenv("LC_CTYPE")
	}
	if locale == "" {
		locale = os.Getenv("LANG")
	}
	return !localeIsUTF8(locale)
}

func localeIsUTF8(locale string) bool {
	if locale == "C" || locale == "POSIX" {
		return false
	}
	if i := strings.IndexByte(locale, '@'); i >= 0 {
		locale = locale[:i]
	}
	i := strings.IndexByte(locale, '.')
	if i < 0 {
		return true // No charset given, e.g. "en_US" or empty: tcell assumes UTF-8.
	}
	charset := strings.ToLower(locale[i+1:])
	return charset == "utf-8" || charset == "utf8"
}

func spinnerFrame(t time.Time) string {
	return glyphs.spinner[int(t.UnixMilli()/100)%len(glyphs.spinner)]
}
