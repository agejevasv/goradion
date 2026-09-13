package radio

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
)

// newTestApp runs the TUI on a simulation screen. mpv is not needed: player
// commands fail softly when the socket is missing.
func newTestApp(t *testing.T, options ...Option) (*Application, tcell.SimulationScreen) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	InitLog(false)

	screen := tcell.NewSimulationScreen("")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(100, 30)

	a := NewApp(NewPlayer(), Stations(""), 0, options...)
	a.app.SetScreen(screen)
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := a.Run(); err != nil {
			t.Error(err)
		}
	}()
	t.Cleanup(func() {
		select {
		case <-done:
		default:
			a.app.Stop()
			<-done
		}
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

func screenText(s tcell.SimulationScreen) string {
	cells, w, h := s.GetContents()
	var sb strings.Builder
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := cells[y*w+x]
			if len(c.Runes) == 0 {
				sb.WriteRune(' ')
			} else {
				sb.WriteRune(c.Runes[0])
			}
		}
		sb.WriteRune('\n')
	}
	return sb.String()
}

func onUI[T any](a *Application, f func() T) T {
	var v T
	a.app.QueueUpdate(func() { v = f() })
	return v
}

type client struct {
	t   *testing.T
	url string
	key string
}

func (c client) do(method, path string, body any) (int, remoteState) {
	c.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, c.url+path, rd)
	req.Header.Set("X-Key", c.key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	var st remoteState
	if resp.StatusCode == 200 {
		if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
			c.t.Fatal(err)
		}
	}
	return resp.StatusCode, st
}

