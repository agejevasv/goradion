package radio

import (
	"encoding/csv"
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
	Jammin Vibez Radio,https://azuracast.jammimvibez.com/listen/classics/stream
	Classic Rock Florida,https://vip2.fastcast4u.com/proxy/classicrockdoug?mp=/1
	Classic Vinyl,http://icecast.walmradio.com:8000/classic
	The Jazz Groove: Mix #1,http://east-mp3-128.streamthejazzgroove.com/stream
	The Jazz Groove: Mix #2,http://west-mp3-128.streamthejazzgroove.com/stream
	Jazz24,https://live.amperwave.net/direct/ppm-jazz24aac256-ibc1
	Linn Radio, http://radio.linn.co.uk:8003/autodj
	Linn Jazz,http://radio.linn.co.uk:8000/autodj
	Linn Classical,http://radio.linn.co.uk:8004/autodj
	Mother Earth Radio,https://motherearth.streamserver24.com/listen/motherearth/motherearth.aac
	Mother Earth Jazz,https://motherearth.streamserver24.com/listen/motherearth_jazz/motherearth.jazz.mp4
	Mother Earth Instrumental,https://motherearth.streamserver24.com/listen/motherearth_instrumental/motherearth.instrumental.aac
	Mother Earth Klassic,https://motherearth.streamserver24.com:18910/motherearth.klassik.aac
	FluxFM: Jazzradio Schwarzenstein,https://streams.fluxfm.de/jazzschwarz/mp3-320/audio/
	FluxFM: Xjazz,https://streams.fluxfm.de/xjazz/mp3-320/audio/
	FluxFM: Chillhop,https://streams.fluxfm.de/Chillhop/mp3-320/streams.fluxfm.de/
	HiRes: Radio Paradise,https://stream.radioparadise.com/flacm
	HiRes: Radio Paradise Mellow,https://stream.radioparadise.com/mellow-flacm
	HiRes: Radio Paradise Rock,https://stream.radioparadise.com/rock-flacm
	HiRes: JB Radio2,https://maggie.torontocast.com:8076/flac
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
	HiRes: MaXXima,http://maxxima.mine.nu:8000/maxx.ogg
	HiRes: Radio Sputnik Underground!,https://radiosputnik.nl:8443/flac`

func Stations(sta string) ([]string, []string) {
	var reader *csv.Reader

	if sta == "" {
		if s, err := cachedDefaultStations(); err != nil {
			reader = csv.NewReader(strings.NewReader(defaultStationsCSV))
		} else {
			reader = csv.NewReader(strings.NewReader(string(s)))
		}
	} else if strings.HasPrefix(sta, "http") {
		s, err := fetchStations(sta)
		if err != nil {
			s = defaultStationsCSV
		}
		reader = csv.NewReader(strings.NewReader(s))
	} else {
		file, err := os.Open(sta)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		defer file.Close()
		reader = csv.NewReader(file)
	}

	stat := make([]string, 0)
	urls := make([]string, 0)

	records, err := reader.ReadAll()
	if err != nil {
		fmt.Println("Can't parse stations CSV:", err)
		os.Exit(1)
	}

	for _, r := range records {
		stat = append(stat, strings.Trim(r[0], " 	"))
		urls = append(urls, strings.Trim(r[1], " 	"))
	}

	return stat, urls
}

func CacheDefaultStations() error {
	s, err := fetchStations(defaultStationsURL)

	if err != nil {
		return err
	}

	return ioutil.WriteFile(cacheFileName(), []byte(s), 0644)
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

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("Failed to GET %s, status code: %d", url, resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(data), nil
}
