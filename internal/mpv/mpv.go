package mpv

import (
	"encoding/json"
	"io"
	"slices"

	"github.com/agejevasv/goradion/internal/logging"
)

// mpvMessage is one line of mpv's JSON IPC: an event or a command reply.
type mpvMessage map[string]any

func parseMessage(data []byte) mpvMessage {
	m := make(mpvMessage)
	if err := json.Unmarshal(data, &m); err != nil {
		logging.Println(err)
	}
	return m
}

func (m mpvMessage) str(key string) string {
	s, _ := m[key].(string)
	return s
}

func (m mpvMessage) isEvent(event string) bool {
	return m.str("event") == event
}

func (m mpvMessage) isPropertyChange(name string) bool {
	return m.isEvent("property-change") && m.str("name") == name
}

func (m mpvMessage) reasonIs(reasons ...string) bool {
	return slices.Contains(reasons, m.str("reason"))
}

// requestID is -1 for messages that are not command replies.
func (m mpvMessage) requestID() int {
	if id, ok := m["request_id"].(float64); ok {
		return int(id)
	}
	return -1
}

// sendCommand writes one command on a connection; a non-zero requestID makes
// mpv echo it in the reply.
func sendCommand(w io.Writer, requestID int, args ...any) error {
	msg := map[string]any{"command": args}
	if requestID != 0 {
		msg["request_id"] = requestID
	}
	b, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	_, err = w.Write(append(b, '\n'))
	return err
}

// mpvCommand sends a command on a connection of its own, for commands whose
// reply nobody waits for.
func mpvCommand(args ...any) bool {
	c, err := netDial()
	if err != nil {
		logging.Println(err, args)
		return false
	}
	defer c.Close()
	if err := sendCommand(c, 0, args...); err != nil {
		logging.Println(err, args)
		return false
	}
	return true
}

func mpvIsListening() bool {
	c, err := netDial()
	if err != nil {
		return false
	}
	c.Close()
	return true
}
