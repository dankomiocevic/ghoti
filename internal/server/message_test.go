package server

import (
	"strings"
	"testing"
)

func TestParseMessageValidRead(t *testing.T) {
	msg, err := ParseMessage(4, []byte("r000"))
	if err != nil {
		t.Fatalf("unexpected error parsing valid read: %v", err)
	}
	if msg.Command != 'r' || msg.Slot != 0 {
		t.Fatalf("unexpected message: %+v", msg)
	}
}

func TestParseMessageValidWrite(t *testing.T) {
	msg, err := ParseMessage(9, []byte("w999Hello"))
	if err != nil {
		t.Fatalf("unexpected error parsing valid write: %v", err)
	}
	if msg.Command != 'w' || msg.Slot != 999 || msg.Value != "Hello" {
		t.Fatalf("unexpected message: %+v", msg)
	}
}

func TestParseMessageRejectsNegativeSlot(t *testing.T) {
	if _, err := ParseMessage(4, []byte("r-01")); err == nil {
		t.Fatal("expected an error for a negative slot number")
	}
}

func TestParseMessageRejectsSignedSlot(t *testing.T) {
	if _, err := ParseMessage(4, []byte("r+01")); err == nil {
		t.Fatal("expected an error for a slot number with a sign")
	}
}

func TestParseMessageRejectsNonDigitSlot(t *testing.T) {
	for _, input := range []string{"r0a0", "r00\x00", "s0 1", "d١٢٣"} {
		if _, err := ParseMessage(len(input), []byte(input)); err == nil {
			t.Fatalf("expected an error for non digit slot in %q", input)
		}
	}
}

func TestParseMessageRejectsTrailingBytesOnReadCommands(t *testing.T) {
	for _, input := range []string{"r000extra", "s000extra", "d000extra"} {
		if _, err := ParseMessage(len(input), []byte(input)); err == nil {
			t.Fatalf("expected an error for trailing bytes in %q", input)
		}
	}
}

func TestParseMessageRejectsTrailingBytesOnQuit(t *testing.T) {
	if _, err := ParseMessage(5, []byte("qjunk")); err == nil {
		t.Fatal("expected an error for trailing bytes after the quit command")
	}
}

func TestParseMessageAcceptsQuit(t *testing.T) {
	msg, err := ParseMessage(1, []byte("q"))
	if err != nil {
		t.Fatalf("unexpected error parsing quit: %v", err)
	}
	if msg.Command != 'q' {
		t.Fatalf("unexpected message: %+v", msg)
	}
}

func TestParseMessageRejectsSizeBeyondBuffer(t *testing.T) {
	if _, err := ParseMessage(64, []byte("r000")); err == nil {
		t.Fatal("expected an error when the size is bigger than the buffer")
	}
}

func TestParseMessageRejectsNegativeSize(t *testing.T) {
	if _, err := ParseMessage(-1, []byte("r000")); err == nil {
		t.Fatal("expected an error for a negative size")
	}
}

// TestParseMessageSlotAlwaysInRange checks the invariant HandleMessage relies on
// before indexing the slots array: a message parsed without error never carries
// a slot outside [0, 1000).
func TestParseMessageSlotAlwaysInRange(t *testing.T) {
	for _, input := range []string{"r-01", "r+01", "r999", "w000v", "q", "uadmin", "ppassw0rd"} {
		msg, err := ParseMessage(len(input), []byte(input))
		if err != nil {
			continue
		}
		if msg.Slot < 0 || msg.Slot >= 1000 {
			t.Fatalf("slot %d out of range for input %q", msg.Slot, input)
		}
	}
}

func FuzzParseMessage(f *testing.F) {
	seeds := []string{
		"",
		"\n",
		"q",
		"r000",
		"r-01",
		"r+01",
		"r999",
		"w000Hello",
		"w999" + strings.Repeat("a", 36),
		strings.Repeat("a", 128),
		"uadmin",
		"ppassw0rd",
		"r00\xff",
		"\xff\xfe\xfd",
		"r000\nr001",
		"r0",
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, input string) {
		msg, err := ParseMessage(len(input), []byte(input))
		if err != nil {
			return
		}
		// A successfully parsed message must be safe to use as an index into
		// the 1000 element slots array.
		if msg.Slot < 0 || msg.Slot >= 1000 {
			t.Fatalf("slot %d out of range for input %q", msg.Slot, input)
		}
	})
}
