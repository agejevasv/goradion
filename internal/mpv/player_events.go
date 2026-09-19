package mpv

import (
	"bufio"
	"io"
	"math"
	"net"
	"time"

	"github.com/agejevasv/goradion/internal/logging"
)

// readEvents follows mpv's events on c until the connection closes, which
// happens when mpv exits.
func (p *Player) readEvents(c net.Conn) {
	defer c.Close()

	props := []string{"filtered-metadata", "audio-bitrate", "pause"}
	if vuState(p.vu.Load()) != vuOff {
		props = append(props, "af-metadata/"+vuFilterLabel, "audio-params/samplerate", "core-idle")
	}
	for _, name := range props {
		if err := sendCommand(c, 0, "observe_property", 1, name); err != nil {
			logging.Println(err)
		}
	}

	// One reader for the whole connection: mpv sends several messages at once.
	reader := bufio.NewReader(c)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			logging.Println("mpv events:", err)
			p.exited()
			return
		}
		m := parseMessage(line)
		if !p.handleVU(c, m) {
			p.handleEvent(c, m)
		}
	}
}

func (p *Player) handleEvent(c io.Writer, m mpvMessage) {
	switch {
	case m.isPropertyChange("audio-bitrate"):
		if br, ok := m["data"].(float64); ok {
			p.update(func(inf *Info) { inf.Bitrate = int(math.Round(br / 1000)) })
		}
		return
	case m.isPropertyChange("pause"):
		// mpv pauses (at least on macOS) when Bluetooth headphones disconnect,
		// although playback could go on through the speakers. goradion has no
		// pause, so it is undone.
		if paused, _ := m["data"].(bool); paused {
			mpvCommand("set_property", "pause", false)
		}
	case m.isPropertyChange("filtered-metadata"):
		if meta, ok := m["data"].(map[string]any); ok {
			p.setSong(songTitle(meta))
		}
	case m.isEvent("playback-restart"):
		p.setSong("")
		p.installVU(c)
	case m.isEvent("end-file"):
		if m.reasonIs("error") {
			p.suspectVU(c)
		}
		if m.reasonIs("eof", "error", "unknown") {
			p.scheduleRetry(m.str("reason"))
		}
	}
	logging.Println(m)
}

// songTitle prefers the Artist and Title tags over the stream title.
func songTitle(meta map[string]any) string {
	artist, _ := meta["Artist"].(string)
	title, _ := meta["Title"].(string)
	if artist != "" && title != "" {
		return artist + " - " + title
	}
	icy, _ := meta["icy-title"].(string)
	return icy
}

// setSong marks a buffering stream as playing, and records a new song.
func (p *Player) setSong(song string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	changed := false
	if p.info.State == Buffering {
		p.info.setState(Playing, "")
		p.info.Song = ""
		p.retry.count = 0
		changed = true
	}
	if song != "" && song != p.info.PrevSong {
		p.info.setState(Playing, "")
		p.info.Song = song
		p.info.PrevSong = song
		p.info.History = appendTrack(p.info.History, Track{Time: time.Now(), Station: p.info.Station, Song: song})
		changed = true
	}
	if changed {
		p.notifyLocked()
	}
}

// appendTrack returns a new slice, so that copies of the old one stay intact.
func appendTrack(history []Track, t Track) []Track {
	start := max(len(history)+1-historySize, 0)
	out := make([]Track, 0, len(history)+1-start)
	out = append(out, history[start:]...)
	return append(out, t)
}
