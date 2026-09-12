package radio

import "testing"

func TestParseRadioBrowserDedupes(t *testing.T) {
	rows := []radioBrowserStation{
		{Name: "Top Bachata Radio", URL: "https://x/topbachata", URLResolved: "https://x/topbachata", Bitrate: 128},
		{Name: "Top Bachata Radio", URL: "https://x/topbachata", URLResolved: "https://x/topbachata", Bitrate: 64},
		{Name: "Bachata Mix", URL: "https://x/mix", URLResolved: "", Tags: "bachata, latin"},
		{Name: "", URL: "https://x/noname"},
		{Name: "No URL"},
		{Name: "Same stream, other name", URL: "https://x/mix"},
	}

	got := parseRadioBrowser(rows)
	if len(got) != 2 {
		t.Fatalf("got %d results, want 2: %+v", len(got), got)
	}
	if got[0].station.title != "Top Bachata Radio" || got[0].bitrate != 128 {
		t.Fatalf("first occurrence should win: %+v", got[0])
	}
	if got[1].station.url != "https://x/mix" || len(got[1].station.tags) != 2 {
		t.Fatalf("unexpected second result: %+v", got[1])
	}
}
