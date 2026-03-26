package radio

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const maxStationNameLen = 50

var reCleanName = regexp.MustCompile(`[^a-zA-Z0-9.,'` + "`" + `"&/ ]`)

type RadioBrowserResult struct {
	station     Station
	countryCode string
	bitrate     int
}

type radioBrowserStation struct {
	Name        string `json:"name"`
	URL         string `json:"url"`
	URLResolved string `json:"url_resolved"`
	CountryCode string `json:"countrycode"`
	Tags        string `json:"tags"`
	Bitrate     int    `json:"bitrate"`
}

func SearchRadioBrowser(query string) ([]RadioBrowserResult, error) {
	server, err := radioBrowserServer()
	if err != nil {
		server = "de1.api.radio-browser.info"
	}

	params := url.Values{}
	params.Set("name", query)
	params.Set("hidebroken", "true")
	params.Set("order", "clickcount")
	params.Set("reverse", "true")
	params.Set("limit", "50")

	apiURL := fmt.Sprintf("https://%s/json/stations/search?%s", server, params.Encode())

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "goradion/"+Version)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var apiResults []radioBrowserStation
	if err := json.NewDecoder(resp.Body).Decode(&apiResults); err != nil {
		return nil, err
	}

	results := make([]RadioBrowserResult, 0, len(apiResults))
	for _, r := range apiResults {
		streamURL := r.URLResolved
		if streamURL == "" {
			streamURL = r.URL
		}
		if streamURL == "" {
			continue
		}

		name := cleanStationName(r.Name)
		if name == "" {
			continue
		}

		var tags []string
		if r.Tags != "" {
			for _, t := range strings.Split(r.Tags, ",") {
				t = strings.TrimSpace(t)
				if t != "" {
					tags = append(tags, t)
				}
			}
		}

		results = append(results, RadioBrowserResult{
			station:     Station{title: name, url: streamURL, tags: tags},
			countryCode: r.CountryCode,
			bitrate:     r.Bitrate,
		})
	}

	return results, nil
}

func radioBrowserServer() (string, error) {
	addrs, err := net.LookupHost("all.api.radio-browser.info")
	if err != nil {
		return "", err
	}
	if len(addrs) == 0 {
		return "", fmt.Errorf("no servers found")
	}

	addr := addrs[rand.Intn(len(addrs))]
	names, err := net.LookupAddr(addr)
	if err != nil {
		return "", err
	}

	if len(names) > 0 {
		return strings.TrimSuffix(names[0], "."), nil
	}
	return "", fmt.Errorf("no server name found")
}

func cleanStationName(name string) string {
	name = strings.TrimSpace(name)

	for _, sep := range []string{" - ", " | ", " || ", " · ", " – ", " — ", " -> "} {
		if i := strings.Index(name, sep); i >= 10 {
			name = name[:i]
			break
		}
	}

	name = reCleanName.ReplaceAllString(name, "")
	name = strings.Join(strings.Fields(name), " ")
	name = strings.TrimRight(name, ",. ")

	if len(name) > maxStationNameLen {
		name = name[:maxStationNameLen]
		if i := strings.LastIndex(name, " "); i > maxStationNameLen/2 {
			name = name[:i]
		}
		name = strings.TrimRight(name, ",. ")
	}

	return name
}
