package metrics

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Config holds the metrics writer configuration. It is embedded in the
// top-level application config and populated from the "metrics:" YAML section.
type Config struct {
	// Enabled controls whether metrics are collected and written at all.
	// When false every metric call is a no-op and no files are created.
	Enabled bool

	// OutputDir is the directory where metrics files are written.
	// The directory is created if it does not exist.
	OutputDir string

	// Rotation controls how often a new file is started.
	// Supported values: "hourly", "daily" (default: "daily").
	Rotation string

	// Retain is the number of rotation files to keep. Older files are
	// deleted automatically. 0 means keep all files (default: 7).
	Retain int

	// Interval is the number of seconds between metric snapshots.
	// Must be >= 1 (default: 10).
	Interval int
}

const (
	defaultRotation = "daily"
	defaultRetain   = 7
	defaultInterval = 10
)

// Run starts the metrics writer and blocks until stop is closed.
// It should be called in a goroutine. Enable() must be called before Run.
func Run(cfg Config, stop <-chan struct{}) {
	if cfg.Interval < 1 {
		cfg.Interval = defaultInterval
	}
	if cfg.Rotation == "" {
		cfg.Rotation = defaultRotation
	}
	if cfg.Retain == 0 {
		cfg.Retain = defaultRetain
	}

	if err := os.MkdirAll(cfg.OutputDir, 0o755); err != nil {
		slog.Error("metrics: failed to create output directory",
			slog.String("dir", cfg.OutputDir),
			slog.Any("error", err))
		return
	}

	ticker := time.NewTicker(time.Duration(cfg.Interval) * time.Second)
	defer ticker.Stop()

	lastTick := time.Now()

	slog.Info("metrics: started",
		slog.String("output_dir", cfg.OutputDir),
		slog.String("rotation", cfg.Rotation),
		slog.Int("retain", cfg.Retain),
		slog.Int("interval_seconds", cfg.Interval))

	for {
		select {
		case <-stop:
			slog.Info("metrics: stopped")
			return
		case t := <-ticker.C:
			elapsed := t.Sub(lastTick).Seconds()
			lastTick = t

			s := TakeSnapshot(elapsed)
			if err := writeSnapshot(cfg, s); err != nil {
				slog.Error("metrics: failed to write snapshot", slog.Any("error", err))
			}
		}
	}
}

// writeSnapshot appends a formatted snapshot to the current rotation file
// and prunes old files when the retention limit is exceeded.
func writeSnapshot(cfg Config, s Snapshot) error {
	filename := rotationFilename(cfg.OutputDir, cfg.Rotation, s.Timestamp)

	f, err := os.OpenFile(filename, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open metrics file: %w", err)
	}
	defer f.Close()

	if _, err := f.WriteString(formatSnapshot(s)); err != nil {
		return fmt.Errorf("write metrics snapshot: %w", err)
	}

	return pruneOldFiles(cfg)
}

// rotationFilename returns the path for the active rotation file at time t.
// Files use UTC time so rotation boundaries are unambiguous.
func rotationFilename(dir, rotation string, t time.Time) string {
	var name string
	switch rotation {
	case "hourly":
		name = t.UTC().Format("metrics-2006-01-02-15.prom")
	default: // "daily"
		name = t.UTC().Format("metrics-2006-01-02.prom")
	}
	return filepath.Join(dir, name)
}

// formatSnapshot serialises s in the Prometheus text exposition format.
// Each snapshot is a self-contained block: HELP and TYPE lines are repeated
// so every block is independently parseable (useful for log-shipping tools).
// Timestamps are in milliseconds since the Unix epoch, as required by the
// Prometheus format.
func formatSnapshot(s Snapshot) string {
	tsMs := s.Timestamp.UnixMilli()
	var sb strings.Builder

	sb.WriteString("# HELP ghoti_connected_clients Number of currently connected clients\n")
	sb.WriteString("# TYPE ghoti_connected_clients gauge\n")
	fmt.Fprintf(&sb, "ghoti_connected_clients %d %d\n", s.ConnectedClients, tsMs)

	sb.WriteString("# HELP ghoti_requests_per_second Requests processed per second over the last interval\n")
	sb.WriteString("# TYPE ghoti_requests_per_second gauge\n")
	fmt.Fprintf(&sb, "ghoti_requests_per_second %.3f %d\n", s.RequestsPerSecond, tsMs)

	sb.WriteString("# HELP ghoti_request_duration_milliseconds Average request duration in milliseconds over the last interval\n")
	sb.WriteString("# TYPE ghoti_request_duration_milliseconds gauge\n")
	fmt.Fprintf(&sb, "ghoti_request_duration_milliseconds %.3f %d\n", s.AvgLatencyMs, tsMs)

	// Blank line between snapshots for readability.
	sb.WriteString("\n")

	return sb.String()
}

// pruneOldFiles removes the oldest metrics files when there are more than
// cfg.Retain files in the output directory. Files are sorted lexicographically;
// since filenames embed a UTC date (and optionally hour), this equals
// chronological order.
func pruneOldFiles(cfg Config) error {
	if cfg.Retain <= 0 {
		return nil
	}

	matches, err := filepath.Glob(filepath.Join(cfg.OutputDir, "metrics-*.prom"))
	if err != nil {
		return fmt.Errorf("enumerate metrics files: %w", err)
	}

	if len(matches) <= cfg.Retain {
		return nil
	}

	sort.Strings(matches)
	toDelete := matches[:len(matches)-cfg.Retain]
	for _, f := range toDelete {
		if err := os.Remove(f); err != nil {
			// Log but do not fail; a missing file is not worth stopping collection.
			slog.Warn("metrics: failed to remove old file",
				slog.String("file", f),
				slog.Any("error", err))
		}
	}
	return nil
}
