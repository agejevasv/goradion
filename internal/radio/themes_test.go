package radio

import (
	"math"
	"os"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
)

func TestThemes(t *testing.T) {
	seen := map[string]bool{}
	for _, th := range themes {
		if seen[th.name] || len(th.name) > themeNameWidth {
			t.Errorf("bad theme name %q", th.name)
		}
		seen[th.name] = true
	}
	if _, ok := findTheme("no-such-theme"); ok {
		t.Error("unknown theme found")
	}
	defer useTheme(themes[0])
	tokyo, _ := findTheme("tokyo-night")
	useTheme(tokyo)
	if got := boldTag(colorAccent); got != "[#7AA2F7::b]" {
		t.Errorf("tag = %q", got)
	}
	if got := colorBorder.Hex(); got != 0x8B95BF {
		t.Errorf("border = %06X", got)
	}
	if got := mix(tcell.ColorGray, tcell.ColorDefault, 0.5); got != tcell.ColorDefault {
		t.Errorf("mix with the terminal's colour = %v", got)
	}
	if got := termBgSequence(colorBg); got != "\x1b]11;#1a1b26\x1b\\" {
		t.Errorf("OSC 11 = %q", got)
	}
	if got := termBgSequence(tcell.ColorDefault); got != resetTermBg {
		t.Errorf("OSC 111 = %q", got)
	}
}

// TestThemeLegibility holds every theme to the rules in docs/themes.md, so a
// palette colour that reads badly in a terminal list is noticed.
func TestThemeLegibility(t *testing.T) {
	// Where no colour in a theme's palette meets a rule, the closest one is used.
	exempt := map[string]string{
		"solarized-light/text":             "Solarized's spec sets body text to base00",
		"solarized-light/cursor text":      "Solarized's spec sets body text to base00",
		"tokyo-night-day/cursor text":      "its CursorLine is the most readable surface it has",
		"material-sandy-beach/cursor text": "its Buttons colour is the most readable surface it has",
	}
	for _, th := range themes[1:] {
		light := luminance(th.bg) > 0.5
		minAccent := 4.5
		if light {
			minAccent = 3
		}
		checks := []struct {
			rule      string
			got, want float64
		}{
			{"text", contrast(th.text, th.bg), 4.5},
			{"cursor text", contrast(th.text, th.surface), 4},
			{"cursor", contrast(th.surface, th.bg), 1.1},
			{"dim", contrast(th.dim, th.bg), 2.3},
			{"accent", contrast(th.accent, th.bg), minAccent},
			{"accent vs warn", deltaE(th.accent, th.warn), 15},
			{"accent vs bright", deltaE(th.accent, th.bright), 15},
			{"accent vs danger", deltaE(th.accent, th.danger), 15},
			{"live vs dim", deltaE(th.live, th.dim), 15},
		}
		for _, c := range checks {
			if c.got < c.want && exempt[th.name+"/"+c.rule] == "" {
				t.Errorf("%s: %s is %.2f, want %.2f or more", th.name, c.rule, c.got, c.want)
			}
		}
	}
}

