package connectionmanager

import (
	"bufio"
	"bytes"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/dankomiocevic/ghoti/internal/auth"
	"github.com/dankomiocevic/ghoti/internal/errs"
)

type Event struct {
	id       string
	data     []byte
	timeout  time.Time
	callback chan string
}

// connState holds the parts of a connection that must be shared between every
// copy of it. Connection values are copied around (the managers keep them in a
// map by value and hand copies to broadcasters), so the close lifecycle cannot
// live in the struct itself.
//
// A nil state means "never closed": connections built directly as literals,
// like the ones in the tests, keep working without a shutdown signal.
type connState struct {
	once sync.Once
	done chan struct{}
}

type Connection struct {
	ID          string
	Quit        chan interface{}
	Events      chan Event
	NetworkConn net.Conn
	LoggedUser  auth.User
	IsLogged    bool
	Username    string
	Callback    chan string
	Buffer      []byte
	Timeout     time.Duration

	state *connState
}

// NewConnection builds a connection ready to be served by an EventProcessor.
//
// The Events channel is never closed: the processor goroutine owns the write
// side of the network connection and drains whatever is queued when the
// connection is closed. Producers enqueue with Enqueue, so a broadcast that is
// holding an old copy of a connection can never send on a closed channel.
func NewConnection(id string, conn net.Conn, bufferSize int, timeout time.Duration) Connection {
	return Connection{
		ID:          id,
		Quit:        make(chan interface{}),
		Events:      make(chan Event, 128),
		NetworkConn: conn,
		LoggedUser:  auth.User{},
		Username:    "",
		IsLogged:    false,
		// Buffered so a callback that arrives just before SendEvent parks on
		// the channel is not lost, and late ones never block the processor.
		Callback: make(chan string, 8),
		Buffer:   make([]byte, bufferSize),
		Timeout:  timeout,
		state:    &connState{done: make(chan struct{})},
	}
}

// Done returns a channel that is closed when the connection is closed. It is
// nil for connections that were not built with NewConnection, which in a
// select behaves as a channel that is never ready.
func (c *Connection) Done() <-chan struct{} {
	if c.state == nil {
		return nil
	}
	return c.state.done
}

// IsClosed reports whether Close has already been called on this connection.
func (c *Connection) IsClosed() bool {
	select {
	case <-c.Done():
		return true
	default:
		return false
	}
}

// Enqueue hands an event to the connection's processor goroutine. It reports
// whether the event was queued: a closed connection or a full queue is a
// failed delivery attempt, never a block and never a panic.
func (c *Connection) Enqueue(event Event) bool {
	if c.IsClosed() {
		return false
	}

	select {
	case c.Events <- event:
		return true
	default:
		return false
	}
}

func (c *Connection) ReceiveMessage() (int, error) {
	reader := bufio.NewReader(c.NetworkConn)
	// Set the connection timeout in the future
	c.NetworkConn.SetReadDeadline(time.Now().Add(c.Timeout))
	size, err := reader.Read(c.Buffer)

	if err != nil {
		// If the error was a timeout, continue receiving data in
		// next loop
		if opErr, ok := err.(*net.OpError); ok && opErr.Timeout() {
			return 0, errs.TranscientError{Err: "Timeout receiving data"}
		}

		if err == io.EOF {
			return 0, errs.PermanentError{Err: "Connection closed"}
		}

		slog.Error("Error receiving data from connection", slog.Any("error", err))
		slog.Debug("Disconnecting",
			slog.String("id", c.ID),
			slog.String("remote_addr", c.NetworkConn.RemoteAddr().String()),
		)
		return 0, errs.PermanentError{Err: "Connection closed"}
	}

	return size, nil
}

func (c *Connection) SendEvent(data string) error {
	eventID := uuid.NewString()
	event := Event{
		id:       eventID,
		data:     []byte(data),
		callback: c.Callback,
		timeout:  time.Now().Add(200 * time.Millisecond),
	}

	slog.Debug("Sending event",
		slog.String("id", c.ID),
		slog.Any("event", event))

	if !c.Enqueue(event) {
		return errs.PermanentError{Err: "Could not send event, channel full"}
	}

	// Wait for the callback to be called. Responses belonging to an event we
	// already gave up on are dropped instead of being reported as the answer
	// to this one.
	deadline := time.After(200 * time.Millisecond)
	for {
		select {
		case response := <-c.Callback:
			slog.Debug("Callback received", slog.String("response", response))

			id, status, _ := strings.Cut(response, " ")
			if id != eventID {
				continue
			}

			switch status {
			case "OK":
				return nil
			case "TIMEOUT":
				return errs.TranscientError{Err: "Timeout waiting for response"}
			case "ERROR":
				return errs.TranscientError{Err: "Error sending event"}
			default:
				return errs.TranscientError{Err: "Unknown response for event " + eventID + ": " + response}
			}
		case <-deadline:
			return errs.TranscientError{Err: "Timeout waiting for callback"}
		}
	}
}

