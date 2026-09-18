package radio

import (
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
	if got := termBgSequence(colorBg); got != "\x1b]11;#1a1b26\x1b\\" {
		t.Errorf("OSC 11 = %q", got)
	}
	if got := termBgSequence(tcell.ColorDefault); got != resetTermBg {
		t.Errorf("OSC 111 = %q", got)
	}
	if got := stripPlayCount("Radio " + fgTag(colorDim) + "(12)[-]"); got != "Radio" {
		t.Errorf("stripPlayCount = %q", got)
	}
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
		return onUI(a, func() bool { return strings.Contains(screenText(screen), " Theme ") })
	})
	screen.InjectKey(tcell.KeyDown, 0, tcell.ModNone)
	waitFor(t, "preview", func() bool { return onUI(a, func() tcell.Color { return bgAt(0, 0) }) == monokai.bg })
	if onUI(a, func() string { return a.config.Theme }) != defaultTheme {
		t.Fatal("preview must not save")
	}

	screen.InjectKey(tcell.KeyEscape, 0, tcell.ModNone)
	waitFor(t, "cancel", func() bool {
		return onUI(a, func() bool { return !a.isThemeModalOpen() && bgAt(0, 0) == tcell.ColorDefault })
	})

	screen.InjectKey(tcell.KeyCtrlT, 0, tcell.ModNone)
	waitFor(t, "theme panel again", func() bool { return onUI(a, a.isThemeModalOpen) })
	screen.InjectKey(tcell.KeyRune, 'b', tcell.ModNone)
	waitFor(t, "saved", func() bool { return onUI(a, func() string { return a.config.Theme }) == "monokai" })
	data, _ := os.ReadFile(configFile())
	if !strings.Contains(string(data), "theme: monokai") {
		t.Fatalf("config = %q", data)
	}
	if onUI(a, a.isThemeModalOpen) || onUI(a, func() tcell.Color { return bgAt(0, 0) }) != monokai.bg {
		t.Fatal("the saved theme should stay after the panel closes")
	}
}