func TestRemoteStartupKey(t *testing.T) {
	status := func(t *testing.T, r *Remote, key string) int {
		t.Helper()
		req, _ := http.NewRequest("GET", fmt.Sprintf("http://127.0.0.1:%d/api/state", r.port), nil)
		req.Header.Set("X-Key", key)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	started := func(t *testing.T, key *string) (*Application, tcell.SimulationScreen, *Remote) {
		t.Helper()
		a, screen := newTestApp(t, WithRemote(key))
		r := onUI(a, func() *Remote { return a.remote })
		if r == nil {
			t.Fatal("WithRemote must start the server with the app")
		}
		if onUI(a, a.isRemoteModalOpen) {
			t.Fatal("autostart must not open the modal")
		}
		return a, screen, r
	}

	t.Run("generated", func(t *testing.T) {
		_, _, r := started(t, nil)
		if len(r.Key()) != tokenLength {
			t.Fatalf("key %q, want a random %d-character code", r.Key(), tokenLength)
		}
	})

	t.Run("custom", func(t *testing.T) {
		key := "Correct Horse [battery] staple"
		a, screen, r := started(t, &key)
		if r.Key() != key {
			t.Fatalf("key %q, want %q", r.Key(), key)
		}
		if !strings.HasSuffix(r.URL(), "/?k=Correct+Horse+%5Bbattery%5D+staple") {
			t.Fatalf("URL %q does not carry the key", r.URL())
		}
		for k, want := range map[string]int{key: 200, strings.ToLower(key): 200, "": 403, "correct": 403} {
			if got := status(t, r, k); got != want {
				t.Errorf("key %q: status %d, want %d", k, got, want)
			}
		}
		screen.InjectKey(tcell.KeyCtrlP, 0, tcell.ModNone)
		waitFor(t, "modal with the key", func() bool { return strings.Contains(screenText(screen), "[battery]") })
		if onUI(a, func() *Remote { return a.remote }) != r {
			t.Fatal("Ctrl+P must reuse the started server")
		}
	})

	t.Run("none", func(t *testing.T) {
		key := ""
		_, screen, r := started(t, &key)
		if r.URL() != r.Address() {
			t.Fatalf("URL %q, want %q", r.URL(), r.Address())
		}
		for _, k := range []string{"", "anything"} {
			if got := status(t, r, k); got != 200 {
				t.Errorf("key %q: status %d, want 200", k, got)
			}
		}
		screen.InjectKey(tcell.KeyCtrlP, 0, tcell.ModNone)
		waitFor(t, "no-code warning", func() bool { return strings.Contains(screenText(screen), "No access code") })
	})
}

func TestRemoteControl(t *testing.T) {
	a, screen := newTestApp(t)

	screen.InjectKey(tcell.KeyCtrlP, 0, tcell.ModNone)
	waitFor(t, "remote server", func() bool { return onUI(a, func() *Remote { return a.remote }) != nil })
	if !onUI(a, a.isRemoteModalOpen) {
		t.Fatal("remote modal should be open")
	}
	first := a.remote
	waitFor(t, "modal drawn", func() bool {
		txt := screenText(screen)
		return strings.Contains(txt, first.Address()) && strings.Contains(txt, first.Key())
	})
	t.Logf("modal:\n%s", screenText(screen))

	screen.InjectKey(tcell.KeyCtrlP, 0, tcell.ModNone)
	waitFor(t, "modal closed", func() bool { return !onUI(a, a.isRemoteModalOpen) })
	screen.InjectKey(tcell.KeyCtrlP, 0, tcell.ModNone)
	waitFor(t, "modal reopened", func() bool { return onUI(a, a.isRemoteModalOpen) })
	if a.remote != first {
		t.Fatal("Ctrl+P must not start a second server")
	}
	screen.InjectKey(tcell.KeyEscape, 0, tcell.ModNone)
	waitFor(t, "modal closed by Esc", func() bool { return !onUI(a, a.isRemoteModalOpen) })
	if onUI(a, func() string { return a.pages.GetPageNames(true)[0] }) != a.pageNames[Tags] {
		t.Fatal("Esc on the modal must not quit or change the page")
	}

	base := fmt.Sprintf("http://127.0.0.1:%d", first.port)
	c := client{t: t, url: base, key: first.token}

	for _, tc := range []struct {
		path, key string
		want      int
	}{
		{"/?k=" + first.token, "", 200},
		{"/", "", 200},
		{"/api/state", first.token, 200},
		{"/api/state", strings.ToUpper(" " + first.token), 200},
		{"/api/state", "", 403},
		{"/api/state?k=wrong", "", 403},
	} {
		req, _ := http.NewRequest("GET", base+tc.path, nil)
		if tc.key != "" {
			req.Header.Set("X-Key", tc.key)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != tc.want {
			t.Fatalf("GET %s key=%q: got %d want %d", tc.path, tc.key, resp.StatusCode, tc.want)
		}
		if tc.want == 200 && strings.HasPrefix(tc.path, "/?") && !strings.Contains(string(body), "goradion") {
			t.Fatalf("GET %s: unexpected body", tc.path)
		}
	}

	_, st := c.do("GET", "/api/state", nil)
	if st.Page != "tags" || len(st.Tags) == 0 || st.Tags[0].Name != allStationsTag {
		t.Fatalf("unexpected initial state: %+v", st)
	}

	code, st := c.do("POST", "/api/tag", map[string]any{"tag": "Jazz"})
	if code != 200 || st.Page != "stations" || st.Tag != "Jazz" || len(st.Stations) == 0 {
		t.Fatalf("open tag: %d %+v", code, st)
	}
	if got := onUI(a, func() string { return a.tag }); got != "Jazz" {
		t.Fatalf("TUI tag = %q", got)
	}
	if code, _ := c.do("POST", "/api/tag", map[string]any{"tag": "Nope"}); code != 404 {
		t.Fatalf("unknown tag: %d", code)
	}

	target := st.Stations[1]
	code, st = c.do("POST", "/api/play", map[string]any{"url": target.URL})
	if code != 200 || st.Player.URL != target.URL || !st.Stations[1].Playing || st.Stations[0].Playing {
		t.Fatalf("play: %d %+v", code, st.Player)
	}
	if idx := onUI(a, a.stationsList.GetCurrentItem); idx != 1+a.calculateStationListOffset() {
		t.Fatalf("TUI selection = %d", idx)
	}
	if code, _ := c.do("POST", "/api/play", map[string]any{"url": "http://nope"}); code != 404 {
		t.Fatalf("unknown station: %d", code)
	}

	_, st = c.do("GET", "/api/state", nil)
	if st.Tags[0].Kind != "favorites" {
		t.Fatalf("favorites tag missing: %+v", st.Tags)
	}
	code, st = c.do("POST", "/api/tag", map[string]any{"tag": favoritesTag})
	if code != 200 || len(st.Stations) != 1 || st.Stations[0].Title != target.Title || !st.Stations[0].Playing {
		t.Fatalf("favorites: %d %+v", code, st.Stations)
	}

	_, st = c.do("POST", "/api/stop", nil)
	if st.Player.URL != "" || st.Player.Status != stopped {
		t.Fatalf("stop: %+v", st.Player)
	}
	_, st = c.do("POST", "/api/play", map[string]any{"url": target.URL})
	if st.Player.URL != target.URL {
		t.Fatalf("replay: %+v", st.Player)
	}

	_, st = c.do("POST", "/api/volume", map[string]any{"volume": 40})
	if st.Player.Volume != 40 {
		t.Fatalf("volume = %d", st.Player.Volume)
	}
	_, st = c.do("POST", "/api/volume", map[string]any{"volume": 150})
	if st.Player.Volume != 100 {
		t.Fatalf("volume = %d", st.Player.Volume)
	}
	if code, _ := c.do("POST", "/api/volume", map[string]any{}); code != 400 {
		t.Fatalf("empty volume: %d", code)
	}

	c.do("POST", "/api/tag", map[string]any{"tag": allStationsTag})
	_, st = c.do("POST", "/api/random", nil)
	if st.Player.URL == "" || st.Player.URL == target.URL {
		t.Fatalf("random: %+v", st.Player)
	}

	_, st = c.do("POST", "/api/shuffle", map[string]any{"minutes": 3})
	if st.Shuffle.Interval != 3 || st.Shuffle.Active {
		t.Fatalf("interval: %+v", st.Shuffle)
	}
	_, st = c.do("POST", "/api/shuffle", nil)
	if !st.Shuffle.Active || st.Shuffle.Remaining <= 0 || st.Shuffle.Remaining > 180 {
		t.Fatalf("shuffle on: %+v", st.Shuffle)
	}
	_, st = c.do("POST", "/api/shuffle", nil)
	if st.Shuffle.Active {
		t.Fatalf("shuffle off: %+v", st.Shuffle)
	}
	if code, _ := c.do("POST", "/api/shuffle", map[string]any{"minutes": 12}); code != 400 {
		t.Fatalf("bad interval: %d", code)
	}

	// Stop first: a random pick above may have landed on the station selected below.
	c.do("POST", "/api/stop", nil)
	resp, err := http.Get(base + "/api/search?q=soma+jazz&k=" + first.token)
	if err != nil {
		t.Fatal(err)
	}
	var sr struct {
		Query   string          `json:"query"`
		Online  bool            `json:"online"`
		Results []remoteStation `json:"results"`
	}
	json.NewDecoder(resp.Body).Decode(&sr)
	resp.Body.Close()
	if len(sr.Results) == 0 || sr.Online {
		t.Fatalf("search: %+v", sr)
	}
	code, st = c.do("POST", "/api/search/select", map[string]any{"query": "soma jazz", "online": false, "url": sr.Results[0].URL})
	if code != 200 || st.Tag != "soma jazz" || st.Player.URL != sr.Results[0].URL || len(st.Stations) != len(sr.Results) {
		t.Fatalf("select: %d %+v", code, st)
	}
	if code, _ := c.do("POST", "/api/search/select", map[string]any{"query": "other", "online": false, "url": sr.Results[0].URL}); code != 400 {
		t.Fatalf("stale select: %d", code)
	}
	_, st = c.do("GET", "/api/state", nil)
	last := st.Tags[len(st.Tags)-1]
	if last.Kind != "search" || last.Name != "soma jazz" || last.Online {
		t.Fatalf("search tag: %+v", last)
	}

	_, st = c.do("POST", "/api/tags", nil)
	if st.Page != "tags" {
		t.Fatalf("tags: %+v", st.Page)
	}
	if onUI(a, func() string { return a.pages.GetPageNames(true)[0] }) != a.pageNames[Tags] {
		t.Fatal("TUI should show tags page")
	}

	a.app.Stop()
	waitFor(t, "server shutdown", func() bool {
		resp, err := http.Get(base + "/")
		if err == nil {
			resp.Body.Close()
		}
		return err != nil
	})
}

func TestQRText(t *testing.T) {
	text, w, h, err := qrText("http://192.168.100.100:7373/?k=abc123")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	if len(lines) != h || h != (w+1)/2 {
		t.Fatalf("got %d lines, width %d, height %d", len(lines), w, h)
	}
	for _, l := range lines {
		body := strings.TrimSuffix(strings.TrimPrefix(l, "[black:white]"), "[-:-]")
		if n := len([]rune(body)); n != w {
			t.Fatalf("line width %d, want %d: %q", n, w, l)
		}
	}
	if w > 40 || h > 20 {
		t.Fatalf("QR too large for a small terminal: %dx%d", w, h)
	}
}

func TestLanIPAndToken(t *testing.T) {
	if ip := lanIP(); ip == "" {
		t.Fatal("no ip")
	}
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		tok := newToken(tokenLength)
		if len(tok) != tokenLength || seen[tok] {
			t.Fatalf("bad token %q", tok)
		}
		seen[tok] = true
	}
}
