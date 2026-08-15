package airplay

import (
	"math"
	"sync/atomic"
	"time"
)

const defaultTargetLatency = 1 * time.Millisecond

// conservativePlayoutLatency is the playout lead required by third-party
// receivers that lack a robust audio jitter buffer (Roku and similar, which
// do not advertise FairPlay SAP). The control-port sync anchor reports that
// the newest audio frame plays this far in the future, which is also the
// buffer lead the receiver has to schedule each packet before its play time.
// With too little lead these receivers drop audio they can no longer schedule.
const conservativePlayoutLatency = 500 * time.Millisecond

// legacyApplePlayoutLatency is the floor for AppleTV2/3. Those boxes advertise
// FairPlay SAP but still use the original RAOP buffer. RECORD reports ~54
// samples (~1.2ms) of hardware render delay there; that is not a jitter
// buffer. 100ms matches the default -target-latency-ms and covers a PipeWire
// quantum (~21ms), one ALAC frame (8ms), and typical Wi-Fi jitter. It is not
// a measured hardware minimum — raise -target-latency-ms if the network is
// worse, rather than treating the Roku 500ms floor as required.
const legacyApplePlayoutLatency = 100 * time.Millisecond

var targetLatencyNS atomic.Int64

func init() {
	targetLatencyNS.Store(int64(defaultTargetLatency))
}

// SetTargetLatency sets the desired end-to-end playout latency target.
// Values are clamped to a sane operational range.
func SetTargetLatency(d time.Duration) {
	if d < 5*time.Millisecond {
		d = 5 * time.Millisecond
	}
	if d > 2*time.Second {
		d = 2 * time.Second
	}
	targetLatencyNS.Store(int64(d))
}

// TargetLatency returns the configured playout latency target.
func TargetLatency() time.Duration {
	d := time.Duration(targetLatencyNS.Load())
	if d <= 0 {
		return defaultTargetLatency
	}
	return d
}

func targetLatencySamples44k1() uint32 {
	return samplesFor44k1(TargetLatency())
}

func samplesFor44k1(d time.Duration) uint32 {
	samples := int64(math.Round(float64(d) * 44100.0 / float64(time.Second)))
	if samples < 1 {
		samples = 1
	}
	if samples > math.MaxUint32 {
		samples = math.MaxUint32
	}
	return uint32(samples)
}

// adoptRecordAudioLatency updates the session playout lead from a RECORD
// Audio-Latency header. The header may only raise the lead. A value below the
// current lead or the per-receiver floor is ignored: AppleTV3 reports ~54
// samples here (about 1.2ms of hardware render delay), which is not a usable
// jitter buffer and garbles as soon as capture jitter exceeds that window.
func adoptRecordAudioLatency(currentSamples uint32, currentLatency, floor time.Duration, recordSamples uint64) (uint32, time.Duration) {
	if recordSamples > 0 && recordSamples > uint64(currentSamples) {
		currentSamples = uint32(recordSamples)
		currentLatency = time.Duration(recordSamples) * time.Second / 44100
	}
	if currentLatency < floor {
		currentLatency = floor
		currentSamples = samplesFor44k1(floor)
	}
	return currentSamples, currentLatency
}
