package telemetry

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
)

// resetGlobal replaces the package-level collector with a fresh one so
// tests do not observe each other's counters.
func resetGlobal() {
	global = newCollector()
}

// scrape performs a GET against the metrics handler and parses the response
// as Prometheus text exposition, keyed by metric family name.
func scrape(t *testing.T) map[string]*dto.MetricFamily {
	t.Helper()

	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 from /metrics, got %d: %s", rec.Code, rec.Body.String())
	}

	parser := expfmt.NewTextParser(model.LegacyValidation)
	families, err := parser.TextToMetricFamilies(rec.Body)
	if err != nil {
		t.Fatalf("scrape output is not valid Prometheus text exposition: %s", err)
	}
	return families
}

// singleValue returns the value of the only sample in a family (gauge or counter).
func singleValue(t *testing.T, families map[string]*dto.MetricFamily, name string) float64 {
	t.Helper()

	fam, ok := families[name]
	if !ok {
		t.Fatalf("metric %q not present in scrape", name)
	}
	if len(fam.GetMetric()) != 1 {
		t.Fatalf("expected exactly one sample for %q, got %d", name, len(fam.GetMetric()))
	}
	m := fam.GetMetric()[0]
	switch fam.GetType() {
	case dto.MetricType_GAUGE:
		return m.GetGauge().GetValue()
	case dto.MetricType_COUNTER:
		return m.GetCounter().GetValue()
	default:
		t.Fatalf("metric %q has unexpected type %s", name, fam.GetType())
		return 0
	}
}

func TestDisabledMetricsAreNoOps(t *testing.T) {
	resetGlobal()

	IncrConnectedClients()
	IncrConnectedClients()
	DecrConnectedClients()
	RecordRequest(5 * time.Millisecond)

	families := scrape(t)
	if v := singleValue(t, families, "ghoti_connected_clients"); v != 0 {
		t.Errorf("expected 0 connected clients, got %v", v)
	}
	if v := singleValue(t, families, "ghoti_requests_total"); v != 0 {
		t.Errorf("expected 0 requests, got %v", v)
	}
}

func TestConnectedClientsGauge(t *testing.T) {
	resetGlobal()
	Enable()

	IncrConnectedClients()
	IncrConnectedClients()
	IncrConnectedClients()
	DecrConnectedClients()

	families := scrape(t)
	if fam := families["ghoti_connected_clients"]; fam.GetType() != dto.MetricType_GAUGE {
		t.Errorf("ghoti_connected_clients must be a gauge, got %s", fam.GetType())
	}
	if v := singleValue(t, families, "ghoti_connected_clients"); v != 2 {
		t.Errorf("expected 2 connected clients, got %v", v)
	}
}

func TestRequestsTotalIsCumulativeAcrossScrapes(t *testing.T) {
	resetGlobal()
	Enable()

	for i := 0; i < 3; i++ {
		RecordRequest(time.Millisecond)
	}
	families := scrape(t)
	if fam := families["ghoti_requests_total"]; fam.GetType() != dto.MetricType_COUNTER {
		t.Errorf("ghoti_requests_total must be a counter, got %s", fam.GetType())
	}
	if v := singleValue(t, families, "ghoti_requests_total"); v != 3 {
		t.Fatalf("expected 3 requests after first scrape, got %v", v)
	}

	// A second scrape must not reset the counter: Prometheus derives rates
	// from cumulative values.
	RecordRequest(time.Millisecond)
	RecordRequest(time.Millisecond)
	families = scrape(t)
	if v := singleValue(t, families, "ghoti_requests_total"); v != 5 {
		t.Errorf("expected 5 requests after second scrape, got %v", v)
	}
}

func TestRequestDurationHistogramInSeconds(t *testing.T) {
	resetGlobal()
	Enable()

	RecordRequest(100 * time.Microsecond)
	RecordRequest(5 * time.Millisecond)
	RecordRequest(2 * time.Second)

	families := scrape(t)
	fam, ok := families["ghoti_request_duration_seconds"]
	if !ok {
		t.Fatalf("ghoti_request_duration_seconds not present in scrape")
	}
	if fam.GetType() != dto.MetricType_HISTOGRAM {
		t.Fatalf("ghoti_request_duration_seconds must be a histogram, got %s", fam.GetType())
	}
	if len(fam.GetMetric()) != 1 {
		t.Fatalf("expected one histogram sample, got %d", len(fam.GetMetric()))
	}

	h := fam.GetMetric()[0].GetHistogram()
	if h.GetSampleCount() != 3 {
		t.Errorf("expected sample count 3, got %d", h.GetSampleCount())
	}
	wantSum := 0.0001 + 0.005 + 2
	if diff := h.GetSampleSum() - wantSum; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("expected sample sum %v seconds, got %v", wantSum, h.GetSampleSum())
	}

	// The buckets must resolve sub-millisecond latencies so tails are
	// visible for an in-memory server: the 100µs sample must land in a
	// bucket that excludes the 5ms sample.
	cumulative := map[float64]uint64{}
	for _, b := range h.GetBucket() {
		cumulative[b.GetUpperBound()] = b.GetCumulativeCount()
	}
	if c, ok := cumulative[0.001]; !ok {
		t.Errorf("expected a 1ms (le=0.001) bucket, got bounds %v", bucketBounds(h))
	} else if c != 1 {
		t.Errorf("expected 1 sample <= 1ms, got %d", c)
	}
	if c, ok := cumulative[1]; !ok {
		t.Errorf("expected a 1s (le=1) bucket, got bounds %v", bucketBounds(h))
	} else if c != 2 {
		t.Errorf("expected 2 samples <= 1s, got %d", c)
	}
}

func bucketBounds(h *dto.Histogram) []float64 {
	bounds := make([]float64, 0, len(h.GetBucket()))
	for _, b := range h.GetBucket() {
		bounds = append(bounds, b.GetUpperBound())
	}
	return bounds
}

func TestStandardCollectorsAreExposed(t *testing.T) {
	resetGlobal()

	families := scrape(t)
	for _, name := range []string{"go_goroutines", "process_cpu_seconds_total"} {
		if _, ok := families[name]; !ok {
			t.Errorf("expected standard collector metric %q in scrape", name)
		}
	}
}

func TestConcurrentRecordRequest(t *testing.T) {
	resetGlobal()
	Enable()

	const goroutines = 50
	const perGoroutine = 200

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < perGoroutine; j++ {
				IncrConnectedClients()
				RecordRequest(time.Microsecond)
				DecrConnectedClients()
			}
		}()
	}
	wg.Wait()

	families := scrape(t)
	if v := singleValue(t, families, "ghoti_requests_total"); v != goroutines*perGoroutine {
		t.Errorf("expected %d requests, got %v", goroutines*perGoroutine, v)
	}
	if v := singleValue(t, families, "ghoti_connected_clients"); v != 0 {
		t.Errorf("expected 0 connected clients after all disconnect, got %v", v)
	}
}
