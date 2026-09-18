package radio

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agejevasv/goradion/internal/mpv"
	"github.com/gdamore/tcell/v2"
)

// newTestApp runs the TUI on a simulation screen. mpv is not needed: player
// commands fail softly when the socket is missing.
func newTestApp(t *testing.T, options ...Option) (*Application, tcell.SimulationScreen) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())

	screen := tcell.NewSimulationScreen("")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(100, 30)

	stations, err := LoadStations("")
	if err != nil {
		t.Fatal(err)
	}
	a := NewApp(mpv.New(), stations, 0, options...)
	a.app.SetScreen(screen)
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := a.Run(); err != nil {
			t.Error(err)
		}
	}()
	var once sync.Once
	stop := func() {
		once.Do(func() {
			a.app.Stop()
			<-done
		})
	}
	testAppStops.Store(a, stop)
	t.Cleanup(func() {
		stop()
		testAppStops.Delete(a)
	})

	waitFor(t, "app running", func() bool {
		done := make(chan struct{})
		go func() { a.app.QueueUpdate(func() {}); close(done) }()
		select {
		case <-done:
			return true
		case <-time.After(50 * time.Millisecond):
			return false
		}
	})
	return a, screen
}

// testAppStops holds a stop function per test app. tview's Run clears its
// screen without the lock after Stop, so a second Stop before Run returns
// races with it.
var testAppStops sync.Map

// stopTestApp stops the app and waits for Run to return.
func stopTestApp(a *Application) {
	stop, _ := testAppStops.Load(a)
	stop.(func())()
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// screenText reads the screen on the UI goroutine, which draws it.
func screenText(a *Application, s tcell.SimulationScreen) string {
	var sb strings.Builder
	a.app.QueueUpdate(func() {
		cells, w, h := s.GetContents()
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				if c := cells[y*w+x]; len(c.Runes) == 0 {
					sb.WriteRune(' ')
				} else {
					sb.WriteRune(c.Runes[0])
				}
			}
			sb.WriteRune('\n')
		}
	})
	return sb.String()
}

func onUI[T any](a *Application, f func() T) T {
	var v T
	a.app.QueueUpdate(func() { v = f() })
	return v
}

func resize(a *Application, screen tcell.SimulationScreen, w, h int) {
	screen.SetSize(w, h)
	a.app.QueueUpdateDraw(func() {})
}

func segsText(segs []seg) string {
	var sb strings.Builder
	for _, s := range segs {
		sb.WriteString(s.text)
	}
	return sb.String()
}
