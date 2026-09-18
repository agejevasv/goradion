package radio

import (
	"fmt"
	"net/http"
	"os/exec"
	"sync"
	"time"
)

const checkWorkers = 16

// CheckStations prints the stations whose streams don't respond.
func CheckStations(stations []Station) {
	client := &http.Client{Timeout: 10 * time.Second}
	alive := make([]bool, len(stations))

	var wg sync.WaitGroup
	sem := make(chan struct{}, checkWorkers)
	for i, s := range stations {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			alive[i] = httpAlive(client, s.url) || mpvAlive(s.url)
		}()
	}
	wg.Wait()

	dead := 0
	for i, s := range stations {
		if !alive[i] {
			fmt.Printf("  DEAD  %s  (%s)\n", s.title, s.url)
			dead++
		}
	}
	fmt.Printf("\n%d/%d stations alive, %d dead\n", len(stations)-dead, len(stations), dead)
}

func httpAlive(client *http.Client, url string) bool {
	resp, err := client.Get(url)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 400
}

// mpvAlive covers what plain HTTP can't, such as playlists of other protocols.
func mpvAlive(url string) bool {
	return exec.Command("mpv", "--no-video", "--no-terminal", "--frames=1", "--network-timeout=10", url).Run() == nil
}
