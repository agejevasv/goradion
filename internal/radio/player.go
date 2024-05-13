package radio

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
)

const (
	stopped = "Stopped"
	socket  = "/tmp/grmpv.sock"
)

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
		volume: 100,
		status: &Status{
			Volume: 100,
		},
	}
}

func (p *Player) Start() {
	if _, err := net.Dial("unix", socket); err == nil {
		panic("goradion is already running in another terminal!")
	}

	p.cmd = exec.Command("mpv", "-no-video", "--idle", fmt.Sprintf("--input-ipc-server=%s", socket))

	stdout, _ := p.cmd.StdoutPipe()

	if err := p.cmd.Start(); err != nil {
		panic(fmt.Sprintf("%s\nPlease install mpv: https://mpv.io", err))
	}

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
				p.status.Song = title
				p.Info <- *p.status
			}
		}
	}()
}

func (p *Player) Toggle(station, url string) {
	if url == p.url {
		p.Stop()
		p.url = ""
		p.status.Status = stopped
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
	defer func() {
		p.status.Volume = p.volume
		p.Info <- *p.status
	}()

	if p.volume == 100 {
		return
	}

	cmd := fmt.Sprintf(`{"command": ["set_property", "volume", %d]}%s`, p.volume+5, "\n")

	if p.writeToMPVSocket([]byte(cmd)) {
		p.mutex.Lock()
		p.volume += 5
		p.mutex.Unlock()
	}
}

func (p *Player) VolumeDn() {
	defer func() {
		p.status.Volume = p.volume
		p.Info <- *p.status
	}()

	if p.volume == 0 {
		return
	}

	cmd := fmt.Sprintf(`{"command": ["set_property", "volume", %d]}%s`, p.volume-5, "\n")

	if p.writeToMPVSocket([]byte(cmd)) {
		p.mutex.Lock()
		p.volume -= 5
		p.mutex.Unlock()
	}
}

func (p *Player) Stop() {
	cmd := fmt.Sprintf(`{"command": ["stop"]}%s`, "\n")
	p.writeToMPVSocket([]byte(cmd))
}

func (p *Player) Load(url string) {
	cmd := fmt.Sprintf(`{"command": ["loadfile", "%s"]}%s`, url, "\n")
	if p.writeToMPVSocket([]byte(cmd)) {
		p.mutex.Lock()
		p.url = url
		p.mutex.Unlock()
	}
}

func (p *Player) Quit() {
	if p.cmd == nil {
		return
	}

	// ignore errors for now
	p.cmd.Process.Signal(os.Kill)
	p.cmd.Process.Wait()
}

func (p *Player) writeToMPVSocket(data []byte) bool {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	c, err := net.Dial("unix", socket)

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
