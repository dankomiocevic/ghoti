package metrics

import (
	"os"
	"path/filepath"
	"strings"
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

// --- metrics.go tests ---

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

// --- writer.go tests ---

func TestRotationFilenameDaily(t *testing.T) {
	ts := time.Date(2024, 2, 21, 15, 30, 0, 0, time.UTC)
	got := rotationFilename("/var/log", "daily", ts)
	want := "/var/log/metrics-2024-02-21.prom"
	if got != want {
		t.Errorf("daily rotation: got %q, want %q", got, want)
	}
}

func TestRotationFilenameHourly(t *testing.T) {
	ts := time.Date(2024, 2, 21, 15, 30, 0, 0, time.UTC)
	got := rotationFilename("/var/log", "hourly", ts)
	want := "/var/log/metrics-2024-02-21-15.prom"
	if got != want {
		t.Errorf("hourly rotation: got %q, want %q", got, want)
	}
}

func TestRotationFilenameUnknownDefaultsToDaily(t *testing.T) {
	ts := time.Date(2024, 2, 21, 15, 30, 0, 0, time.UTC)
	got := rotationFilename("/var/log", "weekly", ts)
	want := "/var/log/metrics-2024-02-21.prom"
	if got != want {
		t.Errorf("unknown rotation should default to daily: got %q, want %q", got, want)
	}
}

func TestFormatSnapshotContainsAllMetrics(t *testing.T) {
	s := Snapshot{
		Timestamp:         time.Date(2024, 2, 21, 12, 0, 0, 0, time.UTC),
		ConnectedClients:  42,
		RequestsPerSecond: 150.5,
		AvgLatencyMs:      1.23,
	}
	out := formatSnapshot(s)

	checks := []string{
		"ghoti_connected_clients 42",
		"ghoti_requests_per_second 150.500",
		"ghoti_request_duration_milliseconds 1.230",
		"# TYPE ghoti_connected_clients gauge",
		"# TYPE ghoti_requests_per_second gauge",
		"# TYPE ghoti_request_duration_milliseconds gauge",
	}
	for _, check := range checks {
		if !strings.Contains(out, check) {
			t.Errorf("output missing %q\nfull output:\n%s", check, out)
		}
	}
}

func TestFormatSnapshotTimestamp(t *testing.T) {
	ts := time.Date(2024, 2, 21, 12, 0, 0, 0, time.UTC)
	s := Snapshot{Timestamp: ts}
	out := formatSnapshot(s)
	// Timestamp in milliseconds: 1708516800000
	if !strings.Contains(out, "1708516800000") {
		t.Errorf("output missing expected millisecond timestamp\nfull output:\n%s", out)
	}
}

func TestPruneOldFiles(t *testing.T) {
	dir := t.TempDir()

	// Create 5 files.
	for _, name := range []string{
		"metrics-2024-02-17.prom",
		"metrics-2024-02-18.prom",
		"metrics-2024-02-19.prom",
		"metrics-2024-02-20.prom",
		"metrics-2024-02-21.prom",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	cfg := Config{OutputDir: dir, Retain: 3}
	if err := pruneOldFiles(cfg); err != nil {
		t.Fatalf("pruneOldFiles error: %v", err)
	}

	remaining, _ := filepath.Glob(filepath.Join(dir, "metrics-*.prom"))
	if len(remaining) != 3 {
		t.Errorf("expected 3 files after pruning, got %d: %v", len(remaining), remaining)
	}
	// The two oldest should have been deleted.
	for _, deleted := range []string{"metrics-2024-02-17.prom", "metrics-2024-02-18.prom"} {
		if _, err := os.Stat(filepath.Join(dir, deleted)); !os.IsNotExist(err) {
			t.Errorf("expected %s to be deleted", deleted)
		}
	}
}

func TestPruneOldFilesRetainZeroKeepsAll(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"metrics-a.prom", "metrics-b.prom", "metrics-c.prom"} {
		os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644)
	}

	cfg := Config{OutputDir: dir, Retain: 0}
	if err := pruneOldFiles(cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	remaining, _ := filepath.Glob(filepath.Join(dir, "metrics-*.prom"))
	if len(remaining) != 3 {
		t.Errorf("expected all 3 files kept, got %d", len(remaining))
	}
}

func TestWriteSnapshotCreatesFile(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{OutputDir: dir, Rotation: "daily", Retain: 7}

	s := Snapshot{
		Timestamp:         time.Date(2024, 2, 21, 10, 0, 0, 0, time.UTC),
		ConnectedClients:  5,
		RequestsPerSecond: 10.0,
		AvgLatencyMs:      2.5,
	}

	if err := writeSnapshot(cfg, s); err != nil {
		t.Fatalf("writeSnapshot error: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "metrics-2024-02-21.prom"))
	if err != nil {
		t.Fatalf("expected metrics file to exist: %v", err)
	}
	if !strings.Contains(string(content), "ghoti_connected_clients 5") {
		t.Errorf("file missing expected content:\n%s", content)
	}
}

func TestWriteSnapshotAppendsToExistingFile(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{OutputDir: dir, Rotation: "daily", Retain: 7}

	ts := time.Date(2024, 2, 21, 10, 0, 0, 0, time.UTC)
	s1 := Snapshot{Timestamp: ts, ConnectedClients: 1}
	s2 := Snapshot{Timestamp: ts, ConnectedClients: 2}

	if err := writeSnapshot(cfg, s1); err != nil {
		t.Fatal(err)
	}
	if err := writeSnapshot(cfg, s2); err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "metrics-2024-02-21.prom"))
	if err != nil {
		t.Fatal(err)
	}

	// Both snapshots should appear in the file.
	if !strings.Contains(string(content), "ghoti_connected_clients 1") ||
		!strings.Contains(string(content), "ghoti_connected_clients 2") {
		t.Errorf("file does not contain both snapshots:\n%s", content)
	}
}
