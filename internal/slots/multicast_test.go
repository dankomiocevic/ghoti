package slots

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/dankomiocevic/ghoti/internal/auth"
	"github.com/dankomiocevic/ghoti/internal/connectionmanager"
)

// fakeConn is a minimal net.Conn used only as an identity in tests.
type fakeConn struct {
	net.Conn
	name string
}

func loadMulticastSlot(multicastFn func(message string, targets []net.Conn, timeout time.Duration) (connectionmanager.MulticastResult, error)) *multicastSlot {
	users := make(map[string]string)
	manager := &MockConnectionManager{
		MulticastFunc: multicastFn,
	}
	return newMulticastSlot(users, manager, "test_slot", 200*time.Millisecond, 3)
}

func TestMulticastSlotCanReadWhenUsersEmpty(t *testing.T) {
	slot := loadMulticastSlot(nil)

	readUser, _ := auth.GetUser("read", "pass")
	if !slot.CanRead(&readUser) {
		t.Fatalf("we should be able to read when users map is empty")
	}
}

func TestMulticastSlotCanWriteWhenUsersEmpty(t *testing.T) {
	slot := loadMulticastSlot(nil)

	writeUser, _ := auth.GetUser("write", "pass")
	if !slot.CanWrite(&writeUser) {
		t.Fatalf("we should be able to write when users map is empty")
	}
}

func TestMulticastSlotPermissions(t *testing.T) {
	users := map[string]string{
		"read_user":  "r",
		"write_user": "w",
		"all_user":   "a",
	}
	slot := newMulticastSlot(users, &MockConnectionManager{}, "test_slot", 200*time.Millisecond, 3)

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

func TestMulticastSlotRegisterAndDeregister(t *testing.T) {
	slot := loadMulticastSlot(nil)
	conn := &fakeConn{name: "a"}

	response, err := slot.Register(conn)
	if err != nil {
		t.Fatalf("unexpected error registering: %v", err)
	}
	if response != "1" {
		t.Fatalf("expected registration response '1', got %q", response)
	}
	if _, ok := slot.members[conn]; !ok {
		t.Fatalf("connection should be registered")
	}

	response, err = slot.Deregister(conn)
	if err != nil {
		t.Fatalf("unexpected error deregistering: %v", err)
	}
	if response != "0" {
		t.Fatalf("expected deregistration response '0', got %q", response)
	}
	if _, ok := slot.members[conn]; ok {
		t.Fatalf("connection should no longer be registered")
	}
}

func TestMulticastSlotDeregisterWhenNotRegistered(t *testing.T) {
	slot := loadMulticastSlot(nil)
	conn := &fakeConn{name: "a"}

	response, err := slot.Deregister(conn)
	if err != nil {
		t.Fatalf("unexpected error deregistering: %v", err)
	}
	if response != "0" {
		t.Fatalf("expected deregistration response '0', got %q", response)
	}
}

func TestMulticastSlotWriteWithNoMembersDoesNotCallManager(t *testing.T) {
	called := false
	slot := loadMulticastSlot(func(message string, targets []net.Conn, timeout time.Duration) (connectionmanager.MulticastResult, error) {
		called = true
		return connectionmanager.MulticastResult{}, nil
	})

	response, err := slot.Write("hello", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if response != "0/0/0" {
		t.Fatalf("expected '0/0/0', got %q", response)
	}
	if called {
		t.Fatalf("manager should not be called when there are no members")
	}
	if slot.Read() != "hello" {
		t.Fatalf("value should still be stored")
	}
}

func TestMulticastSlotWriteTargetsOnlyRegisteredMembers(t *testing.T) {
	registered := &fakeConn{name: "registered"}

	var gotTargets []net.Conn
	slot := loadMulticastSlot(func(message string, targets []net.Conn, timeout time.Duration) (connectionmanager.MulticastResult, error) {
		gotTargets = targets
		if message != "atest_slothello\n" {
			t.Fatalf("unexpected event payload: %q", message)
		}
		return connectionmanager.MulticastResult{Received: 1, Sent: 1, Errors: 0}, nil
	})

	if _, err := slot.Register(registered); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	response, err := slot.Write("hello", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if response != "1/1/0" {
		t.Fatalf("expected '1/1/0', got %q", response)
	}
	if len(gotTargets) != 1 || gotTargets[0] != registered {
		t.Fatalf("expected only the registered member as target, got %v", gotTargets)
	}
}

func TestMulticastSlotDeregistersAfterRepeatedFailures(t *testing.T) {
	failing := &fakeConn{name: "failing"}

	slot := loadMulticastSlot(func(message string, targets []net.Conn, timeout time.Duration) (connectionmanager.MulticastResult, error) {
		return connectionmanager.MulticastResult{Received: 0, Sent: 1, Errors: 1, Failed: targets}, nil
	})

	if _, err := slot.Register(failing); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for i := 0; i < 3; i++ {
		if _, ok := slot.members[failing]; !ok {
			t.Fatalf("member should still be registered before attempt %d", i+1)
		}
		if _, err := slot.Write("hello", nil); err != nil {
			t.Fatalf("unexpected error on write %d: %v", i, err)
		}
	}

	if _, ok := slot.members[failing]; ok {
		t.Fatalf("member should have been deregistered after %d failures", 3)
	}
}

func TestMulticastSlotSuccessResetsFailureCounter(t *testing.T) {
	conn := &fakeConn{name: "flaky"}
	shouldFail := true

	slot := loadMulticastSlot(func(message string, targets []net.Conn, timeout time.Duration) (connectionmanager.MulticastResult, error) {
		if shouldFail {
			return connectionmanager.MulticastResult{Sent: 1, Errors: 1, Failed: targets}, nil
		}
		return connectionmanager.MulticastResult{Sent: 1, Received: 1}, nil
	})

	if _, err := slot.Register(conn); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Two failures, then a success, then a failure again: the member should
	// still be registered because the success reset the counter.
	if _, err := slot.Write("m1", nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := slot.Write("m2", nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	shouldFail = false
	if _, err := slot.Write("m3", nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	shouldFail = true
	if _, err := slot.Write("m4", nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := slot.members[conn]; !ok {
		t.Fatalf("member should still be registered, the success should have reset the counter")
	}
}

func TestMulticastSlotWriteManagerFailure(t *testing.T) {
	conn := &fakeConn{name: "a"}
	slot := loadMulticastSlot(func(message string, targets []net.Conn, timeout time.Duration) (connectionmanager.MulticastResult, error) {
		return connectionmanager.MulticastResult{}, fmt.Errorf("multicast failed")
	})

	if _, err := slot.Register(conn); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err := slot.Write("hello", nil)
	if err == nil {
		t.Fatalf("Error should be returned when manager multicast fails")
	}
}
