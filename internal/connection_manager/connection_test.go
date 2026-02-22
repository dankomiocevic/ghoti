package connection_manager

import (
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

// MockConnection simulates a network connection for testing.
type MockConnection struct {
	writeData []byte
	writeErr  error
	sync.Mutex
}

func (m *MockConnection) Read(b []byte) (n int, err error) {
	return 0, nil
}

func (m *MockConnection) Write(b []byte) (n int, err error) {
	m.Lock()
	defer m.Unlock()
	m.writeData = append(m.writeData, b...)
	return len(b), m.writeErr
}

func (m *MockConnection) GetWriteData() []byte {
	m.Lock()
	defer m.Unlock()
	return append([]byte(nil), m.writeData...)
}

func (m *MockConnection) Close() error {
	return nil
}

func (m *MockConnection) LocalAddr() net.Addr {
	return nil
}

func (m *MockConnection) RemoteAddr() net.Addr {
	return nil
}

func (m *MockConnection) SetDeadline(t time.Time) error {
	return nil
}

func (m *MockConnection) SetReadDeadline(t time.Time) error {
	return nil
}

func (m *MockConnection) SetWriteDeadline(t time.Time) error {
	return nil
}

func loadConnection(_ *testing.T) *Connection {
	conn := &MockConnection{}

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

// TestBatchingSingleEvent tests that a single event is sent immediately.
func TestBatchingSingleEvent(t *testing.T) {
	conn := loadConnection(t)
	go conn.EventProcessor()

	// Send a single event.
	err := conn.SendEvent("test_data")
	if err != nil {
		t.Fatalf("Error sending single event: %s", err)
	}

	// Check that the event was sent to the network.
	mockConn := conn.NetworkConn.(*MockConnection)
	if len(mockConn.GetWriteData()) == 0 {
		t.Fatal("No data was written to network")
	}

	// Verify the data was written correctly.
	expectedData := "test_data"
	if string(mockConn.GetWriteData()) != expectedData {
		t.Fatalf("Expected data '%s', got '%s'", expectedData, string(mockConn.GetWriteData()))
	}
}

// TestBatchingMultipleEvents tests that multiple events are batched together.
func TestBatchingMultipleEvents(t *testing.T) {
	conn := loadConnection(t)

	// Start the event processor
	go conn.EventProcessor()

	// Send multiple events directly to the channel
	for i := 0; i < 5; i++ {
		eventID := uuid.NewString()
		event := Event{
			id:       eventID,
			data:     []byte("test_data_" + string(rune('A'+i))),
			callback: conn.Callback,
			timeout:  time.Now().Add(200 * time.Millisecond),
		}
		conn.Events <- event
	}

	// Wait for the timer to trigger batching
	time.Sleep(50 * time.Millisecond)

	// Check that the events were batched and sent to the network
	mockConn := conn.NetworkConn.(*MockConnection)
	if len(mockConn.GetWriteData()) == 0 {
		t.Fatal("No data was written to network")
	}

	// Verify that all events were sent (may be in multiple batches due to timer)
	// Check that all expected data is present
	writtenData := string(mockConn.GetWriteData())
	expectedEvents := []string{"test_data_A", "test_data_B", "test_data_C", "test_data_D", "test_data_E"}

	for _, expectedEvent := range expectedEvents {
		if !strings.Contains(writtenData, expectedEvent) {
			t.Fatalf("Expected event '%s' not found in written data: '%s'", expectedEvent, writtenData)
		}
	}

	// Verify that events are separated by newlines (may be in multiple batches)
	if !strings.Contains(writtenData, "\n") {
		t.Fatalf("Expected newlines between events, got: '%s'", writtenData)
	}
}

// TestBatchingFullBatch tests that exactly 20 events trigger immediate sending.
func TestBatchingFullBatch(t *testing.T) {
	conn := loadConnection(t)

	// Start the event processor
	go conn.EventProcessor()

	// Send exactly 20 events (full batch) directly to the channel
	for i := range 20 {
		eventID := uuid.NewString()
		event := Event{
			id:       eventID,
			data:     []byte("test_data_" + string(rune('A'+i))),
			callback: conn.Callback,
			timeout:  time.Now().Add(200 * time.Millisecond),
		}
		conn.Events <- event
	}

	// Wait a bit for processing
	time.Sleep(10 * time.Millisecond)

	// Check that the events were batched and sent to the network
	mockConn := conn.NetworkConn.(*MockConnection)
	if len(mockConn.GetWriteData()) == 0 {
		t.Fatal("No data was written to network")
	}

	// Verify the batched data was written correctly
	// Should have 20 lines separated by newlines
	expectedLines := 20
	actualLines := strings.Count(string(mockConn.GetWriteData()), "\n") + 1
	if actualLines != expectedLines {
		t.Fatalf("Expected %d lines in batch, got %d", expectedLines, actualLines)
	}
}

// TestBatchingTimeoutHandling tests that timed-out events are handled correctly.
func TestBatchingTimeoutHandling(t *testing.T) {
	conn := loadConnection(t)

	// Start the event processor
	go conn.EventProcessor()

	// Create an event that will timeout
	eventID := uuid.NewString()
	event := Event{
		id:       eventID,
		data:     []byte("test_data"),
		callback: conn.Callback,
		timeout:  time.Now().Add(-1 * time.Millisecond), // Already timed out
	}

	// Send the timed-out event
	conn.Events <- event

	// Wait a bit for processing
	time.Sleep(10 * time.Millisecond)

	// Check that we received a timeout response
	select {
	case response := <-conn.Callback:
		if response != eventID+" TIMEOUT" {
			t.Fatalf("Expected timeout response, got '%s'", response)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("No response received for timeout event")
	}
}

// TestBatchingMixedTimeoutEvents tests handling of mixed valid and timed-out events.
func TestBatchingMixedTimeoutEvents(t *testing.T) {
	conn := loadConnection(t)

	// Start the event processor
	go conn.EventProcessor()

	// Create events with different timeouts
	validEventID := uuid.NewString()
	timeoutEventID := uuid.NewString()

	validEvent := Event{
		id:       validEventID,
		data:     []byte("valid_data"),
		callback: conn.Callback,
		timeout:  time.Now().Add(200 * time.Millisecond),
	}

	timeoutEvent := Event{
		id:       timeoutEventID,
		data:     []byte("timeout_data"),
		callback: conn.Callback,
		timeout:  time.Now().Add(-1 * time.Millisecond), // Already timed out
	}

	// Send both events
	conn.Events <- validEvent
	conn.Events <- timeoutEvent

	// Wait a bit for processing
	time.Sleep(10 * time.Millisecond)

	// Check responses
	responses := make([]string, 0)
	for i := range 2 {
		select {
		case response := <-conn.Callback:
			responses = append(responses, response)
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("Timeout waiting for response %d", i)
		}
	}

	if len(responses) != 2 {
		t.Fatalf("Expected 2 responses, got %d", len(responses))
	}

	// Should have one timeout and one OK response
	timeoutCount := 0
	okCount := 0
	for _, response := range responses {
		if strings.HasSuffix(response, " TIMEOUT") {
			timeoutCount++
		} else if strings.HasSuffix(response, " OK") {
			okCount++
		}
	}

	if timeoutCount != 1 || okCount != 1 {
		t.Fatalf("Expected 1 timeout and 1 OK response, got %d timeout and %d OK", timeoutCount, okCount)
	}
}

// TestBatchingAllTimeoutEvents tests that all timed-out events are handled correctly.
func TestBatchingAllTimeoutEvents(t *testing.T) {
	conn := loadConnection(t)

	// Start the event processor
	go conn.EventProcessor()

	for range 3 {
		eventID := uuid.NewString()
		event := Event{
			id:       eventID,
			data:     []byte("timeout_data"),
			callback: conn.Callback,
			timeout:  time.Now().Add(-1 * time.Millisecond), // Already timed out
		}
		conn.Events <- event
	}

	// Wait a bit for processing
	time.Sleep(10 * time.Millisecond)

	// Check that we received timeout responses for all events
	responses := make([]string, 0)
	for i := range 3 {
		select {
		case response := <-conn.Callback:
			if !strings.HasSuffix(response, " TIMEOUT") {
				t.Fatalf("Expected timeout response, got '%s'", response)
			}
			responses = append(responses, response)
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("Timeout waiting for response %d", i)
		}
	}

	if len(responses) != 3 {
		t.Fatalf("Expected 3 timeout responses, got %d", len(responses))
	}
}

// TestBatchingChannelEmptyTrigger tests that events are sent when channel is empty.
func TestBatchingChannelEmptyTrigger(t *testing.T) {
	conn := loadConnection(t)

	// Start the event processor
	go conn.EventProcessor()

	// Send a single event
	err := conn.SendEvent("test_data")
	if err != nil {
		t.Fatalf("Error sending event: %s", err)
	}

	// Wait a bit for processing
	time.Sleep(10 * time.Millisecond)

	// Check that the event was sent to the network
	mockConn := conn.NetworkConn.(*MockConnection)
	if len(mockConn.GetWriteData()) == 0 {
		t.Fatal("No data was written to network")
	}

	// Verify the data was written correctly
	expectedData := "test_data"
	if string(mockConn.GetWriteData()) != expectedData {
		t.Fatalf("Expected data '%s', got '%s'", expectedData, string(mockConn.GetWriteData()))
	}
}

// TestBatchingCallbackValidation tests that callbacks are sent correctly after batching.
func TestBatchingCallbackValidation(t *testing.T) {
	conn := loadConnection(t)

	// Start the event processor
	go conn.EventProcessor()

	// Send multiple events and collect their IDs
	eventIDs := make([]string, 5)
	for i := 0; i < 5; i++ {
		eventID := uuid.NewString()
		eventIDs[i] = eventID
		event := Event{
			id:       eventID,
			data:     []byte("test_data_" + string(rune('A'+i))),
			callback: conn.Callback,
			timeout:  time.Now().Add(200 * time.Millisecond),
		}
		conn.Events <- event
	}

	// Wait for the timer to trigger batching
	time.Sleep(50 * time.Millisecond)

	// Check that we received callbacks for all events
	responses := make([]string, 0)
	for i := 0; i < 5; i++ {
		select {
		case response := <-conn.Callback:
			responses = append(responses, response)
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("Timeout waiting for callback %d", i)
		}
	}

	if len(responses) != 5 {
		t.Fatalf("Expected 5 callbacks, got %d", len(responses))
	}

	// Verify all callbacks are OK responses
	for i, response := range responses {
		if !strings.HasSuffix(response, " OK") {
			t.Fatalf("Expected OK response for event %d, got '%s'", i, response)
		}
		// Verify the event ID matches
		expectedID := eventIDs[i]
		if !strings.HasPrefix(response, expectedID) {
			t.Fatalf("Expected callback for event %s, got '%s'", expectedID, response)
		}
	}
}

// TestBatchingCallbackOrder tests that callbacks are sent in the correct order.
func TestBatchingCallbackOrder(t *testing.T) {
	conn := loadConnection(t)

	// Start the event processor
	go conn.EventProcessor()

	// Send events with specific IDs to track order
	eventIDs := []string{"event1", "event2", "event3"}
	for i, eventID := range eventIDs {
		event := Event{
			id:       eventID,
			data:     []byte(fmt.Sprintf("data_%d", i+1)),
			callback: conn.Callback,
			timeout:  time.Now().Add(200 * time.Millisecond),
		}
		conn.Events <- event
	}

	// Wait for the timer to trigger batching
	time.Sleep(50 * time.Millisecond)

	// Check that callbacks are received in order
	for i, expectedID := range eventIDs {
		select {
		case response := <-conn.Callback:
			if !strings.HasPrefix(response, expectedID) {
				t.Fatalf("Expected callback for event %s at position %d, got '%s'", expectedID, i, response)
			}
			if !strings.HasSuffix(response, " OK") {
				t.Fatalf("Expected OK response for event %s, got '%s'", expectedID, response)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("Timeout waiting for callback %d", i)
		}
	}
}

// TestBatchingCallbackWithErrors tests callback behavior when network write fails.
func TestBatchingCallbackWithErrors(t *testing.T) {
	conn := loadConnection(t)

	// Set the mock connection to return an error
	mockConn := conn.NetworkConn.(*MockConnection)
	mockConn.writeErr = fmt.Errorf("network error")

	// Start the event processor
	go conn.EventProcessor()

	// Send multiple events
	eventIDs := make([]string, 3)
	for i := 0; i < 3; i++ {
		eventID := uuid.NewString()
		eventIDs[i] = eventID
		event := Event{
			id:       eventID,
			data:     []byte("test_data"),
			callback: conn.Callback,
			timeout:  time.Now().Add(200 * time.Millisecond),
		}
		conn.Events <- event
	}

	// Wait for the timer to trigger batching
	time.Sleep(50 * time.Millisecond)

	// Check that we received ERROR callbacks for all events
	for i := 0; i < 3; i++ {
		select {
		case response := <-conn.Callback:
			if !strings.HasSuffix(response, " ERROR") {
				t.Fatalf("Expected ERROR response for event %d, got '%s'", i, response)
			}
			// Verify the event ID matches
			expectedID := eventIDs[i]
			if !strings.HasPrefix(response, expectedID) {
				t.Fatalf("Expected callback for event %s, got '%s'", expectedID, response)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("Timeout waiting for callback %d", i)
		}
	}
}

// TestBatchingCallbackMixedTimeout tests callback behavior with mixed valid and timeout events.
func TestBatchingCallbackMixedTimeout(t *testing.T) {
	conn := loadConnection(t)

	// Start the event processor
	go conn.EventProcessor()

	// Create events with different timeouts
	validEventID := uuid.NewString()
	timeoutEventID := uuid.NewString()

	validEvent := Event{
		id:       validEventID,
		data:     []byte("valid_data"),
		callback: conn.Callback,
		timeout:  time.Now().Add(200 * time.Millisecond),
	}

	timeoutEvent := Event{
		id:       timeoutEventID,
		data:     []byte("timeout_data"),
		callback: conn.Callback,
		timeout:  time.Now().Add(-1 * time.Millisecond), // Already timed out
	}

	// Send both events
	conn.Events <- validEvent
	conn.Events <- timeoutEvent

	// Wait a bit for processing
	time.Sleep(10 * time.Millisecond)

	// Check responses
	responses := make([]string, 0)
	for i := 0; i < 2; i++ {
		select {
		case response := <-conn.Callback:
			responses = append(responses, response)
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("Timeout waiting for response %d", i)
		}
	}

	if len(responses) != 2 {
		t.Fatalf("Expected 2 responses, got %d", len(responses))
	}

	// Should have one timeout and one OK response
	timeoutCount := 0
	okCount := 0
	for _, response := range responses {
		if strings.HasSuffix(response, " TIMEOUT") {
			timeoutCount++
		} else if strings.HasSuffix(response, " OK") {
			okCount++
		}
	}

	if timeoutCount != 1 || okCount != 1 {
		t.Fatalf("Expected 1 timeout and 1 OK response, got %d timeout and %d OK", timeoutCount, okCount)
	}

	// Verify specific event IDs
	foundValid := false
	foundTimeout := false
	for _, response := range responses {
		if strings.HasPrefix(response, validEventID) && strings.HasSuffix(response, " OK") {
			foundValid = true
		}
		if strings.HasPrefix(response, timeoutEventID) && strings.HasSuffix(response, " TIMEOUT") {
			foundTimeout = true
		}
	}

	if !foundValid {
		t.Fatal("Did not find valid event callback")
	}
	if !foundTimeout {
		t.Fatal("Did not find timeout event callback")
	}
}
