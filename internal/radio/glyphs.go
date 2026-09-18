package radio

import (
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/rivo/tview"
)

type glyphSet struct {
	play, stop, fail, song, notes, back, dot, shuffle, star, check, swatch string
	left, right, up, down                                                  string
	ellipsis                                                               string

	spinner []string

	gaugeFull, gaugeHalf, gaugeEmpty string

	vuLit, vuUnlit []string

	bands []string // spectrum bars, from quiet to full
}

var unicodeGlyphs = glyphSet{
	play: "▶", stop: "■", fail: "!", song: "♪", notes: "♫", back: "‹", dot: "·", shuffle: "🔀", star: "★", check: "✓", swatch: "██",
	left: "←", right: "→", up: "↑", down: "↓",
	ellipsis:  "…",
	spinner:   []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
	gaugeFull: "━", gaugeHalf: "╸", gaugeEmpty: "─",
	vuLit:   strings.Split("▁▂▂▃▃▄▅▅▆▆▇█", ""),
	vuUnlit: strings.Split("▁▂▂▃▃▄▅▅▆▆▇█", ""),
	bands:   strings.Split("▁▂▃▄▅▆▇█", ""),
}

var asciiGlyphs = glyphSet{
	play: ">", stop: "#", fail: "!", song: "~", notes: "*", back: "<", dot: "-", shuffle: "", star: "*", check: "*", swatch: "##",
	left: "<-", right: "->", up: "^", down: "v",
	ellipsis:  "~",
	spinner:   []string{"|", "/", "-", "\\"},
	gaugeFull: "=", gaugeHalf: "", gaugeEmpty: "-",
	vuLit:   strings.Split("||||||||||||", ""),
	vuUnlit: strings.Split("............", ""),
	bands:   strings.Split(".:|#", ""),
}

var glyphs = unicodeGlyphs

func applyGlyphs(ascii bool) {
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
