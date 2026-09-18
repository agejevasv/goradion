// Package mpv plays radio streams with an mpv process driven over its JSON
// IPC socket, and measures the audio for a level meter and a spectrum.
package mpv

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"github.com/agejevasv/goradion/internal/logging"
)

const (
	DefaultVolume = 80
	VolumeStep    = 5
	historySize   = 20
	maxRetryDelay = 30 * time.Second
	fadeSteps     = 20
)

// State is where playback stands; Info.Status puts it in words.
type State int

const (
	Idle State = iota // nothing played yet
	Buffering
	Playing
	Stopped
	Failed // the stream broke and is retried
)

var statusText = [...]string{
	Idle:      "",
	Buffering: "Buffering...",
	Playing:   "Playing",
	Stopped:   "Stopped",
	Failed:    "Network or stream issues",
}

// Player drives an mpv process over its JSON IPC socket. Its methods are safe
// for concurrent use and never block on a consumer: state changes are
// broadcast through Changed and read with Snapshot.
type Player struct {
	cmd *exec.Cmd

	mu      sync.Mutex
	info    Info
	changed chan struct{} // closed and replaced whenever info changes
	retry   *retry

	// Atomic, so that the UI can draw the meter without the lock.
	vu        atomic.Int32 // a vuState
	meter     atomic.Int32 // index into meterCandidates, kept once mpv takes one
	levelBits atomic.Uint64
	levelAt   atomic.Int64
	bands     atomic.Pointer[[BandCount]float64] // replaced, never modified in place
	bandsAt   atomic.Int64

	// Only touched by the goroutine that reads mpv events.
	sampleRate int
	coreIdle   bool
	watchSince time.Time // when the spectrum watchdog was armed, zero when it is not
	watchTimer *time.Timer
}

type Info struct {
	State    State
	Status   string
	Station  string
	Song     string
	PrevSong string
	URL      string
	Volume   int
	Bitrate  int
	// Replaced, never modified in place, so copies of Info can share it.
	History []Track
}

// setState updates State and Status; reason details a failure.
func (inf *Info) setState(s State, reason string) {
	inf.State, inf.Status = s, statusText[s]
	if reason != "" {
		inf.Status += ": " + reason
	}
}

type Track struct {
	Time    time.Time
	Station string
	Song    string
}

// retry reloads the current stream after it fails, backing off. Every new
// station gets a new one, which cancels the reloads pending for the old one.
type retry struct {
	ctx    context.Context
	cancel context.CancelFunc
	count  int
}

func newRetry() *retry {
	ctx, cancel := context.WithCancel(context.Background())
	return &retry{ctx: ctx, cancel: cancel}
}

func New() *Player {
	return &Player{
		info:    Info{Volume: DefaultVolume},
		changed: make(chan struct{}),
		retry:   newRetry(),
	}
}

// Start launches mpv and returns once its IPC socket accepts connections.
func (p *Player) Start() error {
	p.cmd = exec.Command(
		"mpv",
		"--no-video",
		"--idle",
		"--display-tags=Artist,Title,icy-title",
		"--network-timeout=10",
		fmt.Sprintf("--volume=%d", DefaultVolume),
		fmt.Sprintf("--input-ipc-server=%s", socket),
	)
	if err := p.cmd.Start(); err != nil {
		return fmt.Errorf("%w\nPlease make sure 'mpv' is available. "+
			"Install it using your package manager or visit https://mpv.io for more info", err)
	}

	for delay := 16 * time.Millisecond; !mpvIsListening(); delay *= 2 {
		if delay > 4*time.Second {
			p.cmd.Process.Kill()
			p.cmd.Wait()
			return errors.New("mpv failed to start")
		}
		logging.Printf("waiting for mpv %v", delay)
		time.Sleep(delay)
	}

	c, err := netDial()
	if err != nil {
		return err
	}
	go p.readEvents(c)
	return nil
}

func (p *Player) Quit() {
	logging.Println("quitting mpv")
	if !mpvCommand("quit", 9) && p.cmd != nil && p.cmd.Process != nil {
		logging.Println("mpv failed to quit via socket")
		p.cmd.Process.Kill()
		p.cmd.Wait()
	}
}

