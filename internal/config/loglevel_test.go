package config

import (
	"log/slog"
	"testing"
)

// Every supported log level must map to its own slog level. Mapping "info"
// to LevelDebug silently turns on debug logging for anybody running with the
// ordinary production log level, which is how debug only details end up in
// production logs.
func TestSupportedLogLevelsMapToMatchingLevels(t *testing.T) {
	expected := map[string]slog.Level{
		"debug": slog.LevelDebug,
		"info":  slog.LevelInfo,
		"warn":  slog.LevelWarn,
		"error": slog.LevelError,
	}

	for name, want := range expected {
		got, ok := SupportedLogLevels[name]
		if !ok {
			t.Fatalf("log level %q is not supported", name)
		}
		if got != want {
			t.Errorf("log level %q maps to %v, expected %v", name, got, want)
		}
	}
}

func TestInfoLogLevelDoesNotEnableDebug(t *testing.T) {
	if SupportedLogLevels["info"] <= slog.LevelDebug {
		t.Fatalf("the info log level must not enable debug records")
	}
}
