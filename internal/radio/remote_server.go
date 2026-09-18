package radio

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/agejevasv/goradion/internal/logging"
)

//go:embed remote.html
var remoteHTML []byte

const (
	tokenAlphabet   = "abcdefghijkmnpqrstuvwxyz23456789"
	tokenLength     = 6
	maxRequestBytes = 64 << 10
)

var errNotFound = errors.New("not found")

// Remote is the HTTP server behind Ctrl+P. It lives until the TUI exits.
type Remote struct {
	app   *Application
	token string // empty: no access code is asked for
	port  int
	ip    string
	srv   *http.Server

	mu         sync.Mutex
	lastSearch searchView // the last results served, for selectSearch
}

// startRemote is idempotent: a running server is returned as is.
func (a *Application) startRemote() (*Remote, error) {
	if a.remote != nil {
		return a.remote, nil
	}

	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", a.remotePort))
	if err != nil {
		logging.Printf("remote: port %d unavailable (%v), picking a free one", a.remotePort, err)
		if ln, err = net.Listen("tcp", ":0"); err != nil {
			return nil, err
		}
	}

	token := newToken(tokenLength)
	if a.remoteKey != nil {
		token = strings.TrimSpace(*a.remoteKey)
	}

	r := &Remote{
		app:   a,
		token: token,
		port:  ln.Addr().(*net.TCPAddr).Port,
		ip:    lanIP(),
	}
	r.srv = &http.Server{Handler: r.handler(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		if err := r.srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logging.Printf("remote: server stopped: %v", err)
		}
	}()

	logging.Printf("remote: listening on %s", r.URL())
	a.remote = r
	return r, nil
}

func (a *Application) stopRemote() {
	if a.remote == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := a.remote.srv.Shutdown(ctx); err != nil {
		a.remote.srv.Close()
	}
	logging.Println("remote: server stopped")
	a.remote = nil
}

// Address is what a user types into the phone browser; URL is what the QR
// code carries, key included.
func (r *Remote) Address() string {
	return "http://" + net.JoinHostPort(r.ip, fmt.Sprint(r.port))
}

func (r *Remote) URL() string {
	if r.token == "" {
		return r.Address()
	}
	return r.Address() + "/?k=" + url.QueryEscape(r.token)
}

func (r *Remote) Key() string {
	return r.token
}

func (r *Remote) handler() http.Handler {
	a := r.app
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", r.index)
	mux.HandleFunc("GET /api/state", r.auth(r.state))
	mux.HandleFunc("GET /api/search", r.auth(r.search))
	mux.HandleFunc("POST /api/tags", r.auth(r.action(a.remoteShowTags)))
	mux.HandleFunc("POST /api/tag", r.auth(r.action(a.remoteOpenTag)))
	mux.HandleFunc("POST /api/play", r.auth(r.action(a.remotePlay)))
	mux.HandleFunc("POST /api/stop", r.auth(r.action(a.remoteStop)))
	mux.HandleFunc("POST /api/random", r.auth(r.action(a.remoteRandom)))
	mux.HandleFunc("POST /api/volume", r.auth(r.action(a.remoteVolume)))
	mux.HandleFunc("POST /api/shuffle", r.auth(r.action(a.remoteShuffle)))
	mux.HandleFunc("POST /api/search/select", r.auth(r.action(r.selectSearch)))
	mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	return mux
}

