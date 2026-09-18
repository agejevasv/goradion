package radio

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// Every theme but the terminal one paints its own background; the terminal one
// lets the terminal's colours show through.
type theme struct {
	name     string
	bg       tcell.Color // behind everything
	text     tcell.Color
	dim      tcell.Color // secondary text, unfocused borders
	accent   tcell.Color // focus, selection, playing
	bright   tcell.Color // the volume gauge while it changes
	warn     tcell.Color // buffering, shuffle, online search
	danger   tcell.Color // errors
	onAccent tcell.Color // text on the accent-coloured selection
}

const defaultTheme = "terminal"

func rgb(v int32) tcell.Color { return tcell.NewHexColor(v) }

// The colours follow each theme's published palette.
var themes = []theme{
	{name: defaultTheme, bg: tcell.ColorDefault, text: tcell.ColorDefault, dim: tcell.ColorGray,
		accent: tcell.ColorGreen, bright: tcell.ColorLime, warn: tcell.ColorYellow, danger: tcell.ColorRed,
		onAccent: tcell.ColorBlack},
	{name: "monokai", bg: rgb(0x272822), text: rgb(0xF8F8F2), dim: rgb(0x75715E),
		accent: rgb(0xA6E22E), bright: rgb(0x66D9EF), warn: rgb(0xE6DB74), danger: rgb(0xF92672),
		onAccent: rgb(0x272822)},
	{name: "material", bg: rgb(0x263238), text: rgb(0xEEFFFF), dim: rgb(0x546E7A),
		accent: rgb(0x80CBC4), bright: rgb(0x89DDFF), warn: rgb(0xFFCB6B), danger: rgb(0xF07178),
		onAccent: rgb(0x263238)},
	{name: "tokyo-night", bg: rgb(0x1A1B26), text: rgb(0xC0CAF5), dim: rgb(0x565F89),
		accent: rgb(0x7AA2F7), bright: rgb(0x7DCFFF), warn: rgb(0xE0AF68), danger: rgb(0xF7768E),
		onAccent: rgb(0x1A1B26)},
	{name: "dracula", bg: rgb(0x282A36), text: rgb(0xF8F8F2), dim: rgb(0x6272A4),
		accent: rgb(0xBD93F9), bright: rgb(0xFF79C6), warn: rgb(0xF1FA8C), danger: rgb(0xFF5555),
		onAccent: rgb(0x282A36)},
	{name: "nord", bg: rgb(0x2E3440), text: rgb(0xD8DEE9), dim: rgb(0x616E88),
		accent: rgb(0x88C0D0), bright: rgb(0x8FBCBB), warn: rgb(0xEBCB8B), danger: rgb(0xBF616A),
		onAccent: rgb(0x2E3440)},
	{name: "gruvbox", bg: rgb(0x282828), text: rgb(0xEBDBB2), dim: rgb(0x928374),
		accent: rgb(0xB8BB26), bright: rgb(0x8EC07C), warn: rgb(0xFABD2F), danger: rgb(0xFB4934),
		onAccent: rgb(0x282828)},
	{name: "catppuccin", bg: rgb(0x1E1E2E), text: rgb(0xCDD6F4), dim: rgb(0x6C7086),
		accent: rgb(0xCBA6F7), bright: rgb(0xF5C2E7), warn: rgb(0xF9E2AF), danger: rgb(0xF38BA8),
		onAccent: rgb(0x1E1E2E)},
	{name: "one-dark", bg: rgb(0x282C34), text: rgb(0xABB2BF), dim: rgb(0x5C6370),
		accent: rgb(0x61AFEF), bright: rgb(0x56B6C2), warn: rgb(0xE5C07B), danger: rgb(0xE06C75),
		onAccent: rgb(0x282C34)},
	{name: "solarized-dark", bg: rgb(0x002B36), text: rgb(0x93A1A1), dim: rgb(0x586E75),
		accent: rgb(0x859900), bright: rgb(0x2AA198), warn: rgb(0xB58900), danger: rgb(0xDC322F),
		onAccent: rgb(0x002B36)},
	{name: "rose-pine", bg: rgb(0x191724), text: rgb(0xE0DEF4), dim: rgb(0x6E6A86),
		accent: rgb(0xEBBCBA), bright: rgb(0x9CCFD8), warn: rgb(0xF6C177), danger: rgb(0xEB6F92),
		onAccent: rgb(0x191724)},
	{name: "everforest", bg: rgb(0x2D353B), text: rgb(0xD3C6AA), dim: rgb(0x859289),
		accent: rgb(0xA7C080), bright: rgb(0x83C092), warn: rgb(0xDBBC7F), danger: rgb(0xE67E80),
		onAccent: rgb(0x2D353B)},
	{name: "kanagawa", bg: rgb(0x1F1F28), text: rgb(0xDCD7BA), dim: rgb(0x727169),
		accent: rgb(0x7E9CD8), bright: rgb(0x7FB4CA), warn: rgb(0xE6C384), danger: rgb(0xE82424),
		onAccent: rgb(0x1F1F28)},
	{name: "solarized-light", bg: rgb(0xFDF6E3), text: rgb(0x586E75), dim: rgb(0x93A1A1),
		accent: rgb(0x268BD2), bright: rgb(0x2AA198), warn: rgb(0xB58900), danger: rgb(0xDC322F),
		onAccent: rgb(0xFDF6E3)},
	{name: "catppuccin-latte", bg: rgb(0xEFF1F5), text: rgb(0x4C4F69), dim: rgb(0x9CA0B0),
		accent: rgb(0x8839EF), bright: rgb(0xEA76CB), warn: rgb(0xDF8E1D), danger: rgb(0xD20F39),
		onAccent: rgb(0xEFF1F5)},
}

