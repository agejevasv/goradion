package radio

import (
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

const defaultStationsURL = "https://gist.githubusercontent.com/agejevasv/" +
	"58afa748a7bc14dcccab1ca237d14a0b/raw/stations.csv"

const defaultStationsCSV = `SomaFM: Secret Agent,https://somafm.com/secretagent130.pls,Downtempo;Lounge
	SomaFM: Beat Blender,https://somafm.com/beatblender.pls,House;Downtempo;Electronic
	SomaFM: Bossa Beyond,https://somafm.com/bossa256.pls,Jazz;Lounge
	SomaFM: Groove Salad Classic,https://somafm.com/gsclassic130.pls,Downtempo;Electronic
	SomaFM: DEF CON Radio,https://somafm.com/defcon256.pls,Electronic
	SomaFM: Fluid,http://somafm.com/fluid130.pls,Electronic
	SomaFM: Cliqhop IDM,https://somafm.com/cliqhop256.pls,Electronic
	SomaFM: Illinois Street Lounge,http://somafm.com/illstreet.pls,Oldies;Lounge
	SomaFM: Vaporwaves,http://somafm.com/vaporwaves.pls,Vaporwave
	SomaFM: Drone Zone,https://somafm.com/dronezone256.pls,Ambient
	9128live,https://streams.radio.co/s0aa1e6f4a/listen,Electronic
	Chillsky,https://lfhh.radioca.st/stream,Chillhop;Downtempo
	Nightride,https://stream.nightride.fm/nightride.ogg,Synthwave;Electronic
	Jungletrain.net,http://stream1.jungletrain.net:8000,Drum And Bass;Electronic
	Deepinside Radio Show,https://n44a-eu.rcs.revma.com/uyrbt6xuhnruv,House;Electronic
	Deepinside Guest Sessions,https://n30a-eu.rcs.revma.com/u62vcepz3tzuv,House;Electronic
	Deep Motion FM,https://vm.motionfm.com/motionone_free,House;Electronic
	Lounge Motion FM,https://vm.motionfm.com/motionthree_free,Downtempo;Lounge;Electronic
	Smooth Motion FM,https://vm.motionfm.com/motiontwo_free,Soul;Lounge
	Magic Radio,http://mp3.magic-radio.net/,80s
	Jammin Vibez Radio,https://azuracast.jammimvibez.com/listen/classics/stream,Reggae
	Classic Rock Florida,https://vip2.fastcast4u.com/proxy/classicrockdoug?mp=/1,Rock
	Classic Vinyl,http://icecast.walmradio.com:8000/classic,Jazz;Oldies;Jazz
	The Jazz Groove: Mix #1,http://east-mp3-128.streamthejazzgroove.com/stream,Jazz;Instrumental
	The Jazz Groove: Mix #2,http://west-mp3-128.streamthejazzgroove.com/stream,Jazz;Instrumental
	Jazz24,https://live.amperwave.net/direct/ppm-jazz24aac256-ibc1,Jazz;Instrumental
	Linn Radio, http://radio.linn.co.uk:8003/autodj,Eclectic
	Linn Jazz,http://radio.linn.co.uk:8000/autodj,Jazz;Instrumental
	Linn Classical,http://radio.linn.co.uk:8004/autodj,Classical;Instrumental
	Mother Earth Radio,https://motherearth.streamserver24.com/listen/motherearth/motherearth.aac,Eclectic
	Mother Earth Jazz,https://motherearth.streamserver24.com/listen/motherearth_jazz/motherearth.jazz.mp4,Jazz;Instrumental
	Mother Earth Instrumental,https://motherearth.streamserver24.com/listen/motherearth_instrumental/motherearth.instrumental.aac,Instrumental
	Mother Earth Klassic,https://motherearth.streamserver24.com:18910/motherearth.klassik.aac,Classical;Instrumental
	FluxFM: Jazzradio Schwarzenstein,https://streams.fluxfm.de/jazzschwarz/mp3-320/audio/,Jazz;Instrumental
	FluxFM: Xjazz,https://streams.fluxfm.de/xjazz/mp3-320/audio/,Electronic;Jazz
	FluxFM: Chillhop,https://streams.fluxfm.de/Chillhop/mp3-320/streams.fluxfm.de/,Chillhop;Downtempo
	HiRes: Radio Paradise,https://stream.radioparadise.com/flacm,Eclectic
	HiRes: Radio Paradise Mellow,https://stream.radioparadise.com/mellow-flacm,Ballads;Eclectic
	HiRes: Radio Paradise Rock,https://stream.radioparadise.com/rock-flacm,Rock
	HiRes: JB Radio2,https://maggie.torontocast.com:8076/flac,Rock;Eclectic
	HiRes: Naim Radio,http://mscp3.live-streams.nl:8360/flac.flac,Eclectic
	HiRes: Naim Jazz,http://mscp3.live-streams.nl:8340/jazz-flac.flac,Jazz
	HiRes: Naim Classical,http://mscp3.live-streams.nl:8250/class-flac.flac,Classical;Instrumental
	HiRes: SuperStereo 1: Yacht Rock,http://icecast.centaury.cl:7570/SuperStereoHiRes1,Rock
	HiRes: SuperStereo 2: 50s 60s 70s,http://icecast.centaury.cl:7570/SuperStereoHiRes2,Oldies;Eclectic
	HiRes: SuperStereo 3: 80s,http://icecast.centaury.cl:7570/SuperStereoHiRes3,80s
	HiRes: SuperStereo 4: Ballads 80s 90s 00s,http://icecast.centaury.cl:7570/SuperStereoHiRes4,Ballads
	HiRes: SuperStereo 5: Rock,http://icecast.centaury.cl:7570/SuperStereoHiRes5,Rock
	HiRes: SuperStereo 6: Instrumental Music,http://icecast.centaury.cl:7570/SuperStereoHiRes6,Instrumental
	HiRes: SuperStereo 7: Jazz,http://icecast.centaury.cl:7570/SuperStereoHiRes7,Jazz;Instrumental
	HiRes: MaXXima,http://maxxima.mine.nu:8000/maxx.ogg,House;Electronic
	HiRes: Radio Sputnik Underground!,https://radiosputnik.nl:8443/flac,Electronic;House`

type Station struct {
	title string
	url   string
	tags  []string
}

func Stations(sta string) []Station {
	var reader *csv.Reader

	if sta == "" {
		reader = csv.NewReader(strings.NewReader(defaultStationsCSV))
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

	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()

	if err != nil {
		fmt.Println("Can't parse stations CSV:", err)
		os.Exit(1)
	}

	stations := make([]Station, 0)
	for _, r := range records {
		s := new(Station)
		s.title = strings.Trim(r[0], " 	")
		s.url = strings.Trim(r[1], " 	")
		if len(r) > 2 {
			s.tags = strings.Split(strings.Trim(r[2], " 	"), ";")
		}
		stations = append(stations, *s)
	}

	return stations
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
