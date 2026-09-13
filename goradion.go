package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/agejevasv/goradion/internal/radio"
)

var cfg = flag.String("s", "", "A link or a path to a stations.csv file")
var ver = flag.Bool("v", false, "Show the version number and quit")
var dbg = flag.Bool("d", false, "Enable debug log (goradion.log file in a current dir)")
var chk = flag.Bool("c", false, "")
var port = flag.Int("p", 7373, "Preferred port for the remote control web server (Ctrl+P)")
var ascii = flag.Bool("ascii", false, "Draw plain ASCII symbols, for terminals without Unicode fonts")
var noVU = flag.Bool("no-vu", false, "Hide the audio level meter")

func main() {
	flag.Parse()

	if *ver {
		fmt.Println(radio.VersionString())
		os.Exit(0)
	}

	radio.InitLog(*dbg)

	stations := radio.Stations(*cfg)
	if len(stations) == 0 {
		fmt.Println("Stations list is empty, exiting.")
		os.Exit(0)
	}

	if *chk {
		radio.CheckStations(stations)
		os.Exit(0)
	}

	player := radio.NewPlayer()
	if *noVU {
		player.DisableVU()
	}
	go player.Start()
	defer player.Quit()

	if err := radio.NewApp(player, stations, *port, radio.WithASCII(*ascii)).Run(); err != nil {
		panic(err)
	}
}
