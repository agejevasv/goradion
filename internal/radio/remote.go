package radio

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"
)

//go:embed remote.html
var remoteHTML []byte

const (
	allStationsTag = "All Stations"
	tokenAlphabet  = "abcdefghijkmnpqrstuvwxyz23456789"
	tokenLength    = 6
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
	lastSearch remoteSearch
}

type remoteSearch struct {
	query    string
	online   bool
	stations []Station
}

type remoteTag struct {
	Name   string `json:"name"`
	Kind   string `json:"kind,omitempty"`
	Online bool   `json:"online,omitempty"`
}

type remoteStation struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Meta    string `json:"meta,omitempty"`
	Playing bool   `json:"playing"`
}

type remotePlayer struct {
	Status  string `json:"status"`
	Station string `json:"station"`
	Song    string `json:"song"`
	URL     string `json:"url"`
	Volume  int    `json:"volume"`
	Bitrate int    `json:"bitrate"`
}

type remoteShuffle struct {
	Active    bool `json:"active"`
	Remaining int  `json:"remaining"`
	Interval  int  `json:"interval"`
}

type remoteState struct {
	Version  string          `json:"version"`
	Page     string          `json:"page"`
	Tag      string          `json:"tag"`
	Tags     []remoteTag     `json:"tags"`
	Stations []remoteStation `json:"stations"`
	Player   remotePlayer    `json:"player"`
	Shuffle  remoteShuffle   `json:"shuffle"`
}

// startRemote is idempotent: a running server is returned as is.
func (a *Application) startRemote() (*Remote, error) {
	if a.remote != nil {
		return a.remote, nil
	}

	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", a.remotePort))
	if err != nil {
		log.Printf("remote: port %d unavailable (%v), picking a free one", a.remotePort, err)
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
			log.Printf("remote: server stopped: %v", err)
		}
	}()

	log.Printf("remote: listening on %s", r.URL())
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
	log.Println("remote: server stopped")
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

type actionRequest struct {
	Tag     string `json:"tag"`
	URL     string `json:"url"`
	Volume  *int   `json:"volume"`
	Minutes int    `json:"minutes"`
	Query   string `json:"query"`
	Online  bool   `json:"online"`
}

// auth accepts the key as the "k" query parameter (QR link) or the X-Key
// header (page script). Case is ignored: phone keyboards capitalize.
func (r *Remote) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		key := req.URL.Query().Get("k")
		if key == "" {
			key = req.Header.Get("X-Key")
		}
		key = strings.ToLower(strings.TrimSpace(key))
		if r.token != "" && subtle.ConstantTimeCompare([]byte(key), []byte(strings.ToLower(r.token))) != 1 {
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
	writeJSON(w, r.app.remoteState())
}

// action runs a state-changing request and replies with the resulting state.
func (r *Remote) action(run func(q actionRequest) error) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		var q actionRequest
		if req.ContentLength > 0 {
			if err := json.NewDecoder(req.Body).Decode(&q); err != nil {
				http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
				return
			}
		}
		if err := run(q); err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, errNotFound) {
				status = http.StatusNotFound
			}
			http.Error(w, err.Error(), status)
			return
		}
		writeJSON(w, r.app.remoteState())
	}
}

func (r *Remote) search(w http.ResponseWriter, req *http.Request) {
	query := strings.TrimSpace(req.URL.Query().Get("q"))
	online := req.URL.Query().Get("online") == "1"

	var results []remoteStation
	var stations []Station

	if query != "" && online {
		found, err := SearchRadioBrowser(query)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		for _, f := range found {
			stations = append(stations, f.station)
			results = append(results, remoteStation{Title: f.station.title, URL: f.station.url, Meta: onlineMeta(f)})
		}
	} else if query != "" {
		stations = r.app.filterStations(query)
		for _, s := range stations {
			results = append(results, remoteStation{Title: s.title, URL: s.url})
		}
	}

	r.mu.Lock()
	r.lastSearch = remoteSearch{query: query, online: online, stations: stations}
	r.mu.Unlock()

	if results == nil {
		results = []remoteStation{}
	}
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
			r.app.app.QueueUpdateDraw(func() {
				r.app.openSearchInMain(last.query, last.stations, last.online)
				r.app.stationsList.SetCurrentItem(r.app.findStationIndex(s.url, last.stations))
			})
			r.app.togglePlayManual(s)
			return nil
		}
	}
	return fmt.Errorf("station %w in search results", errNotFound)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// remoteState snapshots what the TUI shows. UI fields are read on the UI
