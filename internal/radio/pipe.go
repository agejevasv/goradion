//go:build windows

package radio

import (
	"github.com/Microsoft/go-winio"
)

const socket = `\\.\pipe\grmpvsock`

func (p *Player) writeToMPV(data []byte) bool {
	c, err := winio.DialPipe(socket, nil)

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
	_, err := winio.DialPipe(socket, nil)
	return err == nil
}
