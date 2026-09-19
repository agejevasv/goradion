//go:build !windows

package mpv

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// fakeMPV listens on the IPC socket and records the commands it receives.
type fakeMPV struct {
	t    *testing.T
	cmds chan []any

	mu     sync.Mutex
	conns  []net.Conn
	events net.Conn // the connection that observes properties
}

func startFakeMPV(t *testing.T) *fakeMPV {
	t.Helper()
	dir, err := os.MkdirTemp("", "mpv") // short: socket paths are limited
	if err != nil {
		t.Fatal(err)
	}
	old := socket
	socket = filepath.Join(dir, "sock")
	ln, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}

	f := &fakeMPV{t: t, cmds: make(chan []any, 100)}
	t.Cleanup(func() {
		ln.Close()
		f.mu.Lock()
		for _, c := range f.conns {
			c.Close()
		}
		f.mu.Unlock()
		socket = old
		os.RemoveAll(dir)
	})
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			f.mu.Lock()
			f.conns = append(f.conns, c)
			f.mu.Unlock()
			go f.read(c)
		}
	}()
	return f
}

func (f *fakeMPV) read(c net.Conn) {
	sc := bufio.NewScanner(c)
	for sc.Scan() {
		var msg struct{ Command []any }
		if err := json.Unmarshal(sc.Bytes(), &msg); err != nil {
			f.t.Errorf("bad command %q: %v", sc.Text(), err)
			continue
		}
		if msg.Command[0] == "observe_property" {
			f.mu.Lock()
			f.events = c
			f.mu.Unlock()
		}
		f.cmds <- msg.Command
	}
}

// expect skips commands until one named name arrives.
func (f *fakeMPV) expect(name string) []any {
	f.t.Helper()
	timeout := time.After(3 * time.Second)
	for {
		select {
		case cmd := <-f.cmds:
			if cmd[0] == name {
				return cmd
			}
		case <-timeout:
			f.t.Fatalf("no %q command", name)
			return nil
		}
	}
}

// expectNone fails if a command named name arrives within d.
func (f *fakeMPV) expectNone(name string, d time.Duration) {
	f.t.Helper()
	timeout := time.After(d)
	for {
		select {
		case cmd := <-f.cmds:
			if cmd[0] == name {
				f.t.Fatalf("unexpected %v", cmd)
			}
		case <-timeout:
			return
		}
	}
}

