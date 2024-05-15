package radio

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const defaultVolume = 80

type Player struct {
	Info   chan Status
	cmd    *exec.Cmd
	status *Status
	url    string
	volume int
	mutex  sync.Mutex
}

type Status struct {
	Status string
	Song   string
	Volume int
}

func NewPlayer() *Player {
	return &Player{
		Info:   make(chan Status),
		volume: defaultVolume,
		status: &Status{
			Volume: defaultVolume,
		},
	}
}

func (p *Player) Start() {
	p.mutex.Lock()
	p.cmd = exec.Command(
		"mpv",
		"-no-video",
		"--idle",
		fmt.Sprintf("--volume=%d", defaultVolume),
		fmt.Sprintf("--input-ipc-server=%s", socket),
	)

	stdout, _ := p.cmd.StdoutPipe()

	if err := p.cmd.Start(); err != nil {
		panic(fmt.Sprintf("%s\nPlease install mpv: https://mpv.io", err))
	}

	for i := 1; !p.mvpIsListening() && i <= 10; i++ {
		if i == 10 {
			panic("mpv failed to start")
		}
		log.Printf("waiting for mpv +%d ms\n", 8<<i)
		time.Sleep((8 << i) * time.Millisecond)
	}

	p.mutex.Unlock()
	log.Println("mpv is ready")

	go func() {
		scanner := bufio.NewScanner(stdout)
		scanner.Split(bufio.ScanLines)

		for scanner.Scan() {
			line := scanner.Text()
			if strings.Contains(line, "icy-title:") {
				title := strings.Trim(strings.ReplaceAll(line, "icy-title:", ""), " ")
				if title == "" {
					continue
				}
				p.mutex.Lock()
				p.status.Song = title
				p.mutex.Unlock()
				p.Info <- *p.status
			}
		}
	}()
}

func (p *Player) Toggle(station, url string) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if url == p.url {
		p.Stop()
		p.url = ""
		p.status.Status = "Stopped"
		p.status.Song = ""
		p.Info <- *p.status
		return
	}

	p.status.Status = station
	p.status.Song = ""
	p.status.Volume = p.volume
	p.Info <- *p.status
	p.Load(url)
}

func (p *Player) VolumeUp() {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	defer func() {
		p.status.Volume = p.volume
		p.Info <- *p.status
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
	p.mutex.Lock()
	defer p.mutex.Unlock()

	defer func() {
		p.status.Volume = p.volume
		p.Info <- *p.status
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
}

func (p *Player) Load(url string) {
	log.Printf("loading %s\n", url)
	cmd := fmt.Sprintf(`{"command": ["loadfile", "%s"]}%s`, url, "\n")
	p.writeToMPV([]byte(cmd))
	p.url = url
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
