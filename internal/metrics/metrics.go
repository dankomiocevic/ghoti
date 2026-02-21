// Package metrics provides a lightweight, lock-free metrics collection system
// for Ghoti. All counter and gauge operations use atomic primitives so they
// impose negligible overhead on request-handling goroutines.
//
// Metrics are written to rotating files in the Prometheus exposition format
// (https://prometheus.io/docs/instrumenting/exposition_formats/) by a
// dedicated background goroutine; the hot-path code only performs atomic
// add/swap operations.
//
// Usage:
//
//	// In main / run command, after loading config:
//	if cfg.Metrics.Enabled {
//	    metrics.Enable()
//	    stop := make(chan struct{})
//	    go metrics.Run(cfg.Metrics, stop)
//	    defer close(stop)
//	}
//
//	// In connection manager:
//	metrics.IncrConnectedClients()
//	defer metrics.DecrConnectedClients()
//
//	// In request handler:
//	start := time.Now()
//	defer func() { metrics.RecordRequest(time.Since(start)) }()
package metrics

import (
	"sync/atomic"
	"time"
)

// collector holds all metrics using lock-free atomic operations.
// The zero value is safe: all operations are no-ops until Enable() is called.
type collector struct {
	// enabled is an atomic boolean (0 = disabled, 1 = enabled).
	// Checked on every metric call so disabled metrics cost only one
	// atomic load per call site.
	enabled int32

	// connectedClients is a gauge: incremented on connect, decremented on disconnect.
	connectedClients atomic.Int64

	// The following three are interval accumulators reset on each snapshot.

	// requestCount counts requests since the last snapshot.
	requestCount atomic.Uint64

	// latencyNsSum is the sum of request durations in nanoseconds since the last snapshot.
	latencyNsSum atomic.Int64

	// latencyCount is the number of latency samples since the last snapshot.
	latencyCount atomic.Uint64
}

// global is the package-level singleton collector.
var global collector

// Snapshot holds a point-in-time reading of all metrics.
type Snapshot struct {
	// Timestamp is when the snapshot was taken.
	Timestamp time.Time

	// ConnectedClients is the number of clients connected at snapshot time.
	ConnectedClients int64

	// RequestsPerSecond is the observed request rate over the last interval.
	RequestsPerSecond float64

	// AvgLatencyMs is the mean request duration in milliseconds over the last interval.
	// Zero means no requests were recorded in the interval.
	AvgLatencyMs float64
}

// Enable activates metric collection. Must be called before starting the
// background writer (metrics.Run). Safe to call multiple times.
func Enable() {
	atomic.StoreInt32(&global.enabled, 1)
}

// isEnabled returns true if metrics collection is active.
func isEnabled() bool {
	return atomic.LoadInt32(&global.enabled) == 1
}

// IncrConnectedClients increments the connected-clients gauge by one.
// No-op when metrics are disabled.
func IncrConnectedClients() {
	if !isEnabled() {
		return
	}
	global.connectedClients.Add(1)
}

// DecrConnectedClients decrements the connected-clients gauge by one.
// No-op when metrics are disabled.
func DecrConnectedClients() {
	if !isEnabled() {
		return
	}
	global.connectedClients.Add(-1)
}

// RecordRequest records a completed request and its wall-clock duration.
// No-op when metrics are disabled.
func RecordRequest(d time.Duration) {
	if !isEnabled() {
		return
	}
	global.requestCount.Add(1)
	global.latencyNsSum.Add(int64(d))
	global.latencyCount.Add(1)
}

// TakeSnapshot collects a point-in-time reading and atomically resets the
// interval accumulators (request count, latency). The connected-clients gauge
// is not reset. elapsed is the number of seconds since the previous snapshot
// and is used to compute the requests-per-second rate.
func TakeSnapshot(elapsed float64) Snapshot {
	reqCount := global.requestCount.Swap(0)
	latNs := global.latencyNsSum.Swap(0)
	latCount := global.latencyCount.Swap(0)

	var rps float64
	if elapsed > 0 {
		rps = float64(reqCount) / elapsed
	}

	var avgMs float64
	if latCount > 0 {
		avgMs = float64(latNs) / float64(latCount) / 1e6
	}

	return Snapshot{
		Timestamp:         time.Now(),
		ConnectedClients:  global.connectedClients.Load(),
		RequestsPerSecond: rps,
		AvgLatencyMs:      avgMs,
	}
}
