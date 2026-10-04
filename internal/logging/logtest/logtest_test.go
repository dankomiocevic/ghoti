package logtest

import (
	"io"
	"log/slog"
	"testing"

	"github.com/dankomiocevic/ghoti/internal/logging"
)

func TestEnableDebug(t *testing.T) {
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelInfo})))
	defer slog.SetDefault(previous)
	infoLogger := slog.Default()

	t.Run("enabled during the test", func(t *testing.T) {
		EnableDebug(t)
		if !logging.DebugEnabled() {
			t.Fatal("DebugEnabled() = false after EnableDebug, want true")
		}
	})

	if slog.Default() != infoLogger {
		t.Fatal("the previous logger was not restored after the test finished")
	}
	if logging.DebugEnabled() {
		t.Fatal("DebugEnabled() = true after the test finished, want false")
	}
}
