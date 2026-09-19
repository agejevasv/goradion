package radio

import (
	"math"

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
	surface  tcell.Color // the cursor bar; ColorDefault draws it in reverse video
	accent   tcell.Color // what is live: playing, the meter, the focused cursor's edge
	bright   tcell.Color // the volume gauge while it changes
	song     tcell.Color // the song playing: a hue of its own, apart from the others
	warn     tcell.Color // buffering, shuffle, online search
	danger   tcell.Color // errors
	live     tcell.Color // the card's light while playing: the green
	onAccent tcell.Color // text on the accent-coloured selection
}

const defaultTheme = "terminal"

func rgb(v int32) tcell.Color { return tcell.NewHexColor(v) }

// The colours come from each theme's own published palette, by the rules in
// docs/themes.md; TestThemeLegibility holds every theme to them.
var themes = []theme{
	{name: defaultTheme, bg: tcell.ColorDefault, text: tcell.ColorDefault, dim: tcell.ColorGray, surface: tcell.ColorDefault,
		accent: tcell.ColorGreen, bright: tcell.ColorLime, song: tcell.ColorPurple, warn: tcell.ColorYellow, danger: tcell.ColorRed, live: tcell.ColorGreen,
		onAccent: tcell.ColorBlack},
	{name: "monokai", bg: rgb(0x272822), text: rgb(0xF8F8F2), dim: rgb(0x75715E), surface: rgb(0x49483E),
		accent: rgb(0xA6E22E), bright: rgb(0x66D9EF), song: rgb(0xAE81FF), warn: rgb(0xE6DB74), danger: rgb(0xF92672), live: rgb(0xA6E22E),
		onAccent: rgb(0x272822)},
	{name: "dracula", bg: rgb(0x282A36), text: rgb(0xF8F8F2), dim: rgb(0x6272A4), surface: rgb(0x44475A),
		accent: rgb(0xBD93F9), bright: rgb(0xFF79C6), song: rgb(0xFF79C6), warn: rgb(0xF1FA8C), danger: rgb(0xFF5555), live: rgb(0x50FA7B),
		onAccent: rgb(0x282A36)},
	{name: "one-dark", bg: rgb(0x282C34), text: rgb(0xABB2BF), dim: rgb(0x5C6370), surface: rgb(0x3E4451),
		accent: rgb(0x61AFEF), bright: rgb(0x56B6C2), song: rgb(0xC678DD), warn: rgb(0xE5C07B), danger: rgb(0xE06C75), live: rgb(0x98C379),
		onAccent: rgb(0x282C34)},
	{name: "material", bg: rgb(0x263238), text: rgb(0xB0BEC5), dim: rgb(0x546E7A), surface: rgb(0x314549),
		accent: rgb(0x80CBC4), bright: rgb(0x89DDFF), song: rgb(0xC792EA), warn: rgb(0xFFCB6B), danger: rgb(0xFF5370), live: rgb(0xC3E88D),
		onAccent: rgb(0x263238)},
	{name: "palenight", bg: rgb(0x292D3E), text: rgb(0xA6ACCD), dim: rgb(0x676E95), surface: rgb(0x414863),
		accent: rgb(0xC792EA), bright: rgb(0x89DDFF), song: rgb(0x89DDFF), warn: rgb(0xFFCB6B), danger: rgb(0xFF5370), live: rgb(0xC3E88D),
		onAccent: rgb(0x292D3E)},
	{name: "material-darker", bg: rgb(0x212121), text: rgb(0xB0BEC5), dim: rgb(0x616161), surface: rgb(0x323232),
		accent: rgb(0xFF9800), bright: rgb(0x89DDFF), song: rgb(0xC792EA), warn: rgb(0xFFCB6B), danger: rgb(0xFF5370), live: rgb(0xC3E88D),
		onAccent: rgb(0x212121)},
	{name: "material-deep-ocean", bg: rgb(0x0F111A), text: rgb(0x8F93A2), dim: rgb(0x717CB4), surface: rgb(0x1F2233),
		accent: rgb(0x84FFFF), bright: rgb(0x89DDFF), song: rgb(0xC792EA), warn: rgb(0xFFCB6B), danger: rgb(0xFF5370), live: rgb(0xC3E88D),
		onAccent: rgb(0x0F111A)},
	{name: "material-space", bg: rgb(0x1B2240), text: rgb(0xEFEFF1), dim: rgb(0x959DAA), surface: rgb(0x303C6A),
		accent: rgb(0xAD9BF6), bright: rgb(0x89DDFF), song: rgb(0x89DDFF), warn: rgb(0xFFCB6B), danger: rgb(0xFF5370), live: rgb(0xC3E88D),
		onAccent: rgb(0x1B2240)},
	{name: "material-forest", bg: rgb(0x002626), text: rgb(0xB2C2B0), dim: rgb(0x49694D), surface: rgb(0x104110),
		accent: rgb(0xFFCC80), bright: rgb(0x89DDFF), song: rgb(0xC792EA), warn: rgb(0xF78C6C), danger: rgb(0xFF5370), live: rgb(0xC3E88D),
		onAccent: rgb(0x002626)},
	{name: "material-volcano", bg: rgb(0x390000), text: rgb(0xFFEAEA), dim: rgb(0x7F6451), surface: rgb(0x550000),
		accent: rgb(0x00BCD4), bright: rgb(0xC792EA), song: rgb(0xC792EA), warn: rgb(0xFFCB6B), danger: rgb(0xFF5370), live: rgb(0xC3E88D),
		onAccent: rgb(0x390000)},
	{name: "nord", bg: rgb(0x2E3440), text: rgb(0xD8DEE9), dim: rgb(0x616E88), surface: rgb(0x434C5E),
		accent: rgb(0x88C0D0), bright: rgb(0xB48EAD), song: rgb(0xB48EAD), warn: rgb(0xEBCB8B), danger: rgb(0xBF616A), live: rgb(0xA3BE8C),
		onAccent: rgb(0x2E3440)},
	{name: "night-owl", bg: rgb(0x011627), text: rgb(0xD6DEEB), dim: rgb(0x637777), surface: rgb(0x1D3B53),
		accent: rgb(0x82AAFF), bright: rgb(0xC792EA), song: rgb(0xC792EA), warn: rgb(0xECC48D), danger: rgb(0xEF5350), live: rgb(0x22DA6E),
		onAccent: rgb(0x011627)},
	{name: "github-dark", bg: rgb(0x0D1117), text: rgb(0xF0F6FC), dim: rgb(0x9198A1), surface: rgb(0x1F232A),
		accent: rgb(0x4493F8), bright: rgb(0xAB7DF8), song: rgb(0xAB7DF8), warn: rgb(0xD29922), danger: rgb(0xF85149), live: rgb(0x3FB950),
		onAccent: rgb(0x0D1117)},
	{name: "tokyo-night", bg: rgb(0x1A1B26), text: rgb(0xC0CAF5), dim: rgb(0x565F89), surface: rgb(0x283457),
		accent: rgb(0x7AA2F7), bright: rgb(0x7DCFFF), song: rgb(0xBB9AF7), warn: rgb(0xE0AF68), danger: rgb(0xF7768E), live: rgb(0x9ECE6A),
		onAccent: rgb(0x1A1B26)},
	{name: "tokyo-night-storm", bg: rgb(0x24283B), text: rgb(0xC0CAF5), dim: rgb(0x565F89), surface: rgb(0x2E3C64),
		accent: rgb(0x7AA2F7), bright: rgb(0x7DCFFF), song: rgb(0xBB9AF7), warn: rgb(0xE0AF68), danger: rgb(0xF7768E), live: rgb(0x9ECE6A),
		onAccent: rgb(0x24283B)},
	{name: "tokyo-night-moon", bg: rgb(0x222436), text: rgb(0xC8D3F5), dim: rgb(0x636DA6), surface: rgb(0x2D3F76),
		accent: rgb(0x82AAFF), bright: rgb(0x86E1FC), song: rgb(0xC099FF), warn: rgb(0xFFC777), danger: rgb(0xFF757F), live: rgb(0xC3E88D),
		onAccent: rgb(0x222436)},
	{name: "catppuccin", bg: rgb(0x1E1E2E), text: rgb(0xCDD6F4), dim: rgb(0x6C7086), surface: rgb(0x45475A),
		accent: rgb(0xCBA6F7), bright: rgb(0xF5C2E7), song: rgb(0x89DCEB), warn: rgb(0xF9E2AF), danger: rgb(0xF38BA8), live: rgb(0xA6E3A1),
		onAccent: rgb(0x1E1E2E)},
	{name: "catppuccin-macchiato", bg: rgb(0x24273A), text: rgb(0xCAD3F5), dim: rgb(0x6E738D), surface: rgb(0x494D64),
		accent: rgb(0xC6A0F6), bright: rgb(0xF5BDE6), song: rgb(0x91D7E3), warn: rgb(0xEED49F), danger: rgb(0xED8796), live: rgb(0xA6DA95),
		onAccent: rgb(0x24273A)},
	{name: "catppuccin-frappe", bg: rgb(0x303446), text: rgb(0xC6D0F5), dim: rgb(0x737994), surface: rgb(0x51576D),
		accent: rgb(0xCA9EE6), bright: rgb(0xF4B8E4), song: rgb(0x99D1DB), warn: rgb(0xE5C890), danger: rgb(0xE78284), live: rgb(0xA6D189),
		onAccent: rgb(0x303446)},
	{name: "rose-pine", bg: rgb(0x191724), text: rgb(0xE0DEF4), dim: rgb(0x6E6A86), surface: rgb(0x403D52),
		accent: rgb(0xEBBCBA), bright: rgb(0x9CCFD8), song: rgb(0xC4A7E7), warn: rgb(0xF6C177), danger: rgb(0xEB6F92), live: rgb(0x95B1AC),
		onAccent: rgb(0x191724)},
	{name: "rose-pine-moon", bg: rgb(0x232136), text: rgb(0xE0DEF4), dim: rgb(0x6E6A86), surface: rgb(0x44415A),
		accent: rgb(0xEA9A97), bright: rgb(0x9CCFD8), song: rgb(0xC4A7E7), warn: rgb(0xF6C177), danger: rgb(0xEB6F92), live: rgb(0x95B1AC),
		onAccent: rgb(0x232136)},
	{name: "gruvbox", bg: rgb(0x282828), text: rgb(0xEBDBB2), dim: rgb(0x928374), surface: rgb(0x504945),
		accent: rgb(0xB8BB26), bright: rgb(0x8EC07C), song: rgb(0xD3869B), warn: rgb(0xFABD2F), danger: rgb(0xFB4934), live: rgb(0xB8BB26),
		onAccent: rgb(0x282828)},
	{name: "everforest", bg: rgb(0x2D353B), text: rgb(0xD3C6AA), dim: rgb(0x859289), surface: rgb(0x475258),
		accent: rgb(0xA7C080), bright: rgb(0x83C092), song: rgb(0x7FBBB3), warn: rgb(0xDBBC7F), danger: rgb(0xE67E80), live: rgb(0xA7C080),
		onAccent: rgb(0x2D353B)},
	{name: "kanagawa", bg: rgb(0x1F1F28), text: rgb(0xDCD7BA), dim: rgb(0x727169), surface: rgb(0x363646),
		accent: rgb(0x7E9CD8), bright: rgb(0x7FB4CA), song: rgb(0x957FB8), warn: rgb(0xE6C384), danger: rgb(0xE82424), live: rgb(0x98BB6C),
		onAccent: rgb(0x1F1F28)},
	{name: "kanagawa-dragon", bg: rgb(0x181616), text: rgb(0xC5C9C5), dim: rgb(0x737C73), surface: rgb(0x393836),
		accent: rgb(0x8BA4B0), bright: rgb(0xA292A3), song: rgb(0xA292A3), warn: rgb(0xC4B28A), danger: rgb(0xE82424), live: rgb(0x87A987),
		onAccent: rgb(0x181616)},
	{name: "nightfox", bg: rgb(0x192330), text: rgb(0xCDCECF), dim: rgb(0x738091), surface: rgb(0x3C5372),
		accent: rgb(0x719CD6), bright: rgb(0x63CDCF), song: rgb(0x9D79D6), warn: rgb(0xDBC074), danger: rgb(0xC94F6D), live: rgb(0x81B29A),
		onAccent: rgb(0x192330)},
	{name: "carbonfox", bg: rgb(0x161616), text: rgb(0xF2F4F8), dim: rgb(0x6E6F70), surface: rgb(0x525253),
		accent: rgb(0x78A9FF), bright: rgb(0xBE95FF), song: rgb(0xBE95FF), warn: rgb(0x08BDBA), danger: rgb(0xEE5396), live: rgb(0x25BE6A),
		onAccent: rgb(0x161616)},
	{name: "ayu-dark", bg: rgb(0x10141C), text: rgb(0xBFBDB6), dim: rgb(0x5B6875), surface: rgb(0x193155),
		accent: rgb(0xE6B450), bright: rgb(0xAAD94C), song: rgb(0xD2A6FF), warn: rgb(0xFF8F40), danger: rgb(0xD95757), live: rgb(0xAAD94C),
		onAccent: rgb(0x10141C)},
	{name: "ayu-mirage", bg: rgb(0x242936), text: rgb(0xCCCAC2), dim: rgb(0x6E7C8E), surface: rgb(0x2B4768),
		accent: rgb(0xFFCC66), bright: rgb(0xD5FF80), song: rgb(0xDFBFFF), warn: rgb(0xFFAD66), danger: rgb(0xFF6666), live: rgb(0xD5FF80),
		onAccent: rgb(0x242936)},
	{name: "solarized-dark", bg: rgb(0x002B36), text: rgb(0x839496), dim: rgb(0x586E75), surface: rgb(0x073642),
		accent: rgb(0x859900), bright: rgb(0x2AA198), song: rgb(0xD33682), warn: rgb(0xB58900), danger: rgb(0xDC322F), live: rgb(0x859900),
		onAccent: rgb(0x002B36)},
	{name: "solarized-light", bg: rgb(0xFDF6E3), text: rgb(0x657B83), dim: rgb(0x93A1A1), surface: rgb(0xEEE8D5),
		accent: rgb(0x268BD2), bright: rgb(0x2AA198), song: rgb(0xD33682), warn: rgb(0xB58900), danger: rgb(0xDC322F), live: rgb(0x859900),
		onAccent: rgb(0xFDF6E3)},
	{name: "material-lighter", bg: rgb(0xFAFAFA), text: rgb(0x546E7A), dim: rgb(0x94A7B0), surface: rgb(0xE7E7E8),
		accent: rgb(0x6182B8), bright: rgb(0x39ADB5), song: rgb(0x7C4DFF), warn: rgb(0xF6A434), danger: rgb(0xE53935), live: rgb(0x91B859),
		onAccent: rgb(0xFAFAFA)},
	{name: "material-sky-blue", bg: rgb(0xF5F5F5), text: rgb(0x005761), dim: rgb(0x01579B), surface: rgb(0xADE2EB),
		accent: rgb(0x6182B8), bright: rgb(0x39ADB5), song: rgb(0x7C4DFF), warn: rgb(0xF6A434), danger: rgb(0xE53935), live: rgb(0x91B859),
		onAccent: rgb(0xF5F5F5)},
	{name: "material-sandy-beach", bg: rgb(0xFFF8ED), text: rgb(0x546E7A), dim: rgb(0x888477), surface: rgb(0xF6D7B0),
		accent: rgb(0x6182B8), bright: rgb(0x39ADB5), song: rgb(0x7C4DFF), warn: rgb(0xF6A434), danger: rgb(0xE53935), live: rgb(0x91B859),
		onAccent: rgb(0xFFF8ED)},
	{name: "github-light", bg: rgb(0xFFFFFF), text: rgb(0x1F2328), dim: rgb(0x59636E), surface: rgb(0xDDF4FF),
		accent: rgb(0x0969DA), bright: rgb(0x8250DF), song: rgb(0x8250DF), warn: rgb(0x9A6700), danger: rgb(0xD1242F), live: rgb(0x1A7F37),
		onAccent: rgb(0xFFFFFF)},
	{name: "catppuccin-latte", bg: rgb(0xEFF1F5), text: rgb(0x4C4F69), dim: rgb(0x9CA0B0), surface: rgb(0xCCD0DA),
		accent: rgb(0x8839EF), bright: rgb(0xEA76CB), song: rgb(0x1E66F5), warn: rgb(0xDF8E1D), danger: rgb(0xD20F39), live: rgb(0x40A02B),
		onAccent: rgb(0xEFF1F5)},
	{name: "tokyo-night-day", bg: rgb(0xE1E2E7), text: rgb(0x3760BF), dim: rgb(0x848CB5), surface: rgb(0xC4C8DA),
		accent: rgb(0x2E7DE9), bright: rgb(0x007197), song: rgb(0x9854F1), warn: rgb(0x8C6C3E), danger: rgb(0xF52A65), live: rgb(0x587539),
		onAccent: rgb(0xE1E2E7)},
	{name: "rose-pine-dawn", bg: rgb(0xFAF4ED), text: rgb(0x464261), dim: rgb(0x9893A5), surface: rgb(0xDFDAD9),
		accent: rgb(0x907AA9), bright: rgb(0x56949F), song: rgb(0x286983), warn: rgb(0xEA9D34), danger: rgb(0xB4637A), live: rgb(0x6D8F89),
		onAccent: rgb(0xFAF4ED)},
	{name: "gruvbox-light", bg: rgb(0xFBF1C7), text: rgb(0x3C3836), dim: rgb(0x928374), surface: rgb(0xD5C4A1),
		accent: rgb(0x79740E), bright: rgb(0x427B58), song: rgb(0x8F3F71), warn: rgb(0xB57614), danger: rgb(0x9D0006), live: rgb(0x79740E),
		onAccent: rgb(0xFBF1C7)},
	{name: "everforest-light", bg: rgb(0xFDF6E3), text: rgb(0x5C6A72), dim: rgb(0x939F91), surface: rgb(0xE6E2CC),
		accent: rgb(0x3A94C5), bright: rgb(0x35A77C), song: rgb(0xDF69BA), warn: rgb(0xDFA000), danger: rgb(0xF85552), live: rgb(0x8DA101),
		onAccent: rgb(0xFDF6E3)},
	{name: "kanagawa-lotus", bg: rgb(0xF2ECBC), text: rgb(0x545464), dim: rgb(0x8A8980), surface: rgb(0xE4D794),
		accent: rgb(0x4D699B), bright: rgb(0x6693BF), song: rgb(0x624C83), warn: rgb(0xE98A00), danger: rgb(0xE82424), live: rgb(0x6F894E),
		onAccent: rgb(0xF2ECBC)},
	{name: "ayu-light", bg: rgb(0xFCFCFC), text: rgb(0x5C6166), dim: rgb(0x828E9F), surface: rgb(0xD7E4F6),
		accent: rgb(0xA37ACC), bright: rgb(0x399EE6), song: rgb(0x22A4E6), warn: rgb(0xF2A300), danger: rgb(0xE65050), live: rgb(0x86B300),
		onAccent: rgb(0xFCFCFC)},
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
	colorSong     tcell.Color
	colorWarn     tcell.Color
	colorDanger   tcell.Color
	colorDim      tcell.Color
	colorText     tcell.Color
	colorBg       tcell.Color
	colorSurface  tcell.Color
	colorOnAccent tcell.Color
	colorLive     tcell.Color
	colorBorder   tcell.Color // the focused pane's frame

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
	colorAccent, colorBright, colorSong = t.accent, t.bright, t.song
	colorWarn, colorDanger = t.warn, t.danger
	colorDim, colorText, colorBg, colorOnAccent = t.dim, t.text, t.bg, t.onAccent
	colorSurface, colorLive = t.surface, t.live
	colorBorder = mix(t.dim, t.text, 0.5)

	styleText = tcell.StyleDefault.Foreground(colorText).Background(colorBg)
	styleDim = styleText.Foreground(colorDim)
	styleAccent = styleText.Foreground(colorAccent)
	styleKey = styleText.Bold(true)
	// The cursor stays neutral, so the accent is left to mark what is playing.
	styleSelected = styleText.Background(colorSurface).Bold(true)
	if colorSurface == tcell.ColorDefault {
		styleSelected = styleText.Reverse(true).Bold(true)
	}
	styleSelectedUnfocused = styleText.Bold(true)

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

// mix blends a into b by t; colours without RGB values (the terminal's own)
// give b.
func mix(a, b tcell.Color, t float64) tcell.Color {
	if a.Hex() < 0 || b.Hex() < 0 {
		return b
	}
	ar, ag, ab := a.RGB()
	br, bg, bb := b.RGB()
	blend := func(x, y int32) int32 { return x + int32(math.Round(float64(y-x)*t)) }
	return tcell.NewRGBColor(blend(ar, br), blend(ag, bg), blend(ab, bb))
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
	a.helpView.SetBorderColor(colorDim).SetTitleColor(colorText)
	a.helpView.SetText(helpText())
	a.applySearchColors()

	// Station and tag labels carry colour tags.
	a.showTags()
	a.reloadStations()
}

func colorList(list *tview.List) {
	list.SetBackgroundColor(colorBg)
	list.SetBorderColor(colorDim)
	list.SetTitleColor(colorText)
	list.SetSelectedStyle(styleSelected)
	list.SetMainTextStyle(styleText)
	list.SetShortcutStyle(styleDim)
}
