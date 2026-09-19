package radio

import (
	_ "embed"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/agejevasv/goradion/internal/logging"
)

//go:embed stations.csv
var defaultStationsCSV string

type Station struct {
	title string
	url   string
	tags  []string
}

// LoadStations reads the stations CSV from a file or an http(s) URL; an empty
// source means the built-in list, which also stands in for a URL that can't
// be fetched.
func LoadStations(source string) ([]Station, error) {
	switch {
	case source == "":
		return parseStations(strings.NewReader(defaultStationsCSV))
	case strings.HasPrefix(source, "http://"), strings.HasPrefix(source, "https://"):
		body, err := fetchStations(source)
		if err != nil {
			logging.Printf("stations: %v, using the built-in list", err)
			return parseStations(strings.NewReader(defaultStationsCSV))
		}
		defer body.Close()
		return parseStations(body)
	default:
		f, err := os.Open(source)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		return parseStations(f)
	}
}

// parseStations reads "title,url[,tag;tag...]" rows and skips rows without a
// URL.
func parseStations(r io.Reader) ([]Station, error) {
	reader := csv.NewReader(r)
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("can't parse stations CSV: %w", err)
	}

	stations := make([]Station, 0, len(records))
	for _, rec := range records {
		if len(rec) < 2 || strings.TrimSpace(rec[1]) == "" {
			continue
		}
		s := Station{title: strings.TrimSpace(rec[0]), url: strings.TrimSpace(rec[1])}
		if len(rec) > 2 {
			for _, tag := range strings.Split(rec[2], ";") {
				if tag = strings.TrimSpace(tag); tag != "" {
					s.tags = append(s.tags, tag)
				}
			}
		}
		stations = append(stations, s)
	}
	return stations, nil
}

func fetchStations(url string) (io.ReadCloser, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("GET %s: status %d", url, resp.StatusCode)
	}
	return resp.Body, nil
}