// goroutine; the player lock is taken afterwards, never inside the queued
// function: the player blocks on the Info channel while holding its lock, and
// the channel's consumer needs the UI loop to draw.
func (a *Application) remoteState() remoteState {
	st := remoteState{Version: VersionString(), Page: "tags"}

	var stations []Station
	a.app.QueueUpdate(func() {
		st.Tag = a.tag
		st.Tags = a.remoteTags()
		if slices.Contains(a.pages.GetPageNames(true), a.pageNames[Main]) {
			st.Page = "stations"
			stations = a.getStationsFromCurrentView()
		}
		st.Shuffle.Interval = int(a.shuffleInterval.Minutes())
		if a.timedRandomActive {
			st.Shuffle.Active = true
			st.Shuffle.Remaining = int(max(a.shuffleInterval-time.Since(a.shuffleIterationStartAt), 0).Seconds())
		}
	})

	a.player.Lock()
	inf := *a.player.info
	a.player.Unlock()

	st.Player = remotePlayer{
		Status:  inf.Status,
		Station: stripPlayCount(inf.Station),
		Song:    inf.Song,
		URL:     inf.Url,
		Volume:  inf.Volume,
		Bitrate: inf.Bitrate,
	}

	st.Stations = make([]remoteStation, 0, len(stations))
	for _, s := range stations {
		st.Stations = append(st.Stations, remoteStation{
			Title:   stripPlayCount(s.title),
			URL:     s.url,
			Playing: s.url != "" && s.url == inf.Url,
		})
	}
	return st
}

func (a *Application) remoteTags() []remoteTag {
	out := make([]remoteTag, 0)
	if a.favorites.hasFavorites() {
		out = append(out, remoteTag{Name: favoritesTag, Kind: "favorites"})
	}
	out = append(out, remoteTag{Name: allStationsTag, Kind: "all"})
	for _, t := range tags(a.stations) {
		out = append(out, remoteTag{Name: t})
	}
	if a.lastSearchTag != "" {
		out = append(out, remoteTag{Name: a.lastSearchTag, Kind: "search", Online: a.lastOnlineStations != nil})
	}
	return out
}

func (a *Application) remoteShowTags(actionRequest) error {
	a.app.QueueUpdateDraw(func() {
		a.tag = ""
		a.show(Tags)
	})
	return nil
}

func (a *Application) remoteOpenTag(q actionRequest) error {
	if q.Tag == favoritesTag && !a.favorites.hasFavorites() {
		return fmt.Errorf("tag %q %w", q.Tag, errNotFound)
	}
	var err error
	a.app.QueueUpdateDraw(func() {
		if !a.openTag(q.Tag) {
			err = fmt.Errorf("tag %q %w", q.Tag, errNotFound)
		}
	})
	return err
}

// remotePlay toggles a station of the list currently shown, like Enter does.
func (a *Application) remotePlay(q actionRequest) error {
	var station Station
	var found bool
	a.app.QueueUpdateDraw(func() {
		stations := a.getStationsFromCurrentView()
		for i, s := range stations {
			if s.url == q.URL {
				station, found = s, true
				a.stationsList.SetCurrentItem(i + a.calculateStationListOffset())
				return
			}
		}
	})
	if !found {
		return fmt.Errorf("station %w in the current list", errNotFound)
	}
	a.togglePlayManual(station)
	return nil
}

func (a *Application) remoteStop(actionRequest) error {
	a.player.Lock()
	current := Station{title: a.player.info.Station, url: a.player.info.Url}
	a.player.Unlock()

	if current.url == "" {
		return nil
	}
	a.togglePlayManual(current)
	return nil
}

func (a *Application) remoteRandom(actionRequest) error {
	var station Station
	var ok bool
	a.app.QueueUpdateDraw(func() {
		stations := a.getStationsFromCurrentView()
		if len(stations) == 0 {
			return
		}
		r := a.randomIndex(stations)
		a.stationsList.SetCurrentItem(r + a.calculateStationListOffset())
		station, ok = stations[r], true
	})
	if !ok {
		return errors.New("no stations to pick from")
	}
	a.togglePlayManual(station)
	return nil
}

func (a *Application) remoteVolume(q actionRequest) error {
	if q.Volume == nil {
		return errors.New("volume required")
	}
	a.player.SetVolume(*q.Volume)
	return nil
}

func (a *Application) remoteShuffle(q actionRequest) error {
	if q.Minutes != 0 {
		if q.Minutes < 1 || q.Minutes > 9 {
			return errors.New("minutes must be 1-9")
		}
		a.setShuffleInterval(q.Minutes)
		return nil
	}
	a.toggleTimedRandom()
	return nil
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
