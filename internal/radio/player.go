package radio

import (
	"bufio"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
)

const (
	stopped = "Stopped"
	playing = "Playing"
)

type Player struct {
	OnAir chan string
	url   string
	cmd   *exec.Cmd
}

func NewPlayer() *Player {
	if err := exec.Command("mpv", "-V").Start(); err != nil {
		panic(fmt.Sprintf("%s\nPlease install mpv: https://mpv.io", err))
	}
	return &Player{OnAir: make(chan string)}
}

func (p *Player) Toggle(station, url string) {
	p.Stop()

	if url == p.url {
		p.OnAir <- stopped
		p.url = ""
		return
	}

	p.OnAir <- fmt.Sprintf("%s: %s", playing, station)
	p.url = url
	p.cmd = exec.Command("mpv", "-no-video", url)
	stdout, _ := p.cmd.StdoutPipe()

	if err := p.cmd.Start(); err != nil {
		panic(err)
	}

	scanner := bufio.NewScanner(stdout)
	scanner.Split(bufio.ScanLines)

	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "icy-title:") {
			title := strings.Trim(strings.ReplaceAll(line, "icy-title:", ""), " ")
			if title == "" {
				continue
			}
			p.OnAir <- fmt.Sprintf("%s | %s", station, title)
		}
	}

	p.cmd.Wait()
}

func (p *Player) Stop() {
	if p.cmd == nil {
		return
	}
	p.cmd.Process.Signal(syscall.SIGTERM)
}
