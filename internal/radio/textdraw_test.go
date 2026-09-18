package radio

import (
	"testing"
	"time"
)

func TestMarquee(t *testing.T) {
	applyGlyphs(false)
	text := "abcdefghij"
	for _, tc := range []struct {
		width   int
		elapsed time.Duration
		want    string
	}{
		{20, time.Hour, text},
		{6, time.Second, "abcdef"},
		{6, marqueePause + 2*marqueeStep, "cdefgh"},
		{6, marqueePause + 8*marqueeStep, "ij   a"},
		{6, marqueePause + time.Duration(len(text)+marqueeGap)*marqueeStep + time.Second, "abcdef"},
	} {
		if got := marquee(text, tc.width, tc.elapsed); got != tc.want {
			t.Errorf("marquee(%d, %v) = %q, want %q", tc.width, tc.elapsed, got, tc.want)
		}
	}
	if got := sliceCells("日本語", 1, 4); got != " 本 " {
		t.Errorf("wide characters cut in half: %q", got)
	}
}

func TestGaugeAndFit(t *testing.T) {
	applyGlyphs(false)
	if got := segsText(gauge(10, 0.55, styleText, styleDim)); got != "━━━━━╸────" {
		t.Errorf("gauge = %q", got)
	}
	if got := segsText(gauge(4, 1.5, styleText, styleDim)); got != "━━━━" {
		t.Errorf("full gauge = %q", got)
	}
	segs := []seg{{"Hello ", styleText}, {"world", styleDim}}
	if got := segsText(fitSegs(segs, 20)); got != "Hello world" {
		t.Errorf("fit = %q", got)
	}
	if got := segsText(fitSegs(segs, 8)); got != "Hello w…" {
		t.Errorf("fit = %q", got)
	}

	applyGlyphs(true)
	defer applyGlyphs(false)
	if got := segsText(gauge(10, 0.55, styleText, styleDim)); got != "======----" {
		t.Errorf("ascii gauge = %q", got)
	}
}

func TestClockAndHints(t *testing.T) {
	if got := clock(75 * time.Second); got != "1:15" {
		t.Errorf("clock = %q", got)
	}
	if got := clock(3725 * time.Second); got != "1:02:05" {
		t.Errorf("clock = %q", got)
	}
	hints := []hint{{"a", "one"}, {"b", "two"}, {"c", "three"}}
	if got := segsText(hintSegs(hints, 100)); got != "a one   b two   c three" {
		t.Errorf("hints = %q", got)
	}
	if got := segsText(hintSegs(hints, 15)); got != "a one   b two" {
		t.Errorf("hints cut = %q", got)
	}
}

func TestLocale(t *testing.T) {
	for locale, want := range map[string]bool{
		"": true, "C": false, "POSIX": false, "en_US.UTF-8": true, "en_US.utf8": true,
		"de_DE.ISO-8859-1": false, "en_US": true, "sr_RS.UTF-8@latin": true,
	} {
		if got := localeIsUTF8(locale); got != want {
			t.Errorf("localeIsUTF8(%q) = %v", locale, got)
		}
	}
}
