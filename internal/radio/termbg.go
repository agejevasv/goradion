package radio

import (
	"fmt"
	"os"

	"github.com/gdamore/tcell/v2"
)

// The terminal's padding around the cell grid can't be drawn on, so a themed
// background is also set as the terminal's default background (OSC 11), and
// reset on exit (OSC 111). Terminals without OSC 11 ignore both.

const resetTermBg = "\x1b]111\x1b\\"

func termBgSequence(c tcell.Color) string {
	if c == tcell.ColorDefault {
		return resetTermBg
	}
	return fmt.Sprintf("\x1b]11;#%06x\x1b\\", c.Hex())
}

func (a *Application) syncTermBg(screen tcell.Screen) {
	if colorBg == a.termBg {
		return
	}
	tty, ok := screen.Tty()
	if !ok {
		return
	}
	if _, err := tty.Write([]byte(termBgSequence(colorBg))); err != nil {
		log.Printf("terminal background: %v", err)
	}
	a.termBg = colorBg
}

// restoreTermBg runs after the screen is closed, so it opens the terminal itself.
func (a *Application) restoreTermBg() {
	if a.termBg == tcell.ColorDefault {
		return
	}
	tty, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0)
	if err != nil {
		return
	}
	defer tty.Close()
	tty.WriteString(resetTermBg)
}
