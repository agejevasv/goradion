//go:build !windows

package radio

import (
	"net"
)

const socket = "/tmp/grmpv.sock"

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
