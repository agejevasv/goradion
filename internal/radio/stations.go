package radio

import (
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

const defaultStationsCSV = `SomaFM: Secret Agent,https://somafm.com/secretagent130.pls,Downtempo;Lounge
	SomaFM: Beat Blender,https://somafm.com/beatblender.pls,House;Downtempo;Electronic
	SomaFM: Bossa Beyond,https://somafm.com/bossa256.pls,Jazz;Lounge
	SomaFM: Groove Salad,https://somafm.com/nossl/groovesalad256.pls,Downtempo;Electronic
	SomaFM: Groove Salad Classic,https://somafm.com/gsclassic130.pls,Downtempo;Electronic
	SomaFM: DEF CON Radio,https://somafm.com/defcon256.pls,Electronic
	SomaFM: Fluid,http://somafm.com/fluid130.pls,Electronic
	SomaFM: Cliqhop IDM,https://somafm.com/cliqhop256.pls,Electronic
	SomaFM: Illinois Street Lounge,http://somafm.com/illstreet.pls,Oldies;Lounge
	SomaFM: Vaporwaves,http://somafm.com/vaporwaves.pls,Electronic
	SomaFM: Department Store Christmas,https://somafm.com/nossl/deptstore256.pls,Xmas
	SomaFM: Tiki Time,https://somafm.com/nossl/tikitime256.pls,Lounge;Oldies
	SomaFM: Synphaera,https://somafm.com/nossl/synphaera256.pls,Ambient;Electronic
	SomaFM: Drone Zone,https://somafm.com/dronezone256.pls,Ambient
	SomaFM: The Dark Zone,https://somafm.com/nossl/darkzone256.pls,Ambient
	SomaFM: Boot Liquor,https://somafm.com/nossl/bootliquor320.pls,Country;Oldies
	SomaFM: Seven Inch Soul,https://somafm.com/nossl/7soul.pls,Oldies;Soul
	SomaFM: Left Coast 70s,https://somafm.com/nossl/seventies320.pls,Rock;Oldies
	SomaFM: Lush,https://somafm.com/nossl/lush.pls,Electronic;Lounge;Electronic
	SomaFM: Underground 80s,https://somafm.com/nossl/u80s256.pls,80s
	SomaFM: Deep Space One,https://somafm.com/nossl/deepspaceone.pls,Ambient
	SomaFM: Space Station,https://somafm.com/nossl/spacestation.pls,Ambient
	SomaFM: Heavyweight Reggae,https://somafm.com/nossl/reggae256.pls,Reggae
	SomaFM: Doomed,https://somafm.com/nossl/doomed256.pls,Ambient
	SomaFM: Metal Detector,https://somafm.com/nossl/metal.pls,Metal
	SomaFM: Christmas Lounge,https://somafm.com/christmas256.pls,Xmas
	9128live,https://streams.radio.co/s0aa1e6f4a/listen,Electronic;Ambient
	Nightride,https://stream.nightride.fm/nightride.ogg,Electronic
	Jungletrain.net,http://stream1.jungletrain.net:8000,Electronic;Jungle
	Futuredrumz,https://orion.shoutca.st/tunein/futuredr.pls,Electronic;Jungle
	Bassdrive,http://ice.bassdrive.net/stream,Electronic;Jungle
	DNBRadio,https://azura.drmnbss.org:8000/radio.mp3,Electronic;Jungle
	Plushrecs Atmospheric,https://azrelay.drmnbss.org/listen/plushrecs/radio.mp3,Electronic;Jungle
	Section8Recs,https://azura.drmnbss.org:8020/radio.mp3,Electronic;Jungle
	KoolFM,https://admin.stream.rinse.fm/proxy/kool/stream,Electronic;Jungle
	RinseFM,https://admin.stream.rinse.fm/proxy/rinse_uk/stream,Electronic
	Deepinside Radio Show,https://n44a-eu.rcs.revma.com/uyrbt6xuhnruv,House;Electronic
	Deepinside Guest Sessions,https://n30a-eu.rcs.revma.com/u62vcepz3tzuv,House;Electronic
	Deep Vibes Radio,http://88.208.218.19:9106/listen.pls,House;Electronic
	Dogglounge,https://dogglounge.com/listen.pls,House;Electronic
	Deep Motion FM,https://vm.motionfm.com/motionone_free,House;Electronic
	Lounge Motion FM,https://vm.motionfm.com/motionthree_free,Downtempo;Lounge
	Smooth Motion FM,https://vm.motionfm.com/motiontwo_free,Soul;Lounge
	Magic Radio,http://mp3.magic-radio.net/,80s;Pop
	Jammin Vibez Radio,https://azuracast.jammimvibez.com/listen/classics/stream,Reggae
	Classic Rock Florida,https://vip2.fastcast4u.com/proxy/classicrockdoug?mp=/1,Rock
	Classic Vinyl,http://icecast.walmradio.com:8000/classic,Jazz;Oldies;Jazz
	The Jazz Groove: Mix #1,http://east-mp3-128.streamthejazzgroove.com/stream,Jazz;Instrumental
	The Jazz Groove: Mix #2,http://west-mp3-128.streamthejazzgroove.com/stream,Jazz;Instrumental
	Jazz24,https://live.amperwave.net/direct/ppm-jazz24aac256-ibc1,Jazz;Instrumental
	Linn Radio, http://radio.linn.co.uk:8003/autodj,Eclectic
	Linn Jazz,http://radio.linn.co.uk:8000/autodj,Jazz;Instrumental
	Linn Classical,http://radio.linn.co.uk:8004/autodj,Classical;Instrumental
	Mother Earth Radio,https://motherearth.streamserver24.com/listen/motherearth/motherearth.aac,Eclectic;Pop
	Mother Earth Jazz,https://motherearth.streamserver24.com/listen/motherearth_jazz/motherearth.jazz.mp4,Jazz;Instrumental
	Mother Earth Instrumental,https://motherearth.streamserver24.com/listen/motherearth_instrumental/motherearth.instrumental.aac,Instrumental
	Mother Earth Klassic,https://motherearth.streamserver24.com:18910/motherearth.klassik.aac,Classical;Instrumental
	FluxFM: Jazzradio Schwarzenstein,https://streams.fluxfm.de/jazzschwarz/mp3-320/audio/,Jazz;Instrumental
	FluxFM: Xjazz,https://streams.fluxfm.de/xjazz/mp3-320/audio/,Electronic;Jazz;Eclectic
	FluxFM: Chillhop,https://streams.fluxfm.de/Chillhop/mp3-320/streams.fluxfm.de/,Chillhop;Downtempo
	FluxFM: Chillout Radio,https://streams.fluxfm.de/chillout/mp3-320/streams.fluxfm.de/play.pls,Lounge
	FluxFM: Electronic Chillout,https://streams.fluxfm.de/klubradio/mp3-320/streams.fluxfm.de/play.pls,Lounge;House;Electronic
	FluxFM: Lounge,https://streams.fluxfm.de/lounge/mp3-320/streams.fluxfm.de/play.pls,Lounge;Eclectic
	FluxFM: Yoga Sounds,https://streams.fluxfm.de/yogasounds/mp3-320/streams.fluxfm.de/play.pls,Ambient
	FluxFM: HipHop Classics,https://streams.fluxfm.de/boomfmclassics/mp3-320/streams.fluxfm.de/play.pls,HipHop;Downtempo
	FluxFM: 60s,https://streams.fluxfm.de/60er/mp3-320/streams.fluxfm.de/play.pls,Oldies;Rock
	FluxFM: 80s,https://streams.fluxfm.de/80er/mp3-320/streams.fluxfm.de/play.pls,80s;Pop
	FluxFM: Finest,https://streams.fluxfm.de/fluxkompensator/mp3-320/streams.fluxfm.de/play.pls,Eclectic;Pop
	FluxFM: Berlin Beach House,https://streams.fluxfm.de/bbeachhouse/mp3-320/streams.fluxfm.de/play.pls,House;Electronic
	FluxFM: NeoFM,https://streams.fluxfm.de/neofm/mp3-320/streams.fluxfm.de/play.pls,Electronic
	Naim Radio,http://mscp3.live-streams.nl:8360/high.aac,Eclectic
	Naim Jazz,http://mscp3.live-streams.nl:8340/jazz-high.aac,Jazz
	Naim Classical,http://mscp3.live-streams.nl:8250/class-high.aac,Classical;Instrumental
	Radio Paradise,http://stream.radioparadise.com/aac-320,Eclectic;Pop
	Radio Paradise Mellow,http://stream.radioparadise.com/mellow-320,Pop;Eclectic
	Radio Paradise Rock,http://stream.radioparadise.com/rock-320,Rock
	HiRes: JB Radio2,http://161.97.135.80:8001/flac,Rock;Eclectic;Hi-Res
	HiRes: SuperStereo 1: Yacht Rock,http://icecast.centaury.cl:7570/SuperStereoHiRes1,Rock;Pop;Hi-Res
	HiRes: SuperStereo 2: 50s 60s 70s,http://icecast.centaury.cl:7570/SuperStereoHiRes2,Oldies;Eclectic;Hi-Res
	HiRes: SuperStereo 3: 80s,http://icecast.centaury.cl:7570/SuperStereoHiRes3,80s;Pop;Hi-Res
	HiRes: SuperStereo 4: Ballads 80s 90s 00s,http://icecast.centaury.cl:7570/SuperStereoHiRes4,Pop;Eclectic;Hi-Res
	HiRes: SuperStereo 5: Rock,http://icecast.centaury.cl:7570/SuperStereoHiRes5,Rock;Hi-Res
	HiRes: SuperStereo 6: Instrumental Music,http://icecast.centaury.cl:7570/SuperStereoHiRes6,Instrumental;Hi-Res
	HiRes: SuperStereo 7: Jazz,http://icecast.centaury.cl:7570/SuperStereoHiRes7,Jazz;Instrumental;Hi-Res
	HiRes: MaXXima,http://maxxima.mine.nu:8000/maxx.ogg,House;Electronic;Hi-Res
	HiRes: Radio Sputnik Underground!,https://radiosputnik.nl:8443/flac,Electronic;House;Hi-Res
	Rainwave Game,https://rainwave.cc/tune_in/1.mp3.m3u,Soundtrack
	Rainwave OC Remix,https://rainwave.cc/tune_in/2.mp3.m3u,Soundtrack
	Rainwave Cover,https://rainwave.cc/tune_in/3.mp3.m3u,Soundtrack
	Rainwave Chiptune,https://rainwave.cc/tune_in/4.mp3.m3u,Soundtrack
	Rainwave All,https://rainwave.cc/tune_in/5.mp3.m3u,Soundtrack
	freeCodeCamp: Code Radio,https://coderadio-admin-v2.freecodecamp.org/listen/coderadio/radio.mp3,Chillhop;Downtempo
	ChristmasFM: Classics,https://christmasfm.cdnstream1.com/2550_128.mp3,Xmas
	Radio Santa Claus,https://streaming.radiostreamlive.com/radiosantaclaus_devices,Xmas
	Radio Swiss Classic,http://stream.srg-ssr.ch/m/rsc_fr/mp3_128,Classical;Instrumental
	Radio Swiss Jazz,http://stream.srg-ssr.ch/m/rsj/mp3_128,Jazz;Instrumental
	Radio Swiss Pop,https://stream.srg-ssr.ch/m/rsp/mp3_128,Eclectic;Pop
	HiOnLine: Classic,https://mediaserv30.live-streams.nl:18088,Classical;Instrumental
	HiOnLine: Lounge,https://mediaserv33.live-streams.nl:18036,Lounge;Downtempo
	HiOnLine: Jazz,https://mediaserv38.live-streams.nl:18006,Jazz
	HiOnLine: Gold,https://mediaserv30.live-streams.nl:18000,Rock;Oldies
	HiOnLine: Pop,http://mediaserv30.live-streams.nl:2199/tunein/hionline.pls,Pop
	HiOnLine: World,http://mediaserv38.live-streams.nl:2199/tunein/onlineworldradio.pls,Pop
	KCRW Eclectic24,https://streams.kcrw.com/e24_mp3,Eclectic;Pop
	Litt: Rock X,https://ice55.securenetsystems.net/DASH38,Rock
	Litt: Monsters Of Rock,https://ice55.securenetsystems.net/DASH14,Rock
	Litt: Yacht Rock,https://ice55.securenetsystems.net/DASH41,Pop;Rock;Soul
	Litt: The Strip,https://ice55.securenetsystems.net/DASH29,Rock
	Litt: Disco Fever,https://ice55.securenetsystems.net/DASH20,Disco
	Litt: 60s,https://ice55.securenetsystems.net/DASH34,Pop;Oldies
	Litt: 70s,https://ice55.securenetsystems.net/DASH26,Pop;Oldies
	Litt: 80s,https://ice55.securenetsystems.net/DASH46,Pop;80s
	Litt: 90s,https://ice55.securenetsystems.net/DASH42,Pop
	Litt: 00s,https://ice55.securenetsystems.net/DASH19,Pop
	Litt: 10s+,https://ice55.securenetsystems.net/DASH52,Pop
	Litt: Smooth Jazz Hits,https://ice55.securenetsystems.net/DASH44,Jazz;Lounge
	Litt: Hip-Hop X,https://ice65.securenetsystems.net/DASH5,HipHop
	Litt: Boomerang,https://ice55.securenetsystems.net/DASH39,Pop;R&B
	Litt: R&B X,https://ice55.securenetsystems.net/DASH47,Pop;R&B
	Litt: Hits X,https://ice55.securenetsystems.net/DASH48,Pop
	Litt: Pop X,https://ice55.securenetsystems.net/DASH17,Pop
	Aardvark Blues FM,http://streaming.live365.com/b77280_128mp3,Blues
	Houston Blues Radio,http://streaming.live365.com/b76353_128mp3,Blues
	Blues Radio,https://i4.streams.ovh/sc/bluesrad/stream,Blues
	DeathFM,http://hi5.death.fm,Metal
	1980sFM,http://hi5.1980s.fm,80s;Pop
	StreamingSoundtracks,http://hi5.streamingsoundtracks.com,Soundtrack
	Entranced,http://hi5.entranced.fm,Lounge;Electronic
	AdagioFm,http://hi5.adagio.fm,Classical;Instrumental
	Rock Radio 1,http://144.217.77.176:8000/listen.pls,Rock;Metal
	VFR: 80s Thrash,https://tuneintoradio1.com:8010/radio.mp3,Metal
	VFR: Thrash,https://tuneintoradio1.com:8000/radio.mp3,Metal`

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
