package radio

import (
	"fmt"
	"math"
	"strings"
)

const bandCount = 12

// bandFreqs are the band centres in Hz, bass first: 40 Hz to 12 kHz in equal
// steps of about three quarters of an octave.
var bandFreqs = [bandCount]int{40, 67, 113, 190, 318, 535, 898, 1508, 2533, 4254, 7145, 12000}

// Bars span spectrumFloorDB to 0 dBFS. Band-limited RMS sits well below the
// overall level, and music carries less energy in the treble, so the tilt
// lowers the bass and raises the treble by half of spectrumTiltDB each. Pink
// noise at a typical radio level of -17 dBFS then hovers around mid-height.
const (
	spectrumFloorDB = -50.0
	spectrumTiltDB  = 10.0
)

// spectrumGraph splits a mono copy of the audio into one-octave bands and
// merges them as extra channels next to the untouched stereo, so that one
// astats reports every level. pan drops the bands again before the output.
func spectrumGraph() string {
	var sb strings.Builder
	sb.WriteString("aformat=channel_layouts=stereo,asplit=2[main][mono];")
	fmt.Fprintf(&sb, "[mono]aformat=channel_layouts=mono,asplit=%d", bandCount)
	for i := range bandCount {
		fmt.Fprintf(&sb, "[b%d]", i)
	}
	for i, f := range bandFreqs {
		fmt.Fprintf(&sb, ";[b%d]bandpass=f=%d:width_type=o:w=1[m%d]", i, f, i)
	}
	sb.WriteString(";[main]")
	for i := range bandCount {
		fmt.Fprintf(&sb, "[m%d]", i)
	}
	// Without threads=1, astats wakes a thread per channel on every frame,
	// which doubles the cost of the graph.
	fmt.Fprintf(&sb, "amerge=inputs=%d,"+
		"astats=threads=1:metadata=1:reset=1:measure_perchannel=RMS_level:measure_overall=none,"+
		"pan=stereo|c0=c0|c1=c1", bandCount+1)
	return sb.String()
}

// bandLevel maps a band's RMS level in dBFS to a bar height between 0 and 1.
// A band centred at or above half the sample rate reads as silent: FFmpeg 4.4
// and later pass it through unfiltered.
func bandLevel(band int, db float64, sampleRate int) float64 {
	if math.IsNaN(db) || (sampleRate > 0 && 2*bandFreqs[band] >= sampleRate) {
		return 0
	}
	tilt := spectrumTiltDB * (float64(band)/(bandCount-1) - 0.5)
	return min(max(1-(db+tilt)/spectrumFloorDB, 0), 1)
}
