package logging

import (
	"io"
	"log/slog"
	"testing"
)

func withLevel(t *testing.T, level slog.Level) {
	t.Helper()

	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: level})))
	t.Cleanup(func() { slog.SetDefault(previous) })
}

func TestDebugEnabled(t *testing.T) {
	tests := []struct {
		level slog.Level
		want  bool
	}{
		{slog.LevelDebug, true},
		{slog.LevelInfo, false},
		{slog.LevelWarn, false},
		{slog.LevelError, false},
	}

	for _, tt := range tests {
		t.Run(tt.level.String(), func(t *testing.T) {
			withLevel(t, tt.level)
			if got := DebugEnabled(); got != tt.want {
				t.Fatalf("DebugEnabled() with level %s = %v, want %v", tt.level, got, tt.want)
			}
		})
	}
}

func TestDebugEnabledDoesNotAllocate(t *testing.T) {
	withLevel(t, slog.LevelInfo)

	allocs := testing.AllocsPerRun(100, func() { DebugEnabled() })
	if allocs != 0 {
		t.Fatalf("DebugEnabled() allocated %v times, want 0", allocs)
	}
}