// luminance and contrast follow WCAG 2.
func luminance(c tcell.Color) float64 {
	r, g, b := c.RGB()
	lin := func(v int32) float64 {
		x := float64(v) / 255
		if x <= 0.03928 {
			return x / 12.92
		}
		return math.Pow((x+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(r) + 0.7152*lin(g) + 0.0722*lin(b)
}

func contrast(a, b tcell.Color) float64 {
	la, lb := luminance(a), luminance(b)
	return (max(la, lb) + 0.05) / (min(la, lb) + 0.05)
}

// deltaE is the CIE76 distance: under 15, two colours pass for one at a glance.
func deltaE(a, b tcell.Color) float64 {
	lab := func(c tcell.Color) [3]float64 {
		r, g, b := c.RGB()
		lin := func(v int32) float64 {
			x := float64(v) / 255
			if x <= 0.04045 {
				return x / 12.92
			}
			return math.Pow((x+0.055)/1.055, 2.4)
		}
		R, G, B := lin(r), lin(g), lin(b)
		x := (R*0.4124 + G*0.3576 + B*0.1805) / 0.95047
		y := R*0.2126 + G*0.7152 + B*0.0722
		z := (R*0.0193 + G*0.1192 + B*0.9505) / 1.08883
		f := func(t float64) float64 {
			if t > 0.008856 {
				return math.Cbrt(t)
			}
			return 7.787*t + 16.0/116
		}
		return [3]float64{116*f(y) - 16, 500 * (f(x) - f(y)), 200 * (f(y) - f(z))}
	}
	p, q := lab(a), lab(b)
	return math.Sqrt((p[0]-q[0])*(p[0]-q[0]) + (p[1]-q[1])*(p[1]-q[1]) + (p[2]-q[2])*(p[2]-q[2]))
}

func TestThemeModal(t *testing.T) {
	defer useTheme(themes[0])
	a, screen := newTestApp(t)
	bgAt := func(x, y int) tcell.Color {
		_, _, style, _ := screen.GetContent(x, y)
		_, bg, _ := style.Decompose()
		return bg
	}
	monokai, _ := findTheme("monokai")

	screen.InjectKey(tcell.KeyCtrlT, 0, tcell.ModNone)
	waitFor(t, "theme panel", func() bool {
		return strings.Contains(screenText(a, screen), " Theme ")
	})
	screen.InjectKey(tcell.KeyDown, 0, tcell.ModNone)
	waitFor(t, "preview", func() bool { return onUI(a, func() tcell.Color { return bgAt(0, 0) }) == monokai.bg })
	if onUI(a, func() string { return a.config.Theme }) != defaultTheme {
		t.Fatal("preview must not save")
	}

	screen.InjectKey(tcell.KeyEscape, 0, tcell.ModNone)
	waitFor(t, "cancel", func() bool {
		return onUI(a, func() bool { return !a.isFront(pageTheme) && bgAt(0, 0) == tcell.ColorDefault })
	})

	screen.InjectKey(tcell.KeyCtrlT, 0, tcell.ModNone)
	waitFor(t, "theme panel again", func() bool { return onUI(a, func() bool { return a.isFront(pageTheme) }) })
	screen.InjectKey(tcell.KeyRune, 'b', tcell.ModNone)
	waitFor(t, "saved", func() bool { return onUI(a, func() string { return a.config.Theme }) == "monokai" })
	data, _ := os.ReadFile(configFile())
	if !strings.Contains(string(data), "theme: monokai") {
		t.Fatalf("config = %q", data)
	}
	if onUI(a, func() bool { return a.isFront(pageTheme) }) || onUI(a, func() tcell.Color { return bgAt(0, 0) }) != monokai.bg {
		t.Fatal("the saved theme should stay after the panel closes")
	}
}

// The song line must not read as another shade of the accent.
func TestThemeSongHue(t *testing.T) {
	hue := func(c tcell.Color) float64 {
		r, g, b := c.RGB()
		h, _, _ := rgbToHSL(float64(r)/255, float64(g)/255, float64(b)/255)
		return h
	}
	for _, th := range themes {
		if !th.song.IsRGB() || !th.accent.IsRGB() {
			continue
		}
		d := math.Abs(hue(th.song) - hue(th.accent))
		if d = math.Min(d, 360-d); d < 35 {
			t.Errorf("%s: song %06X is %.0f° from accent %06X", th.name, th.song.Hex(), d, th.accent.Hex())
		}
	}
}

func rgbToHSL(r, g, b float64) (h, s, l float64) {
	hi, lo := math.Max(r, math.Max(g, b)), math.Min(r, math.Min(g, b))
	l = (hi + lo) / 2
	if hi == lo {
		return 0, 0, l
	}
	d := hi - lo
	switch hi {
	case r:
		h = math.Mod((g-b)/d, 6)
	case g:
		h = (b-r)/d + 2
	default:
		h = (r-g)/d + 4
	}
	return math.Mod(h*60+360, 360), d / (1 - math.Abs(2*l-1)), l
}
