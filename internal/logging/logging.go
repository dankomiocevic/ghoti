// Package logging provides small helpers around log/slog that are shared by
// the rest of the project.
package logging

import (
	"context"
	"log/slog"
)

// DebugEnabled reports whether the default logger is going to emit DEBUG
// lines.
//
// The level is read from the default logger on every call instead of being
// cached, so it keeps working if the logger is replaced (tests do this).
func DebugEnabled() bool {
	return slog.Default().Enabled(context.Background(), slog.LevelDebug)
}
