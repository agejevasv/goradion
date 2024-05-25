package radio

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"runtime"
	"strings"
)

const defaultStationsURL = "https://gist.githubusercontent.com/agejevasv/" +
	"58afa748a7bc14dcccab1ca237d14a0b/raw/stations.csv"

const defaultStationsCSV = `SomaFM: Secret Agent,https://somafm.com/secretagent130.pls
	SomaFM: Beat Blender,https://somafm.com/beatblender.pls
	SomaFM: Bossa Beyond,https://somafm.com/bossa256.pls
	SomaFM: Groove Salad Classic,https://somafm.com/gsclassic130.pls
	SomaFM: DEF CON Radio,http://somafm.com/defcon.pls
	SomaFM: Fluid,http://somafm.com/fluid130.pls
	SomaFM: Cliqhop IDM,https://somafm.com/cliqhop130.pls
	SomaFM: Illinois Street Lounge,http://somafm.com/illstreet.pls
	SomaFM: Underground 80s,https://somafm.com/u80s130.pls
	SomaFM: Vaporwaves,http://somafm.com/vaporwaves.pls
	SomaFM: Drone Zone,http://somafm.com/dronezone.pls
	9128live,https://streams.radio.co/s0aa1e6f4a/listen
	Chillsky,https://lfhh.radioca.st/stream
	Nightride,https://stream.nightride.fm/nightride.ogg
	Jungletrain.net,http://stream1.jungletrain.net:8000
	Lounge Motion FM,https://vm.motionfm.com/motionthree_aacp
	Smooth Motion FM,https://vm.motionfm.com/motiontwo_aacp
	Seeburg 1000,https://psn3.prostreaming.net/proxy/seeburg/stream/;
	Classic Vinyl,http://icecast.walmradio.com:8000/classic
	Jazz Groove,https://audio-edge-cmc51.fra.h.radiomast.io/f0ac4bf3-bbe5-4edb-b828-193e0fdc4f2f
	Jazz24,https://prod-52-201-196-36.amperwave.net/ppm-jazz24aac256-ibc1
	HiRes: City Radio Smooth & Jazz,http://cityradio.ddns.net:8000/cityradio48flac
	HiRes: Radio Paradise,https://stream.radioparadise.com/flacm
	HiRes: Radio Paradise Mellow,https://stream.radioparadise.com/mellow-flacm
	HiRes: Radio Paradise Rock,https://stream.radioparadise.com/rock-flacm
	Classic Rock Florida,https://vip2.fastcast4u.com/proxy/classicrockdoug?mp=/1`

func Stations(sta string) ([]string, []string) {
	var scanner *bufio.Scanner

	if sta == "" {
		go cacheDefaultStations()
		if s, err := cachedDefaultStations(); err != nil {
			scanner = bufio.NewScanner(strings.NewReader(defaultStationsCSV))
		} else {
			scanner = bufio.NewScanner(strings.NewReader(string(s)))
		}
	} else if strings.HasPrefix(sta, "http") {
		s, err := fetchStations(sta)
		if err != nil {
			s = defaultStationsCSV
		}
		scanner = bufio.NewScanner(strings.NewReader(s))
	} else {
		file, err := os.Open(sta)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		defer file.Close()
		scanner = bufio.NewScanner(file)
	}

	stat := make([]string, 0)
	urls := make([]string, 0)

	for scanner.Scan() {
		d := strings.Split(scanner.Text(), ",")
		if len(d) != 2 {
			log.Println("Wrong stations entry:", scanner.Text())
			continue
		}
		stat = append(stat, strings.Trim(d[0], " 	"))
		urls = append(urls, strings.Trim(d[1], " 	"))
	}

	if err := scanner.Err(); err != nil {
		panic(err)
	}

	return stat, urls
}

func cacheDefaultStations() {
	s, err := fetchStations(defaultStationsURL)

	if err != nil {
		log.Println(err)
		return
	}

	ioutil.WriteFile(cacheFileName(), []byte(s), 0644)
}

func cacheFileName() string {
	dir := "/tmp"

	if runtime.GOOS == "windows" {
		dir, _ = os.UserHomeDir()
	}

	return path.Join(dir, "goradion.csv")
}

func cachedDefaultStations() ([]byte, error) {
	return ioutil.ReadFile(cacheFileName())
}

func fetchStations(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(data), nil
}
