//go:build windows

package radio

import (
	"fmt"
	"net"
	"os"

	"github.com/Microsoft/go-winio"
)

var socket = fmt.Sprintf(`\\.\pipe\mpv%dsock`, os.Getpid())

func netDial() (net.Conn, error) {
	return winio.DialPipe(socket, nil)
}
