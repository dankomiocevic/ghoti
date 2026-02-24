package slots

import (
	"fmt"
	"testing"

	"github.com/dankomiocevic/ghoti/internal/auth"
	"github.com/dankomiocevic/ghoti/internal/connection_manager"
)

type MockConnectionManager struct {
	BroadcastFunc func(message string) (string, error)
}

func (m *MockConnectionManager) Broadcast(message string) (string, error) {
	return m.BroadcastFunc(message)
}

func (m *MockConnectionManager) StartListening(string) error {
	return nil
}

func (m *MockConnectionManager) ServeConnections(connection_manager.CallbackFn) error {
	return nil
}

func (m *MockConnectionManager) Delete(string) {
}

func (m *MockConnectionManager) GetAddr() string {
	return ""
}

func (m *MockConnectionManager) Close() {
}

func loadBroadcastSlot(t *testing.T) *broadcastSlot {
	users := make(map[string]string)
	manager := &MockConnectionManager{
		BroadcastFunc: func(message string) (string, error) {
			return "mock response", nil
		},
	}
	slot, err := newBroadcastSlot(users, manager, "test_slot")
	if err != nil {
		t.Fatalf("Slot must not return error: %s", err)
	}
	return slot
}

func TestBroadcastSlotCanReadWhenUsersEmpty(t *testing.T) {
	slot := loadBroadcastSlot(t)

	readUser, _ := auth.GetUser("read", "pass")
	if !slot.CanRead(&readUser) {
		t.Fatalf("we should be able to read when users map is empty")
	}
}

func TestBroadcastSlotCanWriteWhenUsersEmpty(t *testing.T) {
	slot := loadBroadcastSlot(t)

	writeUser, _ := auth.GetUser("write", "pass")
	if !slot.CanWrite(&writeUser) {
		t.Fatalf("we should be able to write when users map is empty")
	}
}

func TestBroadcastSlotReadAndWrite(t *testing.T) {
	slot := loadBroadcastSlot(t)

	if slot.Read() != "" {
		t.Fatalf("Initial value should be empty")
	}

	slot.Write("test_value", nil)
	if slot.Read() != "test_value" {
		t.Fatalf("Value should be 'test_value'")
	}
}

func TestBroadcastSlotPermissionsWithMock(t *testing.T) {
	users := map[string]string{
		"read_user":  "r",
		"write_user": "w",
		"all_user":   "a",
	}
	manager := &MockConnectionManager{
		BroadcastFunc: func(message string) (string, error) {
			return "mock response", nil
		},
	}
	slot, err := newBroadcastSlot(users, manager, "test_slot")
	if err != nil {
		t.Fatalf("Slot must not return error: %s", err)
	}

	readUser, _ := auth.GetUser("read_user", "pass")
	writeUser, _ := auth.GetUser("write_user", "pass")
	allUser, _ := auth.GetUser("all_user", "pass")

	if !slot.CanRead(&readUser) {
		t.Fatalf("Read user should have read permissions")
	}

	if slot.CanWrite(&readUser) {
		t.Fatalf("Read user should not have write permissions")
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
}

func TestBroadcastSlotWriteManagerFailure(t *testing.T) {
	manager := &MockConnectionManager{
		BroadcastFunc: func(message string) (string, error) {
			return "", fmt.Errorf("broadcast failed")
		},
	}
	slot, err := newBroadcastSlot(make(map[string]string), manager, "test_slot")
	if err != nil {
		t.Fatalf("Slot must not return error: %s", err)
	}

	_, err = slot.Write("test_value", nil)
	if err == nil {
		t.Fatalf("Error should be returned when manager broadcast fails")
	}
}
