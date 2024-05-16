package radio

import (
	"bytes"
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

type Control struct {
	start chan bool
	stop  chan bool
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
		fmt.Println("'mpv' was not found in $PATH")
		fmt.Println("Please install 'mpv' using your package manager or visit https://mpv.io for more info.")
		os.Exit(1)
	}

	for i := 1; !p.mpvIsListening() && i <= 10; i++ {
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

func (p *Player) SetSongTitle(artist, title string) {
	if artist != "" {
		p.info.Song = fmt.Sprintf("%s - %s", artist, title)
	} else {
		p.info.Song = title
	}
	p.Info <- *p.info
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
	p.writeToMPV([]byte(cmd))
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
	p.writeToMPV([]byte(cmd))
	p.volume -= 5
}

func (p *Player) Stop() {
	log.Printf("stopping %s\n", p.url)
	cmd := fmt.Sprintf(`{"command": ["stop"]}%s`, "\n")
	p.writeToMPV([]byte(cmd))
	p.info.Status = "Stopped"
	p.info.Song = ""
	p.Info <- *p.info
	p.stopped <- struct{}{}
}

func (p *Player) Load(url string) {
	log.Printf("loading %s\n", url)
	cmd := fmt.Sprintf(`{"command": ["loadfile", "%s"]}%s`, url, "\n")
	p.writeToMPV([]byte(cmd))
	p.url = url
	go p.readMetadata()
}

func (p *Player) Quit() {
	log.Println("quitting mpv")
	cmd := fmt.Sprintf(`{"command": ["quit", 9]}%s`, "\n")

	if ok := p.writeToMPV([]byte(cmd)); !ok && p.cmd != nil {
		log.Println("mpv failed to quit via socket")
		p.cmd.Process.Signal(os.Kill)
		p.cmd.Wait()
	}
}

func (p *Player) readMetadata() {
	cancel := make(chan struct{})
	go func() {
		<-time.After(5 * time.Second)

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
	}()

	var res map[string]any
	ticker := time.NewTicker(time.Duration(500) * time.Millisecond)

	for i := 10; ; {
		select {
		case <-ticker.C:
			cmd := fmt.Sprintf(`{"command": ["get_property", "filtered-metadata"]}%s`, "\n")

			data, err := p.readFromMPV([]byte(cmd))
			if err != nil {
				log.Println(err)
				continue
			}

			if err = json.Unmarshal(data, &res); err != nil {
				log.Println(err)
				continue
			}

			if res["data"] != nil {
				meta := res["data"].(map[string]any)

				log.Println(res["data"])

				t, ok := meta["icy-title"]
				a_, ok1 := meta["Artist"]
				t_, ok2 := meta["Title"]

				if ok || ok1 && ok2 {
					i = -1
					ticker.Stop()
					ticker = time.NewTicker(time.Duration(3000) * time.Millisecond)
				} else if i--; i == 0 {
					ticker.Stop()
					ticker = time.NewTicker(time.Duration(3000) * time.Millisecond)
				}

				p.Lock()
				if ok {
					p.SetSongTitle("", t.(string))
				} else if ok1 && ok2 {
					p.SetSongTitle(a_.(string), t_.(string))
				}
				p.Unlock()
			}
		case <-p.stopped:
			cancel <- struct{}{}
			ticker.Stop()
			return
		}
	}
}

func (p *Player) writeToMPV(data []byte) bool {
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

func (p *Player) mpvIsListening() bool {
	_, err := netDial()
	return err == nil
}

func (p *Player) readFromMPV(data []byte) ([]byte, error) {
	c, err := netDial()

	if err != nil {
		return nil, err
	}

	defer c.Close()

	if _, err = c.Write(data); err != nil {
		return nil, err
	}

	res := make([]byte, 1024)
	if _, err = c.Read(res); err != nil {
		return nil, err
	}

	return bytes.Trim(res, "\x00"), nil
}