func (f *fakeMPV) send(event map[string]any) {
	f.t.Helper()
	f.mu.Lock()
	c := f.events
	f.mu.Unlock()
	b, _ := json.Marshal(event)
	if _, err := c.Write(append(b, '\n')); err != nil {
		f.t.Fatal(err)
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func waitInfo(t *testing.T, p *Player, what string, cond func(Info) bool) Info {
	t.Helper()
	var inf Info
	waitFor(t, what, func() bool { inf = p.Snapshot(); return cond(inf) })
	return inf
}

func TestPlayerToggle(t *testing.T) {
	mpv := startFakeMPV(t)
	p := New()
	changed := p.Changed()

	// A URL that would break JSON built by hand.
	url := `http://radio/"quoted"\path`
	p.Toggle("Radio", url)
	if cmd := mpv.expect("loadfile"); cmd[1] != url {
		t.Fatalf("loadfile %q", cmd[1])
	}
	select {
	case <-changed:
	default:
		t.Fatal("Changed not closed")
	}
	if inf := p.Snapshot(); inf.State != Buffering || inf.Status != "Buffering..." || inf.URL != url || inf.Station != "Radio" {
		t.Fatalf("after toggle: %+v", inf)
	}

	p.Toggle("Radio", url)
	mpv.expect("stop")
	if inf := p.Snapshot(); inf.State != Stopped || inf.URL != "" {
		t.Fatalf("after second toggle: %+v", inf)
	}
	p.Toggle("No URL", "")
	if inf := p.Snapshot(); inf.Station != "Radio" {
		t.Fatalf("a station without a URL was played: %+v", inf)
	}
}

func TestPlayerEvents(t *testing.T) {
	mpv := startFakeMPV(t)
	p := New()
	c, err := netDial()
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { p.readEvents(c); close(done) }()
	t.Cleanup(func() { c.Close(); <-done })
	mpv.expect("observe_property")

	const url = "http://radio"
	p.Toggle("Radio", url)
	mpv.expect("loadfile")

	mpv.send(map[string]any{"event": "playback-restart"})
	waitInfo(t, p, "playing", func(inf Info) bool { return inf.State == Playing })

	mpv.send(map[string]any{"event": "property-change", "name": "audio-bitrate", "data": 192400.0})
	mpv.send(map[string]any{"event": "property-change", "name": "filtered-metadata",
		"data": map[string]any{"icy-title": "Stream Title", "Artist": "Artist", "Title": "Song"}})
	inf := waitInfo(t, p, "song", func(inf Info) bool { return inf.Song != "" && inf.Bitrate != 0 })
	if inf.Song != "Artist - Song" || inf.Bitrate != 192 || len(inf.History) != 1 || inf.History[0].Station != "Radio" {
		t.Fatalf("after metadata: %+v", inf)
	}

	mpv.send(map[string]any{"event": "property-change", "name": "pause", "data": true})
	if cmd := mpv.expect("set_property"); cmd[1] != "pause" || cmd[2] != false {
		t.Fatalf("pause not undone: %v", cmd)
	}

	mpv.send(map[string]any{"event": "end-file", "reason": "error"})
	waitInfo(t, p, "failure", func(inf Info) bool { return inf.State == Failed && inf.Status == "Network or stream issues: error" })
	if cmd := mpv.expect("loadfile"); cmd[1] != url {
		t.Fatalf("retry loaded %v", cmd)
	}
	waitInfo(t, p, "buffering again", func(inf Info) bool { return inf.State == Buffering })

	// Stopping cancels the retry of a failure.
	mpv.send(map[string]any{"event": "end-file", "reason": "eof"})
	waitInfo(t, p, "second failure", func(inf Info) bool { return inf.Status == "Network or stream issues: eof" })
	p.Stop()
	mpv.expectNone("loadfile", 2500*time.Millisecond)
}

func TestSongTitle(t *testing.T) {
	for _, tc := range []struct {
		meta map[string]any
		want string
	}{
		{map[string]any{"icy-title": "Live"}, "Live"},
		{map[string]any{"icy-title": "Live", "Artist": "A", "Title": "T"}, "A - T"},
		{map[string]any{"icy-title": "Live", "Artist": "A"}, "Live"},
		{map[string]any{"icy-title": 42.0}, ""},
		{map[string]any{}, ""},
	} {
		if got := songTitle(tc.meta); got != tc.want {
			t.Errorf("songTitle(%v) = %q, want %q", tc.meta, got, tc.want)
		}
	}
}

func TestPlayerVolume(t *testing.T) {
	mpv := startFakeMPV(t)
	p := New()

	p.ChangeVolume(VolumeStep)
	if cmd := mpv.expect("set_property"); cmd[1] != "volume" || cmd[2] != float64(DefaultVolume+VolumeStep) {
		t.Fatalf("volume command %v", cmd)
	}
	for _, tc := range []struct{ set, want int }{{150, 100}, {-3, 0}, {42, 42}} {
		p.SetVolume(tc.set)
		if got := p.Volume(); got != tc.want {
			t.Errorf("SetVolume(%d): volume %d, want %d", tc.set, got, tc.want)
		}
	}

	p.Fade(context.Background(), 0, 20*time.Millisecond)
	if got := p.Volume(); got != 0 {
		t.Fatalf("faded out to %d", got)
	}
	p.Fade(context.Background(), 60, 20*time.Millisecond)
	if got := p.Volume(); got != 60 {
		t.Fatalf("faded in to %d", got)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(50 * time.Millisecond); cancel() }()
	p.Fade(ctx, 0, time.Hour)
	if got := p.Volume(); got != 60 {
		t.Fatalf("a cancelled fade moved the volume to %d", got)
	}
}

func TestWaitPlaying(t *testing.T) {
	startFakeMPV(t)
	p := New()
	const url = "http://radio"
	p.Toggle("Radio", url)

	done := make(chan struct{})
	go func() {
		p.WaitPlaying(context.Background(), url, time.Hour)
		close(done)
	}()
	select {
	case <-done:
		t.Fatal("returned while buffering")
	case <-time.After(50 * time.Millisecond):
	}
	p.setSong("Song")
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("still waiting after the stream started")
	}

	start := time.Now()
	p.WaitPlaying(context.Background(), "http://other", time.Hour)
	p.Toggle("Other", "http://other")
	p.WaitPlaying(context.Background(), "http://other", 50*time.Millisecond)
	if time.Since(start) > time.Second {
		t.Fatal("WaitPlaying ignored the other stream or the timeout")
	}
}

func TestPlayerExited(t *testing.T) {
	mpv := startFakeMPV(t)
	p := New()
	c, err := netDial()
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { p.readEvents(c); close(done) }()
	mpv.expect("observe_property")

	p.Toggle("Radio", "http://radio")
	mpv.expect("loadfile")

	// mpv exiting closes its end of the connection.
	mpv.mu.Lock()
	mpv.events.Close()
	mpv.mu.Unlock()
	<-done
	inf := p.Snapshot()
	if inf.State != Exited || inf.Station != "Radio" || inf.Status != "mpv exited, please restart goradion" {
		t.Fatalf("after exit: %+v", inf)
	}

	p.Toggle("Other", "http://other")
	p.Stop()
	mpv.expectNone("loadfile", 200*time.Millisecond)
	if inf := p.Snapshot(); inf.State != Exited || inf.Station != "Radio" {
		t.Fatalf("played after exit: %+v", inf)
	}
}
