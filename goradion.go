package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/agejevasv/goradion/internal/radio"
)

var cfg = flag.String("s", "", "A link or a path to a stations.csv file")
var ver = flag.Bool("v", false, "Show the version number and quit")
var dbg = flag.Bool("d", false, "Enable debug log (file)")

func main() {
	flag.Parse()

	if *ver {
		fmt.Println(radio.VersionString())
		os.Exit(0)
	}

	radio.InitLog(*dbg)

	stations, urls := radio.Stations(*cfg)
	if len(stations) == 0 {
		fmt.Println("Stations list is empty, exiting.")
		os.Exit(0)
	}

	player := radio.NewPlayer()
	go player.Start()
	defer player.Quit()

	if err := radio.NewApp(player, stations, urls).Run(); err != nil {
		panic(err)
	}
}
