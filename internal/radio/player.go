package radio

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"
)

const (
	defaultVolume = 80
	buffering     = "Buffering..."
	stopped       = "Stopped"
	playing       = "Playing"
)

type Player struct {
	sync.Mutex
	Info chan Info
	cmd  *exec.Cmd
	info *Info
}

type Info struct {
	Status   string
	Station  string
	Song     string
	PrevSong string
	Url      string
	Volume   int
}

func NewPlayer() *Player {
	return &Player{
		Info: make(chan Info),
		info: &Info{
			Volume: defaultVolume,
		},
	}
}

func (p *Player) Start() {
	p.Lock()
	p.cmd = exec.Command(
		"mpv",
		"-no-video",
		"--idle",
		"--display-tags=Artist,Title,icy-title",
		fmt.Sprintf("--volume=%d", defaultVolume),
		fmt.Sprintf("--input-ipc-server=%s", socket),
	)

	if err := p.cmd.Start(); err != nil {
		fmt.Println(err)
		fmt.Println("Please make sure 'mpv' is available.")
		fmt.Println("Install it using your package manager or visit https://mpv.io for more info.")
		os.Exit(1)
	}

	for i := 1; !mpvIsListening() && i <= 10; i++ {
		if i == 10 {
			fmt.Println("mpv failed to start, quitting")
			os.Exit(1)
		}
		log.Printf("waiting for mpv +%d ms\n", 8<<i)
		time.Sleep((8 << i) * time.Millisecond)
	}
	p.Unlock()

	go p.readMPVEvents()
}

func (p *Player) VolumeUp() {
	p.Lock()
	defer p.Unlock()

	defer func() {
		p.Info <- *p.info
	}()

	if p.info.Volume == 100 {
		return
	}

	log.Printf("setting volume %d\n", p.info.Volume+5)
	cmd := fmt.Sprintf(`{"command": ["set_property", "volume", %d]}%s`, p.info.Volume+5, "\n")
	writeToMPV([]byte(cmd))
	p.info.Volume += 5
}

func (p *Player) VolumeDn() {
	p.Lock()
	defer p.Unlock()

	defer func() {
		p.Info <- *p.info
	}()

	if p.info.Volume == 0 {
		return
	}

	log.Printf("setting volume %d\n", p.info.Volume-5)
	cmd := fmt.Sprintf(`{"command": ["set_property", "volume", %d]}%s`, p.info.Volume, "\n")
	writeToMPV([]byte(cmd))
	p.info.Volume -= 5
}

func (p *Player) Toggle(station, url string) {
	p.Lock()
	defer p.Unlock()

	if p.info.Url != "" {
		p.Stop()

		if url == p.info.Url {
			p.info.PrevSong = ""
			p.info.Url = ""
			return
		}
	}

	p.info.Station = station
	p.info.Status = buffering
	p.info.Song = ""
	p.Info <- *p.info

	p.Load(url)
}

func (p *Player) Stop() {
	log.Printf("stopping %s\n", p.info.Url)
	cmd := fmt.Sprintf(`{"command": ["stop"]}%s`, "\n")
	writeToMPV([]byte(cmd))
	p.info.Status = stopped
	p.info.Song = ""
	p.Info <- *p.info
}

func (p *Player) Load(url string) {
	log.Printf("loading %s\n", url)
	cmd := fmt.Sprintf(`{"command": ["loadfile", "%s"]}%s`, url, "\n")
	writeToMPV([]byte(cmd))
	p.info.Url = url
}

func (p *Player) Quit() {
	log.Println("quitting mpv")
	cmd := fmt.Sprintf(`{"command": ["quit", 9]}%s`, "\n")

	if ok := writeToMPV([]byte(cmd)); !ok && p.cmd != nil {
		log.Println("mpv failed to quit via socket")
		p.cmd.Process.Signal(os.Kill)
		p.cmd.Wait()
	}
}

func (p *Player) readMPVEvents() {
	c, err := netDial()
	if err != nil {
		log.Println(err)
		return
	}
	defer c.Close()

	cmd := fmt.Sprintf(`{"command": ["observe_property", 1, "filtered-metadata"]}%s`, "\n")

	if _, err = c.Write([]byte(cmd)); err != nil {
		log.Println(err)
	}

	for {
		eventBytes, err := bufio.NewReader(c).ReadBytes([]byte("\n")[0])

		if err != nil {
			log.Println(err)
			continue
		}

		r := unmarshal(eventBytes)
		log.Println(r)

		if r["event"] != nil && r["event"].(string) == "playback-restart" && p.info.Status == buffering {
			p.setStatusPlaying()
		}

		if r["reason"] != nil && (r["reason"].(string) == "eof" || r["reason"].(string) == "error") {
			p.setStatusUnexpectedEndFile(r["reason"].(string))
		}

		if data := r["data"]; data != nil {
			p.setCurrentSong(data.(map[string]any))
		}
	}
}

func (p *Player) setStatusPlaying() {
	p.Lock()
	p.info.Status = playing
	p.info.Song = ""
	p.Info <- *p.info
	p.Unlock()
}

func (p *Player) setStatusUnexpectedEndFile(reason string) {
	p.Lock()
	p.info.Status = fmt.Sprintf("Network or stream issues: %s", reason)
	p.info.Song = ""
	p.Info <- *p.info
	p.Unlock()
}

func (p *Player) setCurrentSong(m map[string]any) {
	log.Println(m)

	title, ok := m["icy-title"]

	song := ""

	if ok {
		song = title.(string)
	}

	artist, ok1 := m["Artist"]
	title, ok2 := m["Title"]

	if ok1 && ok2 {
		song = fmt.Sprintf("%s - %s", artist.(string), title.(string))
	}

	p.Lock()
	defer p.Unlock()

	if song != "" && song != p.info.PrevSong {
		p.info.PrevSong = song
		p.info.Status = ""
		p.info.Song = song
		p.Info <- *p.info
	}
}

func writeToMPV(data []byte) bool {
	c, err := netDial()

	if err != nil {
		log.Println(err, string(data))
		return false
	}

	defer c.Close()

	if _, err = c.Write(data); err != nil {
		log.Println(err, string(data))
		return false
	}

	return true
}

func mpvIsListening() bool {
	_, err := netDial()
	return err == nil
}

func unmarshal(data []byte) map[string]any {
	res := make(map[string]any)

	if err := json.Unmarshal(data, &res); err != nil {
		log.Println(err)
	}

	return res
}
