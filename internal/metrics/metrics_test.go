package metrics

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// resetGlobal resets the package-level collector between tests.
func resetGlobal() {
	atomic.StoreInt32(&global.enabled, 0)
	global.connectedClients.Store(0)
	global.requestCount.Store(0)
	global.latencyNsSum.Store(0)
	global.latencyCount.Store(0)
}

func TestDisabledMetricsAreNoOps(t *testing.T) {
	resetGlobal()
	// All calls should be no-ops when disabled.
	IncrConnectedClients()
	IncrConnectedClients()
	DecrConnectedClients()
	RecordRequest(5 * time.Millisecond)

	s := TakeSnapshot(1.0)
	if s.ConnectedClients != 0 {
		t.Errorf("expected 0 connected clients, got %d", s.ConnectedClients)
	}
	if s.RequestsPerSecond != 0 {
		t.Errorf("expected 0 rps, got %.3f", s.RequestsPerSecond)
	}
}

func TestConnectedClientsGauge(t *testing.T) {
	resetGlobal()
	Enable()

	IncrConnectedClients()
	IncrConnectedClients()
	IncrConnectedClients()
	DecrConnectedClients()

	s := TakeSnapshot(1.0)
	if s.ConnectedClients != 2 {
		t.Errorf("expected 2 connected clients, got %d", s.ConnectedClients)
	}
}

func TestRequestsPerSecond(t *testing.T) {
	resetGlobal()
	Enable()

	// 100 requests over a 2-second window → 50 rps
	for i := 0; i < 100; i++ {
		RecordRequest(time.Millisecond)
	}

	s := TakeSnapshot(2.0)
	if s.RequestsPerSecond != 50.0 {
		t.Errorf("expected 50.0 rps, got %.3f", s.RequestsPerSecond)
	}
}

func TestAverageLatency(t *testing.T) {
	resetGlobal()
	Enable()

	// Two requests: 4 ms and 6 ms → average 5 ms
	RecordRequest(4 * time.Millisecond)
	RecordRequest(6 * time.Millisecond)

	s := TakeSnapshot(1.0)
	if s.AvgLatencyMs < 4.9 || s.AvgLatencyMs > 5.1 {
		t.Errorf("expected ~5.0 ms average latency, got %.3f", s.AvgLatencyMs)
	}
}

func TestSnapshotResetsIntervalCounters(t *testing.T) {
	resetGlobal()
	Enable()

	RecordRequest(10 * time.Millisecond)
	RecordRequest(10 * time.Millisecond)

	_ = TakeSnapshot(1.0) // first snapshot drains the counters

	s := TakeSnapshot(1.0) // second snapshot: nothing recorded
	if s.RequestsPerSecond != 0 {
		t.Errorf("expected 0 rps after reset, got %.3f", s.RequestsPerSecond)
	}
	if s.AvgLatencyMs != 0 {
		t.Errorf("expected 0 ms latency after reset, got %.3f", s.AvgLatencyMs)
	}
}

func TestSnapshotDoesNotResetGauge(t *testing.T) {
	resetGlobal()
	Enable()

	IncrConnectedClients()
	IncrConnectedClients()

	_ = TakeSnapshot(1.0)

	s := TakeSnapshot(1.0)
	if s.ConnectedClients != 2 {
		t.Errorf("gauge must survive snapshot reset; expected 2, got %d", s.ConnectedClients)
	}
}

func TestZeroElapsedDoesNotPanic(t *testing.T) {
	resetGlobal()
	Enable()
	RecordRequest(time.Millisecond)
	s := TakeSnapshot(0)
	if s.RequestsPerSecond != 0 {
		t.Errorf("expected 0 rps for zero elapsed, got %.3f", s.RequestsPerSecond)
	}
}

func TestConcurrentRecordRequest(t *testing.T) {
	resetGlobal()
	Enable()

	const goroutines = 50
	const requestsPerGoroutine = 200

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < requestsPerGoroutine; j++ {
				RecordRequest(time.Millisecond)
			}
		}()
	}
	wg.Wait()

	s := TakeSnapshot(1.0)
	expected := float64(goroutines * requestsPerGoroutine)
	if s.RequestsPerSecond != expected {
		t.Errorf("expected %.0f rps, got %.3f", expected, s.RequestsPerSecond)
	}
}

func TestConcurrentConnectedClients(t *testing.T) {
	resetGlobal()
	Enable()

	const goroutines = 100
	var wg sync.WaitGroup
	wg.Add(goroutines * 2)
	for i := 0; i < goroutines; i++ {
		go func() { defer wg.Done(); IncrConnectedClients() }()
		go func() { defer wg.Done(); DecrConnectedClients() }()
	}
	wg.Wait()

	s := TakeSnapshot(1.0)
	if s.ConnectedClients != 0 {
		t.Errorf("expected 0 after equal incr/decr, got %d", s.ConnectedClients)
	}
}
