package slots

import (
	"net"
	"testing"
	"time"

	"github.com/spf13/viper"

	"github.com/dankomiocevic/ghoti/internal/auth"
)

func loadTimeoutSlot(t *testing.T) Slot {
	v := viper.New()

	v.Set("kind", "timeout_memory")
	v.Set("timeout", 1)
	v.Set("users.read", "r")
	v.Set("users.write", "w")
	v.Set("users.allu", "a")

	slot, err := GetSlot(v, nil, "")
	if err != nil {
		t.Fatalf("Slot must not return error: %s", err)
	}
	return slot
}

func TestTimeoutMemory(t *testing.T) {
	slot := loadTimeoutSlot(t)

	timeoutSlot := slot.(*timeoutSlot)
	if timeoutSlot.timeout != 1*time.Second {
		t.Fatalf("timeout must be 10: %s", timeoutSlot.timeout)
	}
}

func TestTimeoutRead(t *testing.T) {
	readUser, _ := auth.GetUser("read", "pass")
	writeUser, _ := auth.GetUser("write", "pass")
	allUser, _ := auth.GetUser("allu", "pass")

	slot := loadTimeoutSlot(t)

	if !slot.CanRead(&readUser) {
		t.Fatalf("we should be able to read with the read user")
	}

	if slot.CanRead(&writeUser) {
		t.Fatalf("we should not be able to read with the write user")
	}

	if !slot.CanRead(&allUser) {
		t.Fatalf("we should be able to read with the read/write user")
	}
}

func TestTimeoutWrite(t *testing.T) {
	readUser, _ := auth.GetUser("read", "pass")
	writeUser, _ := auth.GetUser("write", "pass")
	allUser, _ := auth.GetUser("allu", "pass")

	slot := loadTimeoutSlot(t)

	if slot.CanWrite(&readUser) {
		t.Fatalf("we should not be able to write with the read user")
	}

	if !slot.CanWrite(&writeUser) {
		t.Fatalf("we should be able to write with the write user")
	}

	if !slot.CanWrite(&allUser) {
		t.Fatalf("we should be able to write with the read/write user")
	}
}

func TestTimeoutMemoryMissingConfig(t *testing.T) {
	v := viper.New()

	v.Set("kind", "timeout_memory")

	_, err := GetSlot(v, nil, "")
	if err == nil {
		t.Fatalf("Slot must return error")
	}
}

func TestMultipleWrites(t *testing.T) {
	_, clientOne := net.Pipe()
	_, clientTwo := net.Pipe()
	slot := loadTimeoutSlot(t)

	resp, err := slot.Write("Hello!", clientOne)
	if err != nil {
		t.Fatalf("error writing slot: %s", err)
	}

	if resp != "Hello!" {
		t.Fatalf("wrong value stored in slot: %s", resp)
	}

	resp = slot.Read()
	if resp != "Hello!" {
		t.Fatalf("wrong value stored in slot: %s", resp)
	}

	_, err = slot.Write("Hello!", clientTwo)
	if err == nil {
		t.Fatalf("Writing before timeout should fail")
	}

	resp, err = slot.Write("Hello Again!", clientOne)
	if err != nil {
		t.Fatalf("error writing slot: %s", err)
	}

	if resp != "Hello Again!" {
		t.Fatalf("wrong value stored in slot: %s", resp)
	}

	resp = slot.Read()
	if resp != "Hello Again!" {
		t.Fatalf("wrong value stored in slot: %s", resp)
	}

	time.Sleep(1 * time.Second)
	resp, err = slot.Write("Hello back!", clientTwo)
	if err != nil {
		t.Fatalf("error writing slot: %s", err)
	}

	if resp != "Hello back!" {
		t.Fatalf("wrong value stored in slot: %s", resp)
	}

	resp = slot.Read()
	if resp != "Hello back!" {
		t.Fatalf("wrong value stored in slot: %s", resp)
	}

	_, err = slot.Write("Hello!", clientOne)
	if err == nil {
		t.Fatalf("Writing before timeout should fail")
	}
}
