//go:build !windows

package radio

import (
	"fmt"
	"net"
	"os"
)

var socket = fmt.Sprintf("/tmp/mpv%d.sock", os.Getpid())

func netDial() (net.Conn, error) {
	return net.Dial("unix", socket)
}
