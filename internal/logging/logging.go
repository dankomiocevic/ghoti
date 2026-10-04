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
// slog drops a DEBUG line when the level is higher, but the arguments of the
// call are evaluated before slog gets the chance to check it. Things like
// RemoteAddr().String(), string concatenation or wrapping every attribute in
// an interface still allocate on every call. Wrapping the call with
// DebugEnabled avoids that work on the hot paths when DEBUG is not enabled.
//
// The level is read from the default logger on every call instead of being
// cached, so it keeps working if the logger is replaced (tests do this).
func DebugEnabled() bool {
	return slog.Default().Enabled(context.Background(), slog.LevelDebug)
}