func (p *Player) Snapshot() Info {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.info
}

// URL is the stream loaded, empty when stopped.
func (p *Player) URL() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.info.URL
}

func (p *Player) Volume() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.info.Volume
}

// Changed returns a channel that is closed on the next state change. Take it
// before the Snapshot it follows, so that no change is missed.
func (p *Player) Changed() <-chan struct{} {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.changed
}

func (p *Player) notifyLocked() {
	close(p.changed)
	p.changed = make(chan struct{})
}

func (p *Player) update(change func(inf *Info)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	change(&p.info)
	p.notifyLocked()
}

// WaitPlaying returns once url plays or the player moved on to another
// stream, or when ctx is done or timeout has passed.
func (p *Player) WaitPlaying(ctx context.Context, url string, timeout time.Duration) {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for {
		changed := p.Changed()
		if inf := p.Snapshot(); inf.URL != url || inf.State == Playing || inf.Song != "" {
			return
		}
		select {
		case <-changed:
		case <-ctx.Done():
			return
		case <-deadline.C:
			return
		}
	}
}

// Toggle plays the stream at url, or stops it when it is the one playing.
// The station title is shown and kept in the history.
func (p *Player) Toggle(station, url string) {
	if url == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	if url == p.info.URL {
		p.stopLocked()
		return
	}

	p.retry.cancel()
	p.retry = newRetry()
	p.info.Station = station
	p.info.URL = url
	p.info.setState(Buffering, "")
	p.info.Bitrate = 0
	p.info.Song = ""
	p.info.PrevSong = ""
	p.notifyLocked()
	p.loadLocked()
}

func (p *Player) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.info.URL != "" {
		p.stopLocked()
	}
}

func (p *Player) stopLocked() {
	logging.Printf("stopping %s", p.info.URL)
	p.retry.cancel()
	mpvCommand("stop")
	p.info.setState(Stopped, "")
	p.info.URL = ""
	p.info.Song = ""
	p.info.PrevSong = ""
	p.info.Bitrate = 0
	p.notifyLocked()
}

func (p *Player) loadLocked() {
	logging.Printf("loading %s", p.info.URL)
	mpvCommand("loadfile", p.info.URL)
}

// scheduleRetry reports a failed stream and reloads it after a delay that
// doubles with every failure in a row.
func (p *Player) scheduleRetry(reason string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.info.URL == "" {
		return
	}
	r := p.retry
	delay := min(time.Second<<r.count, maxRetryDelay)
	r.count++
	p.info.setState(Failed, reason)
	p.info.Song = ""
	p.notifyLocked()

	go func() {
		select {
		case <-r.ctx.Done():
			return
		case <-time.After(delay):
		}
		p.mu.Lock()
		defer p.mu.Unlock()
		if r != p.retry || r.ctx.Err() != nil {
			return
		}
		p.info.setState(Buffering, "")
		p.info.PrevSong = ""
		p.notifyLocked()
		p.loadLocked()
	}()
}

func (p *Player) ChangeVolume(delta int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.setVolumeLocked(p.info.Volume + delta)
}

// SetVolume clamps volume to 0-100.
func (p *Player) SetVolume(volume int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.setVolumeLocked(volume)
}

func (p *Player) setVolumeLocked(volume int) {
	volume = min(max(volume, 0), 100)
	if volume == p.info.Volume {
		return
	}
	logging.Printf("setting volume %d", volume)
	mpvCommand("set_property", "volume", volume)
	p.info.Volume = volume
	p.notifyLocked()
}

// Fade moves the volume to target in steps over duration, stopping early
// when ctx is done.
func (p *Player) Fade(ctx context.Context, target int, duration time.Duration) {
	from := p.Volume()
	if duration <= 0 {
		p.SetVolume(target)
		return
	}
	tick := time.NewTicker(duration / fadeSteps)
	defer tick.Stop()
	for i := 1; i <= fadeSteps; i++ {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		p.SetVolume(from + (target-from)*i/fadeSteps)
	}
}
