package connection_manager

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
)

func loadConnection(t *testing.T) *Connection {
	conn := &TestConn{}
	conn.On("Write", mock.Anything).Return(10, nil)
	conn.On("SetWriteDeadline", mock.Anything).Return(nil)
	conn.On("Close").Return(nil)

	return &Connection{
		Id:          uuid.NewString(),
		Quit:        make(chan interface{}),
		Events:      make(chan Event, 10),
		NetworkConn: conn,
		IsLogged:    false,
		Username:    "",
		Callback:    make(chan string, 10),
		Buffer:      make([]byte, 1024),
		Timeout:     200 * time.Millisecond,
	}
}

func TestConnectionSendEventSuccess(t *testing.T) {
	conn := loadConnection(t)

	go func() {
		event := <-conn.Events
		conn.Callback <- event.id + " OK"
	}()

	err := conn.SendEvent("test_data")
	if err != nil {
		t.Fatalf("Error should not be returned for successful event: %s", err)
	}
}

func TestConnectionSendEventTimeout(t *testing.T) {
	conn := loadConnection(t)

	go func() {
		event := <-conn.Events
		conn.Callback <- event.id + " TIMEOUT"
	}()

	err := conn.SendEvent("test_data")
	if err == nil || err.Error() != "Timeout waiting for response" {
		t.Fatalf("Timeout error should be returned for event: %s", err)
	}
}

func TestConnectionSendEventError(t *testing.T) {
	conn := loadConnection(t)

	go func() {
		event := <-conn.Events
		conn.Callback <- event.id + " ERROR"
	}()

	err := conn.SendEvent("test_data")
	if err == nil || err.Error() != "Error sending event" {
		t.Fatalf("Error should be returned for event: %s", err)
	}
}

func TestConnectionSendEventUnknownResponse(t *testing.T) {
	conn := loadConnection(t)

	var event Event
	go func() {
		event = <-conn.Events
		conn.Callback <- event.id + " UNKNOWN"
	}()

	err := conn.SendEvent("test_data")
	if err == nil || err.Error() != "Unknown response for event "+event.id+": "+event.id+" UNKNOWN" {
		t.Fatalf("Unknown response error should be returned for event: %s", err)
	}
}

func TestConnectionSendEventChannelFull(t *testing.T) {
	conn := &Connection{
		Events:   make(chan Event, 0),
		Callback: make(chan string, 1),
	}

	err := conn.SendEvent("test_data")
	if err == nil || err.Error() != "Could not send event, channel full" {
		t.Fatalf("Channel full error should be returned for event: %s", err)
	}
}
