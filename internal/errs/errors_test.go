package errs

import (
	"testing"
)

func TestError(t *testing.T) {
	e := Error("NOT_LEADER")

	if e.id != "000" {
		t.Fatalf("Error ID was not 000: %s", e.id)
	}

	if e.name != "NOT_LEADER" {
		t.Fatalf("Error name was not NOT_LEADER: %s", e.name)
	}

	if e.response != "000\n" {
		t.Fatalf("Error response was not e000: %s", e.response)
	}
}

func TestResponseWithArgument(t *testing.T) {
	e := Error("NOT_LEADER")

	// The argument has to go before the newline, otherwise a client reading
	// line by line never receives it.
	if got := e.ResponseWithArgument("xxx", "nodeA"); got != "exxx000nodeA\n" {
		t.Fatalf("ResponseWithArgument returned %q, want %q", got, "exxx000nodeA\n")
	}
}
