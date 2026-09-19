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
			{"Ctrl+B", "bookmark the station under the cursor, or the one playing; again to remove"},
			{arrows, "volume down or up"},
			{"Ctrl+R", "shuffle: a random station every few minutes"},
			{"Alt+1 to 9", "shuffle interval in minutes"},
		}},
		{"Finding stations", [][2]string{
			{"/ #", "tags"},
			{"~", "all stations"},
			{"Ctrl+F :", "search your stations; press again to search online"},
			{"Ctrl+S", "search online at radio-browser.info"},
			{glyphs.up + " " + glyphs.down + " PgUp PgDn", "move through the list"},
			{"Tab", "switch between tags and stations (wide terminals)"},
			{"Esc", "go back; quits from the tags list"},
		}},
		{"More", [][2]string{
			{"Ctrl+P", "control goradion from your phone"},
			{"Ctrl+T", "colour theme"},
			{"?", "this help"},
			{"Mouse", "click a station to play it, scroll lists; scroll or click the volume gauge"},
		}},
		{"Command line", [][2]string{
			{"-ascii", "plain ASCII symbols for terminals without Unicode fonts"},
			{"-no-vu", "hide the audio meter"},
			{"-s file|url", "stations CSV to use"},
			{"-p port", "preferred port for the phone remote"},
			{"-r key", "start the phone remote at launch; key is optional, \"\" for none"},
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
