package telemetry

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

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

func TestRunWritesMetricsFile(t *testing.T) {
	resetGlobal()
	Enable()

	dir := t.TempDir()
	cfg := Config{
		Enabled:   true,
		OutputDir: dir,
		Rotation:  "daily",
		Retain:    7,
		Interval:  1,
	}

	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		Run(cfg, stop)
		close(done)
	}()

	// Record some activity so the snapshot is non-trivial.
	IncrConnectedClients()
	RecordRequest(2 * time.Millisecond)

	// Wait long enough for at least one tick (interval = 1 s).
	time.Sleep(1500 * time.Millisecond)

	close(stop)
	<-done

	matches, err := filepath.Glob(filepath.Join(dir, "metrics-*.prom"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) == 0 {
		t.Fatal("expected at least one metrics file to be written")
	}

	content, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "ghoti_connected_clients") {
		t.Errorf("metrics file missing expected content:\n%s", content)
	}
}

func TestRunCreatesOutputDir(t *testing.T) {
	resetGlobal()
	Enable()

	// Use a nested path that does not exist yet.
	dir := filepath.Join(t.TempDir(), "nested", "metrics")
	cfg := Config{
		Enabled:   true,
		OutputDir: dir,
		Rotation:  "daily",
		Retain:    7,
		Interval:  1,
	}

	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		Run(cfg, stop)
		close(done)
	}()

	time.Sleep(1500 * time.Millisecond)
	close(stop)
	<-done

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Errorf("expected output directory %q to be created", dir)
	}
}

func TestRunInvalidOutputDirStops(t *testing.T) {
	resetGlobal()
	Enable()

	// Pass a path where a file already exists in place of the directory,
	// so MkdirAll will fail.
	f, err := os.CreateTemp("", "ghoti-metrics-*.prom")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	defer os.Remove(f.Name())

	cfg := Config{
		Enabled:   true,
		OutputDir: filepath.Join(f.Name(), "subdir"), // file used as parent: invalid
		Rotation:  "daily",
		Retain:    7,
		Interval:  1,
	}

	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		Run(cfg, stop)
		close(done)
	}()

	// Run should return quickly because MkdirAll fails.
	select {
	case <-done:
		// expected
	case <-time.After(3 * time.Second):
		close(stop)
		t.Error("Run did not exit after invalid output directory")
	}
}
