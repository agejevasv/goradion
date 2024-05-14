//go:build !windows

package radio

import (
	"fmt"
	"net"
	"os"
)

var socket = fmt.Sprintf("/tmp/mpv%d.sock", os.Getpid())

func (p *Player) writeToMPV(data []byte) bool {
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

func (p *Player) mvpIsListening() bool {
	_, err := net.Dial("unix", socket)
	return err == nil
}
