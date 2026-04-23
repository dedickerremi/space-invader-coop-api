package game

import (
	"sort"
	"sync"
	"time"
)

// Game-loop performance is dashboard-critical: a tick budget overrun
// (loop work > 1/30s) means frames drop and clients see lag. We keep a
// rolling window of the last metricsWindow samples for tick duration
// and broadcast duration; percentiles are computed lazily on read.
//
// metricsWindow = 1000 samples ≈ 33s at 30 Hz. Big enough to smooth
// transient hiccups, small enough to react to a real regression within
// a half-minute on the dashboard.
const metricsWindow = 1000

// Tick budget at 30 Hz. Surfaced to the dashboard so the operator can
// see the headroom in absolute terms.
const tickBudgetMicros = int64(time.Second / 30 / time.Microsecond) // ≈ 33333

type metricRing struct {
	mu      sync.Mutex
	samples [metricsWindow]int64 // microseconds
	idx     int
	filled  int
	total   uint64
}

func (r *metricRing) record(d time.Duration) {
	micros := d.Microseconds()
	r.mu.Lock()
	r.samples[r.idx] = micros
	r.idx = (r.idx + 1) % metricsWindow
	if r.filled < metricsWindow {
		r.filled++
	}
	r.total++
	r.mu.Unlock()
}

func (r *metricRing) snapshot() DurationStats {
	r.mu.Lock()
	n := r.filled
	total := r.total
	if n == 0 {
		r.mu.Unlock()
		return DurationStats{}
	}
	buf := make([]int64, n)
	copy(buf, r.samples[:n])
	r.mu.Unlock()

	sort.Slice(buf, func(i, j int) bool { return buf[i] < buf[j] })

	var sum int64
	for _, v := range buf {
		sum += v
	}
	return DurationStats{
		Count:     total,
		AvgMicros: sum / int64(n),
		P50Micros: percentile(buf, 0.50),
		P95Micros: percentile(buf, 0.95),
		P99Micros: percentile(buf, 0.99),
		MaxMicros: buf[n-1],
	}
}

func percentile(sorted []int64, p float64) int64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(float64(len(sorted)-1) * p)
	return sorted[idx]
}

// DurationStats is a snapshot of one timing metric. All durations are
// microseconds — the dashboard formats them as the unit makes sense.
type DurationStats struct {
	Count     uint64 `json:"count"`     // total observations since boot
	AvgMicros int64  `json:"avgMicros"` // mean over the window
	P50Micros int64  `json:"p50Micros"`
	P95Micros int64  `json:"p95Micros"`
	P99Micros int64  `json:"p99Micros"`
	MaxMicros int64  `json:"maxMicros"`
}

// LoopMetrics is the full set of game-loop timing snapshots returned to
// the monitoring layer. Captured once per /api/stats request.
type LoopMetrics struct {
	Tick             DurationStats `json:"tick"`
	Broadcast        DurationStats `json:"broadcast"`
	TickBudgetMicros int64         `json:"tickBudgetMicros"`
}

var (
	tickRing      metricRing
	broadcastRing metricRing
)

// RecordTick is called from StartLoop after each Tick() returns.
func RecordTick(d time.Duration) { tickRing.record(d) }

// RecordBroadcast is called from StartLoop after the per-tick broadcast
// finishes. Captures everything the hub did for this frame, across all
// matches — single value per tick, not per match.
func RecordBroadcast(d time.Duration) { broadcastRing.record(d) }

// GetMetrics returns the current snapshot of loop timings. Safe to call
// from any goroutine.
func GetMetrics() LoopMetrics {
	return LoopMetrics{
		Tick:             tickRing.snapshot(),
		Broadcast:        broadcastRing.snapshot(),
		TickBudgetMicros: tickBudgetMicros,
	}
}