// auth accepts the key as the "k" query parameter (QR link) or the X-Key
// header (page script). Case is ignored: phone keyboards capitalize.
func (r *Remote) auth(next http.HandlerFunc) http.HandlerFunc {
	want := []byte(strings.ToLower(r.token))
	return func(w http.ResponseWriter, req *http.Request) {
		key := req.URL.Query().Get("k")
		if key == "" {
			key = req.Header.Get("X-Key")
		}
		key = strings.ToLower(strings.TrimSpace(key))
		if len(want) > 0 && subtle.ConstantTimeCompare([]byte(key), want) != 1 {
			http.Error(w, "Forbidden: wrong access code. The code is shown by goradion when you press Ctrl+P.", http.StatusForbidden)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		next(w, req)
	}
}

func (r *Remote) index(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(remoteHTML)
}

func (r *Remote) state(w http.ResponseWriter, _ *http.Request) {
	var st remoteState
	r.app.app.QueueUpdate(func() { st = r.app.remoteState() })
	writeJSON(w, st)
}

// action runs a state-changing request on the UI goroutine and replies with
// the resulting state.
func (r *Remote) action(run func(q actionRequest) error) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		var q actionRequest
		err := json.NewDecoder(http.MaxBytesReader(w, req.Body, maxRequestBytes)).Decode(&q)
		if err != nil && !errors.Is(err, io.EOF) {
			http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
			return
		}

		var st remoteState
		r.app.app.QueueUpdateDraw(func() {
			if err = run(q); err == nil {
				st = r.app.remoteState()
			}
		})
		if err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, errNotFound) {
				status = http.StatusNotFound
			}
			http.Error(w, err.Error(), status)
			return
		}
		writeJSON(w, st)
	}
}

func (r *Remote) search(w http.ResponseWriter, req *http.Request) {
	query := strings.TrimSpace(req.URL.Query().Get("q"))
	online := req.URL.Query().Get("online") == "1"

	results := []remoteStation{}
	var stations []Station
	if query != "" && online {
		found, err := searchRadioBrowser(query)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		for _, f := range found {
			stations = append(stations, f.station)
			results = append(results, remoteStation{Title: f.station.title, URL: f.station.url, Meta: onlineMeta(f)})
		}
	} else {
		stations = searchStations(r.app.stations, query)
		for _, s := range stations {
			results = append(results, remoteStation{Title: s.title, URL: s.url})
		}
	}

	r.mu.Lock()
	r.lastSearch = searchView{query: query, stations: stations, online: online}
	r.mu.Unlock()

	writeJSON(w, map[string]any{"query": query, "online": online, "results": results})
}

// selectSearch does what picking a result in the TUI search modal does: the
// results become the current list and the chosen station starts playing.
func (r *Remote) selectSearch(q actionRequest) error {
	r.mu.Lock()
	last := r.lastSearch
	r.mu.Unlock()

	if last.query == "" || last.query != q.Query || last.online != q.Online {
		return errors.New("search results are stale, search again")
	}
	for _, s := range last.stations {
		if s.url == q.URL {
			r.app.selectSearchResult(last.query, last.stations, s, last.online)
			return nil
		}
	}
	return fmt.Errorf("station %w in search results", errNotFound)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		logging.Printf("remote: %v", err)
	}
}

func newToken(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	for i := range b {
		b[i] = tokenAlphabet[int(b[i])%len(tokenAlphabet)]
	}
	return string(b)
}

// lanIP is the default-route IPv4 address, else the first one of an interface
// that is up, else loopback.
func lanIP() string {
	usable := func(ip net.IP) bool {
		ip = ip.To4()
		return ip != nil && !ip.IsLoopback() && !ip.IsLinkLocalUnicast()
	}

	// Connecting a UDP socket only resolves the route; nothing is sent.
	if conn, err := net.Dial("udp4", "8.8.8.8:80"); err == nil {
		ip := conn.LocalAddr().(*net.UDPAddr).IP
		conn.Close()
		if usable(ip) {
			return ip.String()
		}
	}

	if ifaces, err := net.Interfaces(); err == nil {
		for _, iface := range ifaces {
			if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
				continue
			}
			addrs, err := iface.Addrs()
			if err != nil {
				continue
			}
			for _, addr := range addrs {
				if ipn, ok := addr.(*net.IPNet); ok && usable(ipn.IP) {
					return ipn.IP.String()
				}
			}
		}
	}

	return "127.0.0.1"
}
