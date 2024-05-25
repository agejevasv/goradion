package radio

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"math"
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
	Info  chan Info
	cmd   *exec.Cmd
	info  *Info
	retry *Retry
}

type Info struct {
	Status   string
	Station  string
	Song     string
	PrevSong string
	Url      string
	Volume   int
	Bitrate  int
}

type Retry struct {
	ctx    context.Context
	cancel context.CancelFunc
	count  uint64
}

func NewPlayer() *Player {
	return &Player{
		retry: new(Retry),
		Info:  make(chan Info),
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
		"--network-timeout=10",
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

	if p.retry.cancel != nil {
		p.retry.cancel()
	}

	ctx, cancel := context.WithCancel(context.Background())
	p.retry = &Retry{ctx: ctx, cancel: cancel}

	p.info.Station = station
	p.info.Status = buffering
	p.info.Bitrate = 0
	p.info.Song = ""
	p.Info <- *p.info

	p.Load(url)
}

func (p *Player) Stop() {
	if p.retry.cancel != nil {
		p.retry.cancel()
	}
	log.Printf("stopping %s\n", p.info.Url)
	cmd := fmt.Sprintf(`{"command": ["stop"]}%s`, "\n")
	writeToMPV([]byte(cmd))
	p.info.Status = stopped
	p.info.Song = ""
	p.info.Bitrate = 0
	p.Info <- *p.info
}

func (p *Player) Load(url string) {
	if url == "" {
		p.Stop()
		return
	}
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

	cmds := []string{
		fmt.Sprintf(`{"command": ["observe_property", 1, "filtered-metadata"]}%s`, "\n"),
		fmt.Sprintf(`{"command": ["observe_property", 1, "audio-bitrate"]}%s`, "\n"),
	}

	for _, cmd := range cmds {
		if _, err = c.Write([]byte(cmd)); err != nil {
			log.Println(err)
		}
	}

	for {
		eventBytes, err := bufio.NewReader(c).ReadBytes([]byte("\n")[0])

		if err != nil {
			log.Println(err)
			continue
		}

		rsp := unmarshal(eventBytes)
		log.Println(rsp)

		if eventIs(rsp, "property-change") && nameIs(rsp, "audio-bitrate") {
			br, ok := rsp["data"].(float64)
			if ok {
				p.info.Bitrate = int(math.Round(br / 1000.0))
				p.Info <- *p.info
			}
		}

		if eventIs(rsp, "playback-restart") && p.info.Status == buffering {
			p.setStatusPlaying()
		}

		if eventIs(rsp, "property-change") && nameIs(rsp, "filtered-metadata") {
			meta, ok := rsp["data"].(map[string]any)
			if ok {
				p.setCurrentSong(meta)
			}
		}

		if eventIs(rsp, "end-file") && reasonIsAnyOf(rsp, "eof", "error", "unknown") {
			go func() {
				select {
				case <-p.retry.ctx.Done():
					log.Println("Retry loading is cancelled")
					return
				case <-time.After((1 << p.retry.count) * time.Second):
					p.retry.count++
					p.info.PrevSong = ""
					p.info.Status = buffering
					p.Info <- *p.info
					p.Load(p.info.Url)
				}
			}()
			p.setStatusUnexpectedEndFile(rsp["reason"].(string))
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

func reasonIsAnyOf(m map[string]any, needle ...string) bool {
	if m["reason"] == nil {
		return false
	}

	for _, n := range needle {
		if m["reason"].(string) == n {
			return true
		}
	}

	return false
}

func eventIs(m map[string]any, needle string) bool {
	return m["event"] != nil && m["event"].(string) == needle
}

func nameIs(m map[string]any, needle ...string) bool {
	if m["name"] == nil {
		return false
	}

	for _, n := range needle {
		if m["name"].(string) == n {
			return true
		}
	}

	return false
}
