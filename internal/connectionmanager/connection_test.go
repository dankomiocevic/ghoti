package connectionmanager

import (
	"fmt"
	"net"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/dankomiocevic/ghoti/internal/errs"
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
		ID:          uuid.NewString(),
		Quit:        make(chan interface{}),
		Events:      make(chan Event, 10),
		NetworkConn: conn,
		IsLogged:    false,
		Username:    "",
		Buffer:      make([]byte, 1024),
		Timeout:     200 * time.Millisecond,
	}
}

func TestConnectionSendResponseSuccess(t *testing.T) {
	conn := loadConnection(t)

	if err := conn.SendResponse("v000value\n"); err != nil {
		t.Fatalf("Error should not be returned for a successful write: %s", err)
	}

	mockConn := conn.NetworkConn.(*MockConnection)
	if got := string(mockConn.GetWriteData()); got != "v000value\n" {
		t.Fatalf("Expected the response to be written as is, got '%s'", got)
	}
}

// TestConnectionSendResponseDoesNotUseTheQueue verifies that responses are
// written straight to the network: no EventProcessor is running here, so a
// response that went through the Events queue would never be written.
func TestConnectionSendResponseDoesNotUseTheQueue(t *testing.T) {
	conn := NewConnection(uuid.NewString(), &MockConnection{}, 1024, 200*time.Millisecond)
	defer conn.Close()

	if err := conn.SendResponse("v000value\n"); err != nil {
		t.Fatalf("Error should not be returned for a successful write: %s", err)
	}

	if len(conn.Events) != 0 {
		t.Fatalf("The response should not be queued, found %d events", len(conn.Events))
	}

	mockConn := conn.NetworkConn.(*MockConnection)
	if got := string(mockConn.GetWriteData()); got != "v000value\n" {
		t.Fatalf("Expected the response to be written, got '%s'", got)
	}
}

func TestConnectionSendResponseWriteError(t *testing.T) {
	conn := loadConnection(t)
	conn.NetworkConn.(*MockConnection).writeErr = fmt.Errorf("network error")

	err := conn.SendResponse("v000value\n")
	if _, ok := err.(errs.TranscientError); !ok {
		t.Fatalf("A failed write should be a transient error, got: %v", err)
	}
}

func TestConnectionSendResponseClosed(t *testing.T) {
	conn := NewConnection(uuid.NewString(), &MockConnection{}, 1024, 200*time.Millisecond)
	conn.Close()

	err := conn.SendResponse("v000value\n")
	if _, ok := err.(errs.PermanentError); !ok {
		t.Fatalf("Writing on a closed connection should be a permanent error, got: %v", err)
	}

	if got := conn.NetworkConn.(*MockConnection).GetWriteData(); len(got) != 0 {
		t.Fatalf("Nothing should be written on a closed connection, got '%s'", got)
	}
}

// enqueueEvent queues an event on the connection and returns the channel its
// outcome will be reported on.
func enqueueEvent(t *testing.T, conn *Connection, data string) chan string {
	t.Helper()

	callback := make(chan string, 1)
	event := Event{
		id:       uuid.NewString(),
		data:     []byte(data),
		callback: callback,
		timeout:  time.Now().Add(200 * time.Millisecond),
	}
	if !conn.Enqueue(event) {
		t.Fatal("Event could not be queued")
	}
	return callback
}

