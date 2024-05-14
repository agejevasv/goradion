//go:build windows

package radio

import (
	"fmt"
	"os"

	"github.com/Microsoft/go-winio"
)

var socket = fmt.Sprintf(`\\.\pipe\mpv%dsock`, os.Getpid())

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