// EventProcessor owns the write side of the connection: it is the only
// goroutine that writes to the network connection and the only one that
// consumes the Events queue. It returns once the connection is closed, after
// failing every event that is still queued so producers do not have to wait
// for their deadline.
func (c *Connection) EventProcessor() {
	var eventBatch []Event
	batchSize := 20
	timer := time.NewTimer(0) // Start with a zero timer
	timer.Stop()              // Stop it immediately
	defer timer.Stop()

	done := c.Done()

	for {
		select {
		case event := <-c.Events:
			eventBatch = append(eventBatch, event)

			// If we have enough events, send immediately
			if len(eventBatch) >= batchSize {
				c.sendBatchedEvents(eventBatch)
				eventBatch = eventBatch[:0] // Clear the batch
				timer.Stop()                // Stop the timer since we sent immediately
			} else {
				// Check if the channel is empty - if so, send immediately
				if len(eventBatch) == 1 && len(c.Events) == 0 {
					c.sendBatchedEvents(eventBatch)
					eventBatch = eventBatch[:0] // Clear the batch
					timer.Stop()                // Stop the timer since we sent immediately
				} else {
					// Start a timer to send events if no more come in
					timer.Reset(5 * time.Millisecond)
				}
			}

		case <-timer.C:
			// Timer expired, send any batched events
			if len(eventBatch) > 0 {
				c.sendBatchedEvents(eventBatch)
				eventBatch = eventBatch[:0] // Clear the batch
			}

		case <-done:
			c.drain(eventBatch)
			return
		}
	}
}

// drain answers every event that will never reach the network because the
// connection is gone.
func (c *Connection) drain(pending []Event) {
	for _, event := range pending {
		respond(event, "ERROR")
	}

	for {
		select {
		case event := <-c.Events:
			respond(event, "ERROR")
		default:
			return
		}
	}
}

func (c *Connection) sendBatchedEvents(events []Event) {
	if len(events) == 0 {
		return
	}

	// Merge the events that are still in time into a single message. The ones
	// that already expired are answered here and are not written out.
	//
	// The batch is filtered in place: the events that are kept are written back
	// over the ones that were dropped, so the write index never runs ahead of
	// the read index and no second slice is needed.
	var buf bytes.Buffer
	valid := events[:0]
	for _, event := range events {
		if time.Now().After(event.timeout) {
			respond(event, "TIMEOUT")
			continue
		}

		if len(valid) > 0 {
			buf.WriteString("\n")
		}
		buf.Write(event.data)
		valid = append(valid, event)
	}

	// If no valid events to send, return early
	if len(valid) == 0 {
		return
	}

	// Send the batched data to the network
	c.NetworkConn.SetWriteDeadline(time.Now().Add(200 * time.Millisecond))
	err := writeFull(c.NetworkConn, buf.Bytes())

	slog.Debug("Sending batched events",
		slog.String("id", c.ID),
		slog.Int("event_count", len(events)),
		slog.Int("total_size", buf.Len()))

	// Handle each event's callback individually
	status := "OK"
	if err != nil {
		status = "ERROR"
	}

	for _, event := range valid {
		respond(event, status)
	}
}

// writeFull writes the whole buffer, retrying while the connection keeps
// making progress. Then, net.Conn.Write can report a partial write, on a deadline
// for example, so the returned count cannot be ignored: a short write is a
// truncated event, not a delivered one.
func writeFull(conn net.Conn, data []byte) error {
	for written := 0; written < len(data); {
		n, err := conn.Write(data[written:])
		if n > 0 {
			written += n
		}

		if err != nil {
			return err
		}

		if n <= 0 {
			return io.ErrShortWrite
		}
	}

	return nil
}

// respond delivers the outcome of an event to whoever asked for it without
// ever blocking. Callback channels are sized to hold every response their
// owner asked for, so a full channel means the owner already gave up on the
// event and the processor must not wedge on it.
func respond(event Event, status string) {
	var b strings.Builder
	b.Grow(len(event.id) + len(status) + 1)
	b.WriteString(event.id)
	b.WriteString(" ")
	b.WriteString(status)

	select {
	case event.callback <- b.String():
	default:
	}
}

// Close signals the processor goroutine to stop and closes the network
// connection. It is safe to call more than once and never closes the Events
// channel, so producers holding a copy of the connection cannot panic.
func (c *Connection) Close() error {
	if c.state != nil {
		c.state.once.Do(func() { close(c.state.done) })
	}

	return c.NetworkConn.Close()
}