// waitEvent waits for the outcome of a queued event and fails unless it was
// written.
func waitEvent(t *testing.T, callback chan string) {
	t.Helper()

	select {
	case response := <-callback:
		if !strings.HasSuffix(response, " OK") {
			t.Fatalf("Expected the event to be written, got '%s'", response)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Timeout waiting for the event to be written")
	}
}

// chunkConn accepts at most a few bytes per Write, the way a socket does on a
// short write, so writeFull needs several calls to send one message. That is
// the window where two unsynchronised writers would mix their bytes.
type chunkConn struct {
	MockConnection
}

func (c *chunkConn) Write(b []byte) (int, error) {
	if len(b) > 3 {
		b = b[:3]
	}
	n, err := c.MockConnection.Write(b)
	runtime.Gosched()
	return n, err
}

// TestResponsesAndEventsDoNotMix writes responses from one goroutine while
// the EventProcessor writes broadcast events to the same connection. Every
// line that reaches the network must be one whole message.
func TestResponsesAndEventsDoNotMix(t *testing.T) {
	nc := &chunkConn{}
	conn := NewConnection(uuid.NewString(), nc, 1024, 200*time.Millisecond)
	go conn.EventProcessor()
	defer conn.Close()

	const (
		response = "v000response-response-response\n"
		event    = "a006broadcast-broadcast-broadcast\n"
		count    = 200
	)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for range count {
			if err := conn.SendResponse(response); err != nil {
				t.Errorf("Error sending response: %s", err)
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		for range count {
			waitEvent(t, enqueueEvent(t, &conn, event))
		}
	}()
	wg.Wait()

	lines := strings.Split(strings.TrimSuffix(string(nc.GetWriteData()), "\n"), "\n")
	if len(lines) != 2*count {
		t.Fatalf("Expected %d lines, got %d", 2*count, len(lines))
	}
	for i, line := range lines {
		if line+"\n" != response && line+"\n" != event {
			t.Fatalf("Line %d is not a whole message: '%s'", i, line)
		}
	}
}

// TestBatchedEventsHaveNoEmptyLines verifies that events written in the same
// batch are not separated by an extra newline: every event already ends with
// its own.
func TestBatchedEventsHaveNoEmptyLines(t *testing.T) {
	conn := loadConnection(t)
	callback := make(chan string, 2)
	for _, data := range []string{"a006one\n", "a006two\n"} {
		conn.Events <- Event{
			id:       uuid.NewString(),
			data:     []byte(data),
			callback: callback,
			timeout:  time.Now().Add(200 * time.Millisecond),
		}
	}

	go conn.EventProcessor()
	for range 2 {
		select {
		case <-callback:
		case <-time.After(200 * time.Millisecond):
			t.Fatal("Timeout waiting for the events to be written")
		}
	}

	got := string(conn.NetworkConn.(*MockConnection).GetWriteData())
	if got != "a006one\na006two\n" {
		t.Fatalf("Expected two lines and no empty ones, got %q", got)
	}
}

// TestBatchingSingleEvent tests that a single event is sent immediately.
func TestBatchingSingleEvent(t *testing.T) {
	conn := loadConnection(t)
	go conn.EventProcessor()

	// Send a single event.
	waitEvent(t, enqueueEvent(t, conn, "test_data"))

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
	callback := make(chan string, 32)

	// Start the event processor
	go conn.EventProcessor()

	// Send multiple events directly to the channel
	for i := 0; i < 5; i++ {
		eventID := uuid.NewString()
		event := Event{
			id:       eventID,
			data:     []byte("test_data_" + string(rune('A'+i)) + "\n"),
			callback: callback,
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
	callback := make(chan string, 32)

	// Start the event processor
	go conn.EventProcessor()

	// Send exactly 20 events (full batch) directly to the channel
	for i := range 20 {
		eventID := uuid.NewString()
		event := Event{
			id:       eventID,
			data:     []byte("test_data_" + string(rune('A'+i)) + "\n"),
			callback: callback,
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
	// Should have 20 lines, each one ended by its newline
	expectedLines := 20
	actualLines := strings.Count(string(mockConn.GetWriteData()), "\n")
	if actualLines != expectedLines {
		t.Fatalf("Expected %d lines in batch, got %d", expectedLines, actualLines)
	}
}

// TestBatchingTimeoutHandling tests that timed-out events are handled correctly.
func TestBatchingTimeoutHandling(t *testing.T) {
	conn := loadConnection(t)
	callback := make(chan string, 32)

	// Start the event processor
	go conn.EventProcessor()

	// Create an event that will timeout
	eventID := uuid.NewString()
	event := Event{
		id:       eventID,
		data:     []byte("test_data"),
		callback: callback,
		timeout:  time.Now().Add(-1 * time.Millisecond), // Already timed out
	}

	// Send the timed-out event
	conn.Events <- event

	// Wait a bit for processing
	time.Sleep(10 * time.Millisecond)

	// Check that we received a timeout response
	select {
	case response := <-callback:
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
	callback := make(chan string, 32)

	// Start the event processor
	go conn.EventProcessor()

	// Create events with different timeouts
	validEventID := uuid.NewString()
	timeoutEventID := uuid.NewString()

	validEvent := Event{
		id:       validEventID,
		data:     []byte("valid_data"),
		callback: callback,
		timeout:  time.Now().Add(200 * time.Millisecond),
	}

	timeoutEvent := Event{
		id:       timeoutEventID,
		data:     []byte("timeout_data"),
		callback: callback,
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
		case response := <-callback:
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
	callback := make(chan string, 32)

	// Start the event processor
	go conn.EventProcessor()

	for range 3 {
		eventID := uuid.NewString()
		event := Event{
			id:       eventID,
			data:     []byte("timeout_data"),
			callback: callback,
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
		case response := <-callback:
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
	waitEvent(t, enqueueEvent(t, conn, "test_data"))

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
	callback := make(chan string, 32)

	// Start the event processor
	go conn.EventProcessor()

	// Send multiple events and collect their IDs
	eventIDs := make([]string, 5)
	for i := 0; i < 5; i++ {
		eventID := uuid.NewString()
		eventIDs[i] = eventID
		event := Event{
			id:       eventID,
			data:     []byte("test_data_" + string(rune('A'+i)) + "\n"),
			callback: callback,
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
		case response := <-callback:
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
	callback := make(chan string, 32)

	// Start the event processor
	go conn.EventProcessor()

	// Send events with specific IDs to track order
	eventIDs := []string{"event1", "event2", "event3"}
	for i, eventID := range eventIDs {
		event := Event{
			id:       eventID,
			data:     []byte(fmt.Sprintf("data_%d", i+1)),
			callback: callback,
			timeout:  time.Now().Add(200 * time.Millisecond),
		}
		conn.Events <- event
	}

	// Wait for the timer to trigger batching
	time.Sleep(50 * time.Millisecond)

	// Check that callbacks are received in order
	for i, expectedID := range eventIDs {
		select {
		case response := <-callback:
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
	callback := make(chan string, 32)

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
			callback: callback,
			timeout:  time.Now().Add(200 * time.Millisecond),
		}
		conn.Events <- event
	}

	// Wait for the timer to trigger batching
	time.Sleep(50 * time.Millisecond)

	// Check that we received ERROR callbacks for all events
	for i := 0; i < 3; i++ {
		select {
		case response := <-callback:
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
	callback := make(chan string, 32)

	// Start the event processor
	go conn.EventProcessor()

	// Create events with different timeouts
	validEventID := uuid.NewString()
	timeoutEventID := uuid.NewString()

	validEvent := Event{
		id:       validEventID,
		data:     []byte("valid_data"),
		callback: callback,
		timeout:  time.Now().Add(200 * time.Millisecond),
	}

	timeoutEvent := Event{
		id:       timeoutEventID,
		data:     []byte("timeout_data"),
		callback: callback,
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
		case response := <-callback:
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
