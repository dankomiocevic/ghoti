// Package logtest provides logging helpers for tests. It lives apart from the
// logging package so production code does not link the testing package.
package logtest

import (
	"io"
	"log/slog"
	"testing"
)

// EnableDebug sets the default logger to DEBUG for the duration of the test
// and puts the previous logger back when the test finishes.
//
// The default logger is global, so this must not be used in tests that call
// t.Parallel.
func EnableDebug(t testing.TB) {
	t.Helper()

	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous) })
}
