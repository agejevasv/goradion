package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/agejevasv/goradion/internal/logging"
	"github.com/agejevasv/goradion/internal/mpv"
	"github.com/agejevasv/goradion/internal/radio"
)

var cfg = flag.String("s", "", "A link or a path to a stations.csv file")
var ver = flag.Bool("v", false, "Show the version number and quit")
var dbg = flag.Bool("d", false, "Enable debug log (goradion.log file in a current dir)")
var chk = flag.Bool("c", false, "")
var port = flag.Int("p", 7373, "Preferred port for the remote control web server (Ctrl+P)")
var ascii = flag.Bool("ascii", false, "Draw plain ASCII symbols, for terminals without Unicode fonts")
var noVU = flag.Bool("no-vu", false, "Hide the audio meter")
var remote remoteFlag

func init() {
	flag.Var(&remote, "r", "Start the remote control web server at launch, with an optional access `key` (\"\" for none)")
}

func main() {
	flag.CommandLine.Parse(markOptionalValue(os.Args[1:], "r"))

	if *ver {
		fmt.Println(radio.VersionString())
		os.Exit(0)
	}

	if err := logging.Init(*dbg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	stations, err := radio.LoadStations(*cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if len(stations) == 0 {
		fmt.Println("Stations list is empty, exiting.")
		os.Exit(0)
	}

	if *chk {
		radio.CheckStations(stations)
		os.Exit(0)
	}

	player := mpv.New()
	if *noVU {
		player.DisableVU()
	}
	if err := player.Start(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer player.Quit()

	options := []radio.Option{radio.WithASCII(*ascii)}
	if remote.on {
		options = append(options, radio.WithRemote(remote.key))
	}

	if err := radio.NewApp(player, stations, *port, options...).Run(); err != nil {
		player.Quit()
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// keyPrefix marks a value given on the command line, so that remoteFlag tells
// "-r" (the flag package passes "true") from "-r true" or "-r=".
const keyPrefix = "key:"

// remoteFlag is "-r [key]". A missing key leaves key nil.
type remoteFlag struct {
	on  bool
	key *string
}

func (f *remoteFlag) String() string   { return "" }
func (f *remoteFlag) IsBoolFlag() bool { return true }

func (f *remoteFlag) Set(v string) error {
	f.on, f.key = true, nil
	if key, ok := strings.CutPrefix(v, keyPrefix); ok {
		f.key = &key
	}
	return nil
}

// markOptionalValue rewrites "-name value" and "-name=value" to
// "-name=key:value". The flag package has no optional values: a bool flag
// never consumes the next argument. A value starting with "-" must be joined
// with "=".
func markOptionalValue(args []string, name string) []string {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" || arg == "-" || !strings.HasPrefix(arg, "-") {
			return append(out, args[i:]...)
		}
		flagName, value, hasValue := strings.Cut(strings.TrimPrefix(strings.TrimPrefix(arg, "-"), "-"), "=")
		switch {
		case flagName == name && hasValue:
			arg = "-" + name + "=" + keyPrefix + value
		case flagName == name && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-"):
			i++
			arg = "-" + name + "=" + keyPrefix + args[i]
		case !hasValue && takesValue(flagName) && i+1 < len(args):
			out = append(out, arg)
			i++
			arg = args[i]
		}
		out = append(out, arg)
	}
	return out
}

func takesValue(name string) bool {
	f := flag.Lookup(name)
	if f == nil {
		return false
	}
	b, ok := f.Value.(interface{ IsBoolFlag() bool })
	return !ok || !b.IsBoolFlag()
}
