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

const defaultVolume = 80

type Player struct {
	sync.Mutex
	Info    chan Info
	cmd     *exec.Cmd
	info    *Info
	url     string
	volume  int
	stopped chan struct{}
}

type Info struct {
	Status string
	Song   string
	Volume int
}

func NewPlayer() *Player {
	return &Player{
		Info:   make(chan Info),
		volume: defaultVolume,
		info: &Info{
			Volume: defaultVolume,
		},
		stopped: make(chan struct{}),
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
	log.Println("mpv is ready")
}

func (p *Player) Toggle(station, url string) {
	p.Lock()
	defer p.Unlock()

	if p.url != "" {
		p.Stop()

		if url == p.url {
			p.url = ""
			return
		}
	}

	p.info.Status = station
	p.info.Song = "Loading..."
	p.info.Volume = p.volume
	p.Info <- *p.info

	p.Load(url)
}

func (p *Player) VolumeUp() {
	p.Lock()
	defer p.Unlock()

	defer func() {
		p.info.Volume = p.volume
		p.Info <- *p.info
	}()

	if p.volume == 100 {
		return
	}

	log.Printf("setting volume %d\n", p.volume+5)
	cmd := fmt.Sprintf(`{"command": ["set_property", "volume", %d]}%s`, p.volume+5, "\n")
	writeToMPV([]byte(cmd))
	p.volume += 5
}

func (p *Player) VolumeDn() {
	p.Lock()
	defer p.Unlock()

	defer func() {
		p.info.Volume = p.volume
		p.Info <- *p.info
	}()

	if p.volume == 0 {
		return
	}

	log.Printf("setting volume %d\n", p.volume-5)
	cmd := fmt.Sprintf(`{"command": ["set_property", "volume", %d]}%s`, p.volume-5, "\n")
	writeToMPV([]byte(cmd))
	p.volume -= 5
}

func (p *Player) Stop() {
	log.Printf("stopping %s\n", p.url)
	cmd := fmt.Sprintf(`{"command": ["stop"]}%s`, "\n")
	writeToMPV([]byte(cmd))
	p.info.Status = "Stopped"
	p.info.Song = ""
	p.Info <- *p.info
	p.stopped <- struct{}{}
}

func (p *Player) Load(url string) {
	log.Printf("loading %s\n", url)
	cmd := fmt.Sprintf(`{"command": ["loadfile", "%s"]}%s`, url, "\n")
	writeToMPV([]byte(cmd))
	p.url = url
	go p.observeMetadataChanges()
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

func (p *Player) observeMetadataChanges() {
	cancel := make(chan struct{})
	go p.markSongAsUnknownAfterTimeout(cancel, time.After(5*time.Second))

	// prevent metadata from prev stream
	<-time.After(500 * time.Millisecond)

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
		select {
		default:
			eventBytes, err := bufio.NewReader(c).ReadBytes([]byte("\n")[0])

			if err != nil {
				log.Println(err)
				continue
			}

			if data := unmarshal(eventBytes)["data"]; data != nil {
				p.setNewMetadata(data.(map[string]any))
			}
		case <-p.stopped:
			cancel <- struct{}{}
			return
		}
	}
}

func (p *Player) setNewMetadata(m map[string]any) {
	log.Println(m)

	title, ok := m["icy-title"]

	if ok {
		p.Lock()
		p.info.Song = title.(string)
		p.Info <- *p.info
		p.Unlock()
		return
	}

	artist, ok1 := m["Artist"]
	title, ok2 := m["Title"]

	if ok1 && ok2 {
		p.Lock()
		p.info.Song = fmt.Sprintf("%s - %s", artist.(string), title.(string))
		p.Info <- *p.info
		p.Unlock()
	}
}

func (p *Player) markSongAsUnknownAfterTimeout(cancel chan struct{}, timeout <-chan time.Time) {
	<-timeout

	select {
	case <-cancel:
		return
	default:
		p.Lock()
		defer p.Unlock()
		if p.info.Song == "Loading..." {
			p.info.Song = "Unknown"
			p.Info <- *p.info
		}
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
