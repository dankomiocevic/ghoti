package errors

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
