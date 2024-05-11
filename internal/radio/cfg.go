package radio

import (
	"bufio"
	"io"
	"net/http"
	"os"
	"strings"
)

const defaultStations = "https://gist.githubusercontent.com/agejevasv/" +
	"58afa748a7bc14dcccab1ca237d14a0b/raw/stations.csv"

func Stations(sta string) ([]string, []string) {
	var scanner *bufio.Scanner

	if sta == "" {
		scanner = bufio.NewScanner(strings.NewReader(fetchStations(defaultStations)))
	} else if strings.HasPrefix(sta, "http") {
		scanner = bufio.NewScanner(strings.NewReader(fetchStations(sta)))
	} else {
		file, err := os.Open(sta)
		if err != nil {
			panic(err)
		}
		defer file.Close()
		scanner = bufio.NewScanner(file)
	}

	stat := make([]string, 0)
	urls := make([]string, 0)

	for scanner.Scan() {
		d := strings.Split(scanner.Text(), ",")
		stat = append(stat, strings.Trim(d[0], " "))
		urls = append(urls, strings.Trim(d[1], " "))
	}

	if err := scanner.Err(); err != nil {
		panic(err)
	}

	return stat, urls
}

func fetchStations(url string) string {
	resp, err := http.Get(url)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}

	return string(data)
}