func findTheme(name string) (theme, bool) {
	for _, p := range themes {
		if p.name == name {
			return p, true
		}
	}
	return themes[0], false
}

func themeNames() []string {
	names := make([]string, len(themes))
	for i, p := range themes {
		names[i] = p.name
	}
	return names
}

// The current theme's colours, set by useTheme on the UI goroutine.
var (
	colorAccent   tcell.Color
	colorBright   tcell.Color
	colorWarn     tcell.Color
	colorDanger   tcell.Color
	colorDim      tcell.Color
	colorText     tcell.Color
	colorBg       tcell.Color
	colorOnAccent tcell.Color

	styleText              tcell.Style
	styleDim               tcell.Style
	styleAccent            tcell.Style
	styleKey               tcell.Style
	styleSelected          tcell.Style
	styleSelectedUnfocused tcell.Style
)

func init() { useTheme(themes[0]) }

// useTheme sets the colours for custom drawing and for primitives created from
// now on; Application.setTheme also recolours the existing ones.
func useTheme(t theme) {
	colorAccent, colorBright = t.accent, t.bright
	colorWarn, colorDanger = t.warn, t.danger
	colorDim, colorText, colorBg, colorOnAccent = t.dim, t.text, t.bg, t.onAccent

	styleText = tcell.StyleDefault.Foreground(colorText).Background(colorBg)
	styleDim = styleText.Foreground(colorDim)
	styleAccent = styleText.Foreground(colorAccent)
	styleKey = styleText.Bold(true)
	styleSelected = tcell.StyleDefault.Foreground(colorOnAccent).Background(colorAccent).Bold(true)
	styleSelectedUnfocused = styleText.Foreground(colorAccent).Bold(true)

	s := &tview.Styles
	s.PrimitiveBackgroundColor = colorBg
	s.ContrastBackgroundColor = colorBg
	s.MoreContrastBackgroundColor = colorBg
	s.BorderColor = colorDim
	s.TitleColor = colorText
	s.GraphicsColor = colorDim
	s.PrimaryTextColor = colorText
	s.SecondaryTextColor = colorAccent
	s.TertiaryTextColor = colorDim
	s.ContrastSecondaryTextColor = colorDim
}

// colorTag is c as a tview style tag colour; "-" keeps the default.
func colorTag(c tcell.Color) string {
	if c == tcell.ColorDefault {
		return "-"
	}
	return c.String()
}

func fgTag(c tcell.Color) string   { return "[" + colorTag(c) + "]" }
func boldTag(c tcell.Color) string { return "[" + colorTag(c) + "::b]" }

// setTheme also recolours the primitives created before the theme changed.
func (a *Application) setTheme(t theme) {
	useTheme(t)
	for _, l := range []*tview.List{a.tagsList, a.stationsList, a.searchResults, a.themeList} {
		colorList(l)
	}
	for _, b := range []*tview.Box{a.card.Box, a.hints.Box} {
		b.SetBackgroundColor(colorBg)
		b.SetBorderColor(colorDim)
	}
	for _, v := range []*tview.TextView{a.helpView, a.remoteQR, a.remoteText} {
		v.SetBackgroundColor(colorBg)
		v.SetTextStyle(styleText)
	}
	a.helpView.SetBorderColor(colorDim).SetTitleColor(colorAccent)
	a.helpView.SetText(helpText())
	a.applySearchColors()

	// Station and tag labels carry colour tags.
	a.setupTagsList()
	a.reloadStations()
}

func colorList(list *tview.List) {
	list.SetBackgroundColor(colorBg)
	list.SetBorderColor(colorDim)
	list.SetTitleColor(colorAccent)
	list.SetSelectedStyle(styleSelected)
	list.SetMainTextStyle(styleText)
	list.SetShortcutStyle(styleDim)
}
