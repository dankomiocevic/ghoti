package server

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

// captureLogs runs fn with the default logger pointed at a buffer set to the
// most verbose level, and returns everything that was written.
func captureLogs(t *testing.T, fn func()) string {
	t.Helper()

	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	defer slog.SetDefault(previous)

	fn()
	return buf.String()
}

func TestParseMessageDoesNotLogPassword(t *testing.T) {
	const password = "sup3rs3cr3tpassword"

	output := captureLogs(t, func() {
		input := "p" + password
		if _, err := ParseMessage(len(input), []byte(input)); err != nil {
			t.Fatalf("unexpected error parsing password command: %v", err)
		}
	})

	if strings.Contains(output, password) {
		t.Fatalf("password value leaked into the logs: %s", output)
	}
}

func TestParseMessageDoesNotLogUsername(t *testing.T) {
	const username = "someusername"

	output := captureLogs(t, func() {
		input := "u" + username
		if _, err := ParseMessage(len(input), []byte(input)); err != nil {
			t.Fatalf("unexpected error parsing user command: %v", err)
		}
	})

	if strings.Contains(output, username) {
		t.Fatalf("username value leaked into the logs: %s", output)
	}
}

// Non credential commands must still be logged in full, the redaction is
// only meant to cover the two commands that carry credentials.
func TestParseMessageStillLogsNonCredentialCommands(t *testing.T) {
	output := captureLogs(t, func() {
		if _, err := ParseMessage(9, []byte("w999Hello")); err != nil {
			t.Fatalf("unexpected error parsing write command: %v", err)
		}
	})

	if !strings.Contains(output, "w999Hello") {
		t.Fatalf("expected the write command to be logged in full: %s", output)
	}
}

// A password sent before the connection is authenticated is still a
// credential, the redaction must not depend on the message being valid.
func TestParseMessageDoesNotLogPasswordOnMalformedInput(t *testing.T) {
	// Two bytes is below minMessageSize, so the parser rejects the message.
	const password = "xy"

	output := captureLogs(t, func() {
		input := "p" + password
		if _, err := ParseMessage(len(input), []byte(input)); err == nil {
			t.Fatal("expected an error for a message shorter than minMessageSize")
		}
	})

	if strings.Contains(output, password) {
		t.Fatalf("password value leaked into the logs: %s", output)
	}
}
