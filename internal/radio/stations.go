package radio

import (
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

const defaultStationsCSV = `FluxFM: Jazzradio Schwarzenstein,https://streams.fluxfm.de/jazzschwarz/mp3-320/audio/,Jazz;Instrumental
	FluxFM: Xjazz,https://streams.fluxfm.de/xjazz/mp3-320/audio/,Electronic;Jazz;Eclectic
	FluxFM: Chillhop,https://streams.fluxfm.de/Chillhop/mp3-320/streams.fluxfm.de/,Chillhop;Downtempo
	FluxFM: Chillout Radio,https://streams.fluxfm.de/chillout/mp3-320/streams.fluxfm.de/play.pls,Lounge
	FluxFM: Electronic Chillout,https://streams.fluxfm.de/klubradio/mp3-320/streams.fluxfm.de/play.pls,Lounge;House;Electronic
	FluxFM: Lounge,https://streams.fluxfm.de/lounge/mp3-320/streams.fluxfm.de/play.pls,Lounge;Eclectic
	FluxFM: Yoga Sounds,https://streams.fluxfm.de/yogasounds/mp3-320/streams.fluxfm.de/play.pls,Ambient
	FluxFM: HipHop Classics,https://streams.fluxfm.de/boomfmclassics/mp3-320/streams.fluxfm.de/play.pls,HipHop;Downtempo
	FluxFM: 60s,https://streams.fluxfm.de/60er/mp3-320/streams.fluxfm.de/play.pls,Oldies;Rock;Pop
	FluxFM: 70s,https://streams.fluxfm.de/70er/mp3-320/streams.fluxfm.de/play.pls,Oldies;Rock;Pop
	FluxFM: 80s,https://streams.fluxfm.de/80er/mp3-320/streams.fluxfm.de/play.pls,80s;Pop
	FluxFM: 00s,https://streams.fluxfm.de/flx_2000/mp3-320/streams.fluxfm.de/play.pls,Pop
	FluxFM: Finest,https://streams.fluxfm.de/fluxkompensator/mp3-320/streams.fluxfm.de/play.pls,Eclectic;Pop
	FluxFM: Berlin Beach House,https://streams.fluxfm.de/bbeachhouse/mp3-320/streams.fluxfm.de/play.pls,House;Electronic
	FluxFM: NeoFM,https://streams.fluxfm.de/neofm/mp3-320/streams.fluxfm.de/play.pls,Electronic
	FluxFM: ElektroFlux,https://streams.fluxfm.de/elektro/mp3-320/streams.fluxfm.de/play.pls,Pop;Electronic
	FluxFM: B-Funk,https://streams.fluxfm.de/event01/mp3-320/streams.fluxfm.de/play.pls,Soul;Funk
	FluxFM: Indie Disco,https://streams.fluxfm.de/indiedisco/mp3-320/streams.fluxfm.de/play.pls,Pop
	FluxFM: John Reed Radio,https://streams.fluxfm.de/john-reed/mp3-320/streams.fluxfm.de/play.pls,Electronic;House;Pop
	FluxFM: MetalFM,https://streams.fluxfm.de/metalfm/mp3-320/streams.fluxfm.de/play.pls,Metal
	Linn Radio, http://radio.linn.co.uk:8003/autodj,Eclectic
	Linn Jazz,http://radio.linn.co.uk:8000/autodj,Jazz;Instrumental
	Linn Classical,http://radio.linn.co.uk:8004/autodj,Classical;Instrumental
	Mother Earth Radio,https://motherearth.streamserver24.com/listen/motherearth/motherearth.aac,Eclectic;Pop
	Mother Earth Jazz,https://motherearth.streamserver24.com/listen/motherearth_jazz/motherearth.jazz.mp4,Jazz;Instrumental
	Mother Earth Instrumental,https://motherearth.streamserver24.com/listen/motherearth_instrumental/motherearth.instrumental.aac,Instrumental
	Mother Earth Klassic,https://motherearth.streamserver24.com/listen/motherearth_klassik/motherearth.klassik.aac,Classical;Instrumental
	Naim Radio,http://mscp3.live-streams.nl:8360/high.aac,Eclectic
	Naim Jazz,http://mscp3.live-streams.nl:8340/jazz-high.aac,Jazz
	Naim Classical,http://mscp3.live-streams.nl:8250/class-high.aac,Classical;Instrumental
	Radio Paradise: Main,http://stream.radioparadise.com/aac-320,Eclectic;Pop
	Radio Paradise: Mellow,http://stream.radioparadise.com/mellow-320,Pop;Eclectic
	Radio Paradise: Rock,http://stream.radioparadise.com/rock-320,Rock
	Radio Paradise: Global,http://stream.radioparadise.com/global-320,Eclectic
	Radio Paradise: Beyond...,http://stream.radioparadise.com/beyond-320,Eclectic
	Radio Paradise: 2050,http://stream.radioparadise.com/radio2050-320,Eclectic
	Radio Swiss Classic,http://stream.srg-ssr.ch/m/rsc_fr/mp3_128,Classical;Instrumental
	Radio Swiss Jazz,http://stream.srg-ssr.ch/m/rsj/mp3_128,Jazz;Instrumental
	Radio Swiss Pop,https://stream.srg-ssr.ch/m/rsp/mp3_128,Eclectic;Pop
	HiOnLine: Classic,https://mediaserv30.live-streams.nl:18088,Classical;Instrumental
	HiOnLine: Lounge,https://mediaserv33.live-streams.nl:18036,Lounge;Downtempo
	HiOnLine: Jazz,https://mediaserv38.live-streams.nl:18006,Jazz
	HiOnLine: Gold,https://mediaserv30.live-streams.nl:18000,Rock;Oldies
	HiOnLine: Pop,http://mediaserv30.live-streams.nl:2199/tunein/hionline.pls,Pop
	HiOnLine: World,http://mediaserv38.live-streams.nl:2199/tunein/onlineworldradio.pls,Pop
	Motion FM: Deep,https://vm.motionfm.com/motionone_free,House;Electronic
	Motion FM: Lounge,https://vm.motionfm.com/motionthree_free,Downtempo;Lounge
	Motion FM: Smooth,https://vm.motionfm.com/motiontwo_free,Soul;Lounge
	SomaFM: Secret Agent,https://somafm.com/secretagent130.pls,Downtempo;Lounge
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
	Magic Radio,http://mp3.magic-radio.net/,80s;Pop
	Classic Rock Florida,https://vip2.fastcast4u.com/proxy/classicrockdoug?mp=/1,Rock
	Classic Vinyl,http://icecast.walmradio.com:8000/classic,Jazz;Oldies;Jazz
	Jazz24,https://live.amperwave.net/direct/ppm-jazz24aac256-ibc1,Jazz;Instrumental
	MaXXima,http://maxxima.mine.nu:8000/maxx.ogg,House;Electronic
	Radio Sputnik Underground!,https://radiosputnik.nl:8443/flac,Electronic;House
	freeCodeCamp: Code Radio,https://coderadio-admin-v2.freecodecamp.org/listen/coderadio/radio.mp3,Chillhop;Downtempo
	Chillsky,https://chill.radioca.st/stream,Chillhop;Downtempo
	ChristmasFM: Classics,https://christmasfm.cdnstream1.com/2550_128.mp3,Xmas
	Radio Santa Claus,https://streaming.radiostreamlive.com/radiosantaclaus_devices,Xmas
	KCRW Eclectic24,https://streams.kcrw.com/e24_mp3,Eclectic;Pop
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
	VFR: Thrash,https://tuneintoradio1.com:8000/radio.mp3,Metal
	Rainwave Game,https://rainwave.cc/tune_in/1.mp3.m3u,Soundtrack
	Rainwave OC Remix,https://rainwave.cc/tune_in/2.mp3.m3u,Soundtrack
	Rainwave Cover,https://rainwave.cc/tune_in/3.mp3.m3u,Soundtrack
	Rainwave Chiptune,https://rainwave.cc/tune_in/4.mp3.m3u,Soundtrack
	Rainwave All,https://rainwave.cc/tune_in/5.mp3.m3u,Soundtrack
	Ambient Sleeping Pill,http://radio.stereoscenic.com/asp-h,Ambient
	Classical KUSC,https://playerservices.streamtheworld.com/pls/KUSCMP256.pls,Classical;Instrumental
	WFMU,https://wfmu.org/wfmu_mp3.pls,Eclectic
	WFMU: Rock'n'Soul,https://wfmu.org/wfmu_rock.pls,Rock;Soul
	WFMU: Sheena's Jungle,https://wfmu.org/sheena.pls,Eclectic
	Worldwide FM,https://worldwidefm.out.airtime.pro/worldwidefm_b,Eclectic
	NTS: 1,https://stream-relay-geo.ntslive.net/stream,Eclectic
	NTS: 2,https://stream-relay-geo.ntslive.net/stream2,Eclectic
	NTS Mixtape: Slow Focus,https://stream-mixtape-geo.ntslive.net/mixtape,Ambient;Mix
	NTS Mixtape: Low Key,https://stream-mixtape-geo.ntslive.net/mixtape2,Downtempo;Mix
	NTS Mixtape: Expansions,https://stream-mixtape-geo.ntslive.net/mixtape3,Jazz;Mix
	NTS Mixtape: Poolside,https://stream-mixtape-geo.ntslive.net/mixtape4,Pop;Mix
	NTS Mixtape: 4 To The Floor,https://stream-mixtape-geo.ntslive.net/mixtape5,House;Electronic;Mix
	NTS Mixtape: Memory Lane,https://stream-mixtape-geo.ntslive.net/mixtape6,Rock;Mix
	NTS Mixtape: Island Time,https://stream-mixtape-geo.ntslive.net/mixtape21,Reggae;Mix
	NTS Mixtape: Rap House,https://stream-mixtape-geo.ntslive.net/mixtape22,HipHop;Mix
	NTS Mixtape: Sweat,https://stream-mixtape-geo.ntslive.net/mixtape24,Mix
	NTS Mixtape: The Tube,https://stream-mixtape-geo.ntslive.net/mixtape26,Mix
	NTS Mixtape: Feelings,https://stream-mixtape-geo.ntslive.net/mixtape27,Soul;Mix
	NTS Mixtape: Labyrinth,https://stream-mixtape-geo.ntslive.net/mixtape31,Mix
	NTS Mixtape: The Pit,https://stream-mixtape-geo.ntslive.net/mixtape34,Metal;Mix
	NTS Mixtape: Sheet Music,https://stream-mixtape-geo.ntslive.net/mixtape35,Instrumental;Mix
	NTS Mixtape: Otaku,https://stream-mixtape-geo.ntslive.net/mixtape36,Soundtrack;Mix`

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
