package radio

import (
	"testing"

	"github.com/agejevasv/goradion/internal/mpv"
	"github.com/gdamore/tcell/v2"
)

// Keys that are shortcuts elsewhere must reach the search field.
func TestSearchFieldKeys(t *testing.T) {
	a, screen := newTestApp(t)
	screen.InjectKey(tcell.KeyCtrlF, 0, tcell.ModNone)
	waitFor(t, "search", func() bool { return onUI(a, func() bool { return a.isFront(pageSearch) }) })

	for _, r := range "lo-fi/#?~:+" {
		screen.InjectKey(tcell.KeyRune, r, tcell.ModNone)
	}
	screen.InjectKey(tcell.KeyLeft, 0, tcell.ModNone)
	screen.InjectKey(tcell.KeyRune, '!', tcell.ModNone)
	waitFor(t, "typed text", func() bool {
		return onUI(a, a.searchInput.GetText) == "lo-fi/#?~:!+"
	})
	if !onUI(a, func() bool { return a.isFront(pageSearch) }) || a.player.Volume() != mpv.DefaultVolume {
		t.Fatal("typing in the search field must not change pages or the volume")
	}

	screen.InjectKey(tcell.KeyEscape, 0, tcell.ModNone)
	waitFor(t, "search closed", func() bool { return onUI(a, func() bool { return a.isFront(pageTags) }) })
	screen.InjectKey(tcell.KeyRune, '-', tcell.ModNone)
	waitFor(t, "volume down", func() bool { return a.player.Volume() == mpv.DefaultVolume-mpv.VolumeStep })
}

func TestHelpKeys(t *testing.T) {
	a, screen := newTestApp(t)
	screen.InjectKey(tcell.KeyRune, '~', tcell.ModNone)
	waitFor(t, "all stations", func() bool { return onUI(a, a.frontPage) == pageMain })
	screen.InjectKey(tcell.KeyRune, '?', tcell.ModNone)
	waitFor(t, "help", func() bool { return onUI(a, a.frontPage) == pageHelp })
	screen.InjectKey(tcell.KeyRune, '?', tcell.ModNone)
	waitFor(t, "back from help", func() bool { return onUI(a, a.frontPage) == pageMain })
	screen.InjectKey(tcell.KeyRune, '?', tcell.ModNone)
	screen.InjectKey(tcell.KeyEscape, 0, tcell.ModNone)
	waitFor(t, "esc closes help", func() bool { return onUI(a, a.frontPage) == pageMain })
}
