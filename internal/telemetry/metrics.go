// Package telemetry exposes Ghoti runtime metrics to Prometheus.
//
// Metrics are registered on a private registry together with the standard Go
// runtime and process collectors, and served on a dedicated HTTP listener at
// GET /metrics in the Prometheus text exposition format. All hot-path
// operations are a single atomic add on a client_golang counter, gauge or
// histogram, so instrumentation adds negligible overhead to request handling.
//
// Usage:
//
//	// In main / run command, after loading config:
//	if cfg.Metrics.Enabled {
//	    telemetry.Enable()
//	    srv := telemetry.NewServer(cfg.Metrics)
//	    if err := srv.Start(); err != nil { ... }
//	    defer srv.Stop()
//	}
//
//	// In connection manager:
//	telemetry.IncrConnectedClients()
//	defer telemetry.DecrConnectedClients()
//
//	// In request handler:
//	start := time.Now()
//	defer func() { telemetry.RecordRequest(time.Since(start)) }()
package telemetry

import (
	"net/http"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// requestDurationBuckets covers the latency range of an in-memory server:
// from tens of microseconds up to a second, so tail latency is visible
// instead of being flattened into a single average.
var requestDurationBuckets = []float64{
	0.00005, 0.0001, 0.00025, 0.0005,
	0.001, 0.0025, 0.005, 0.01,
	0.025, 0.05, 0.1, 0.25, 0.5, 1,
}

// collector owns the registry and every Ghoti metric. The zero value is not
// usable; create one with newCollector.
type collector struct {
	// enabled is an atomic boolean (0 = disabled, 1 = enabled).
	// Checked on every metric call so disabled metrics cost only one
	// atomic load per call site.
	enabled int32

	registry *prometheus.Registry

	connectedClients prometheus.Gauge
	requestsTotal    prometheus.Counter
	requestDuration  prometheus.Histogram
}

// newCollector builds a collector with all Ghoti metrics plus the standard
// Go runtime and process collectors registered on a fresh registry.
func newCollector() *collector {
	c := &collector{
		registry: prometheus.NewRegistry(),
		connectedClients: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "ghoti_connected_clients",
			Help: "Number of currently connected clients.",
		}),
		requestsTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "ghoti_requests_total",
			Help: "Total number of requests processed.",
		}),
		requestDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "ghoti_request_duration_seconds",
			Help:    "Request processing duration in seconds.",
			Buckets: requestDurationBuckets,
		}),
	}

	c.registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		c.connectedClients,
		c.requestsTotal,
		c.requestDuration,
	)

	return c
}

// global is the package-level singleton collector.
var global = newCollector()

// Enable activates metric collection. Safe to call multiple times.
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
	global.connectedClients.Inc()
}

// DecrConnectedClients decrements the connected-clients gauge by one.
// No-op when metrics are disabled.
func DecrConnectedClients() {
	if !isEnabled() {
		return
	}
	global.connectedClients.Dec()
}

// RecordRequest records a completed request and its wall-clock duration.
// No-op when metrics are disabled.
func RecordRequest(d time.Duration) {
	if !isEnabled() {
		return
	}
	global.requestsTotal.Inc()
	global.requestDuration.Observe(d.Seconds())
}

// Handler returns the HTTP handler that serves the Prometheus text
// exposition for every registered metric.
func Handler() http.Handler {
	return promhttp.HandlerFor(global.registry, promhttp.HandlerOpts{})
}
