package radio

import (
	"fmt"
	"strings"

	"github.com/rivo/tview"
)

func helpText() string {
	arrows := glyphs.left + " " + glyphs.right + " - +"
	sections := []struct {
		title string
		keys  [][2]string
	}{
		{"Playing", [][2]string{
			{"a-z A-Z", "play or stop the station with that letter"},
			{"Enter Space", "play or stop the station under the cursor"},
			{"*", "play a random station from the list"},
			{arrows, "volume down or up"},
			{"Ctrl+B", "bookmark the station under the cursor, or the one playing; again to remove"},
			{"Ctrl+R", "shuffle: a random station every few minutes"},
			{"Ctrl+Z", "sleep timer: 15, 30, 45, 60, 90 minutes, off; fades out in the last minute"},
			{"Alt+1 to 9", "shuffle interval in minutes"},
		}},
		{"Finding stations", [][2]string{
			{"/ #", "tags"},
			{"a-z A-Z", "on the tags list: open the tag with that letter"},
			{"$ ^", "on the tags list: bookmarks, the last search"},
			{glyphs.up + " " + glyphs.down + " PgUp PgDn", "move through the list"},
			{"Tab", "switch between tags and stations (wide terminals)"},
			{"Esc", "go back; quits from the tags list"},
			{":", "search your stations"},
			{"Ctrl+F", "search your stations; press again to search online"},
			{"Ctrl+S", "search online; press again to search your stations"},
		}},
		{"More", [][2]string{
			{"?", "this help"},
			{"Ctrl+P", "control goradion from your phone"},
			{"Ctrl+T", "colour theme"},
			{"Mouse", "click a station to play it, scroll lists; scroll or click the volume gauge"},
			{"", "click the sleep countdown to turn the timer off"},
		}},
	}
	var sb strings.Builder
	sb.WriteString(boldTag(colorAccent) + VersionString() + "[-::-]\n")
	for _, sec := range sections {
		sb.WriteString("\n[::b]" + sec.title + "[::-]\n")
		for _, k := range sec.keys {
			fmt.Fprintf(&sb, "  %s%-15s[-] %s\n", fgTag(colorAccent), k[0], k[1])
		}
	}
	sb.WriteString("\n[::b]Settings[::-]\n  " + tview.Escape(configFile()) + "\n")
	return sb.String()
}
