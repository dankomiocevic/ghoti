package slots

import (
	"math"
	"sync"
	"testing"

	"github.com/dankomiocevic/ghoti/internal/auth"
)

func loadAtomicSlot(_ *testing.T) *atomicSlot {
	users := make(map[string]string)
	slot := &atomicSlot{
		users: users,
		value: 0,
		mu:    sync.RWMutex{},
	}
	return slot
}

func TestAtomicSlotIncrementsOnRead(t *testing.T) {
	slot := loadAtomicSlot(t)

	initialValue := slot.Read()
	if initialValue != "1" {
		t.Fatalf("Initial read should return 1, got %s", initialValue)
	}

	secondValue := slot.Read()
	if secondValue != "2" {
		t.Fatalf("Second read should return 2, got %s", secondValue)
	}
}

func TestAtomicSlotResetsAtMaxInt64(t *testing.T) {
	slot := loadAtomicSlot(t)
	slot.value = math.MaxInt64

	value := slot.Read()
	if value != "0" {
		t.Fatalf("Read at MaxInt64 should reset to 0, got %s", value)
	}
}

func TestAtomicSlotCanReadWhenUsersEmpty(t *testing.T) {
	slot := loadAtomicSlot(t)

	readUser, _ := auth.GetUser("read", "pass")
	if !slot.CanRead(&readUser) {
		t.Fatalf("We should be able to read when users map is empty")
	}
}

func TestAtomicSlotCanWriteWhenUsersEmpty(t *testing.T) {
	slot := loadAtomicSlot(t)

	writeUser, _ := auth.GetUser("write", "pass")
	if !slot.CanWrite(&writeUser) {
		t.Fatalf("We should be able to write when users map is empty")
	}
}

func TestAtomicSlotPermissions(t *testing.T) {
	users := map[string]string{
		"read_user":  "r",
		"write_user": "w",
		"all_user":   "a",
		"none_user":  "x",
	}

	slot := &atomicSlot{
		users: users,
		value: 0,
		mu:    sync.RWMutex{},
	}

	readUser, _ := auth.GetUser("read_user", "pass")
	writeUser, _ := auth.GetUser("write_user", "pass")
	allUser, _ := auth.GetUser("all_user", "pass")
	noneUser, _ := auth.GetUser("none_user", "pass")
	unknownUser, _ := auth.GetUser("unknown", "pass")

	if !slot.CanRead(&readUser) {
		t.Fatalf("Read user should have read permissions")
	}

	if slot.CanWrite(&readUser) {
		t.Fatalf("Read user should not have write permissions")
	}

	if slot.CanRead(&writeUser) {
		t.Fatalf("Write user should not have read permissions")
	}

	if !slot.CanWrite(&writeUser) {
		t.Fatalf("Write user should have write permissions")
	}

	if !slot.CanRead(&allUser) {
		t.Fatalf("All user should have read permissions")
	}

	if !slot.CanWrite(&allUser) {
		t.Fatalf("All user should have write permissions")
	}

	if slot.CanRead(&noneUser) {
		t.Fatalf("None user should not have read permissions")
	}

	if slot.CanWrite(&noneUser) {
		t.Fatalf("None user should not have write permissions")
	}

	if slot.CanRead(&unknownUser) {
		t.Fatalf("Unknown user should not have read permissions")
	}

	if slot.CanWrite(&unknownUser) {
		t.Fatalf("Unknown user should not have write permissions")
	}
}

func TestAtomicSlotWrite(t *testing.T) {
	slot := loadAtomicSlot(t)

	result, err := slot.Write("42", nil)
	if err != nil {
		t.Fatalf("Write should not return error: %v", err)
	}

	if result != "42" {
		t.Fatalf("Write should return the same value, got %s", result)
	}

	if slot.value != 42 {
		t.Fatalf("Value should be updated to 42, got %d", slot.value)
	}

	readValue := slot.Read()
	if readValue != "43" {
		t.Fatalf("Read after write should increment from 42 to 43, got %s", readValue)
	}
}

func TestAtomicSlotWriteNonInteger(t *testing.T) {
	slot := loadAtomicSlot(t)

	_, err := slot.Write("not-an-integer", nil)
	if err == nil {
		t.Fatalf("Write with non-integer should return error")
	}
}

func TestAtomicSlotWriteNegativeValue(t *testing.T) {
	slot := loadAtomicSlot(t)

	_, err := slot.Write("-10", nil)
	if err == nil {
		t.Fatalf("Write with negative value should return error")
	}
}
