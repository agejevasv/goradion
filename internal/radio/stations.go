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
	SomaFM: DEF CON Radio,https://somafm.com/defcon256.pls
	SomaFM: Fluid,http://somafm.com/fluid130.pls
	SomaFM: Cliqhop IDM,https://somafm.com/cliqhop256.pls
	SomaFM: Illinois Street Lounge,http://somafm.com/illstreet.pls
	SomaFM: Vaporwaves,http://somafm.com/vaporwaves.pls
	SomaFM: Drone Zone,https://somafm.com/dronezone256.pls
	9128live,https://streams.radio.co/s0aa1e6f4a/listen
	Chillsky,https://lfhh.radioca.st/stream
	Nightride,https://stream.nightride.fm/nightride.ogg
	Jungletrain.net,http://stream1.jungletrain.net:8000
	Deepinside Radio Show,https://n44a-eu.rcs.revma.com/uyrbt6xuhnruv
	Deepinside Guest Sessions,https://n30a-eu.rcs.revma.com/u62vcepz3tzuv
	Deep Motion FM,https://vm.motionfm.com/motionone_free
	Lounge Motion FM,https://vm.motionfm.com/motionthree_free
	Smooth Motion FM,https://vm.motionfm.com/motiontwo_free
	Magic Radio,http://mp3.magic-radio.net/
	Classic Rock Florida,https://vip2.fastcast4u.com/proxy/classicrockdoug?mp=/1
	Classic Vinyl,http://icecast.walmradio.com:8000/classic
	The Jazz Groove: Mix #1,http://east-mp3-128.streamthejazzgroove.com/stream
	The Jazz Groove: Mix #2,http://west-mp3-128.streamthejazzgroove.com/stream
	Jazz24,https://live.amperwave.net/direct/ppm-jazz24aac256-ibc1
	Linn Jazz,http://radio.linn.co.uk:8000/autodj
	HiRes: City Radio Smooth & Jazz,http://cityradio.ddns.net:8000/cityradio48flac
	HiRes: Radio Paradise,https://stream.radioparadise.com/flacm
	HiRes: Radio Paradise Mellow,https://stream.radioparadise.com/mellow-flacm
	HiRes: Radio Paradise Rock,https://stream.radioparadise.com/rock-flacm
	HiRes: JB Radio2,https://maggie.torontocast.com:8076/flac
	HiRes: MaXXima,http://maxxima.mine.nu:8000/maxx.ogg
	HiRes: Radio Sputnik Underground!,https://radiosputnik.nl:8443/flac
	HiRes: SomaFM: Groove Salad,https://hls.somafm.com/hls/groovesalad/FLAC/program.m3u8
	HiRes: Naim Radio,http://mscp3.live-streams.nl:8360/flac.flac
	HiRes: Naim Jazz,http://mscp3.live-streams.nl:8340/jazz-flac.flac
	HiRes: Naim Classical,http://mscp3.live-streams.nl:8250/class-flac.flac
	HiRes: SuperStereo 1,http://icecast.centaury.cl:7570/SuperStereoHiRes1
	HiRes: SuperStereo 2,http://icecast.centaury.cl:7570/SuperStereoHiRes2
	HiRes: SuperStereo 3,http://icecast.centaury.cl:7570/SuperStereoHiRes3
	HiRes: SuperStereo 4,http://icecast.centaury.cl:7570/SuperStereoHiRes4
	HiRes: SuperStereo 5,http://icecast.centaury.cl:7570/SuperStereoHiRes5
	HiRes: SuperStereo 6,http://icecast.centaury.cl:7570/SuperStereoHiRes6
	HiRes: SuperStereo 7,http://icecast.centaury.cl:7570/SuperStereoHiRes7
	HiRes: ∏ano,https://stream.p-node.org/piano.flac
	Mother Earth Jazz,https://motherearth.streamserver24.com/listen/motherearth_jazz/motherearth.jazz.mp4
	Mother Earth Instrumental,https://motherearth.streamserver24.com/listen/motherearth_instrumental/motherearth.instrumental.aac
	Mother Earth Radio,https://motherearth.streamserver24.com/listen/motherearth/motherearth.aac
	Mother Earth Klassic,https://motherearth.streamserver24.com:18910/motherearth.klassik.aac
	Linn Classical,http://radio.linn.co.uk:8004/autodj
	Linn Radio: http://radio.linn.co.uk:8003/autodj
	Jammin Vibez Radio,https://azuracast.jammimvibez.com/listen/classics/stream
	Seeburg 1000,https://psn3.prostreaming.net/proxy/seeburg/stream/;`

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
