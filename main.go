package main

import (
	"flag"

	"github.com/agejevasv/goradion/internal/radio"
)

func main() {
	cfg := flag.String("s", "", "link or path to a stations.csv file")
	flag.Parse()

	player := radio.NewPlayer()
	defer player.Stop()

	stations, urls := radio.Stations(*cfg)

	if err := radio.NewApp(player, stations, urls).Run(); err != nil {
		panic(err)
	}
}
