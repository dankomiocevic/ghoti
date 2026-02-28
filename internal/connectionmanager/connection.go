package connectionmanager

import (
	"bufio"
	"io"
	"log/slog"
	"net"
	"strings"
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

	// Send event to the channel and return an error if the channel is full
	select {
	case c.Events <- event:
	default:
		return errs.PermanentError{Err: "Could not send event, channel full"}
	}

	// Wait for the callback to be called
	select {
	case response := <-c.Callback:
		slog.Debug("Callback received", slog.String("response", response))
		switch response {
		case eventID + " OK":
			return nil
		case eventID + " TIMEOUT":
			return errs.TranscientError{Err: "Timeout waiting for response"}
		case eventID + " ERROR":
			return errs.TranscientError{Err: "Error sending event"}
		default:
			return errs.TranscientError{Err: "Unknown response for event " + eventID + ": " + response}
		}
	case <-time.After(200 * time.Millisecond):
		return errs.TranscientError{Err: "Timeout waiting for callback"}
	}
}

func (c *Connection) EventProcessor() {
	var eventBatch []Event
	batchSize := 20
	timer := time.NewTimer(0) // Start with a zero timer
	timer.Stop()              // Stop it immediately

	for {
		select {
		case event, ok := <-c.Events:
			if !ok {
				// Channel closed, send any remaining events
				if len(eventBatch) > 0 {
					c.sendBatchedEvents(eventBatch)
				}
				return
			}

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
		}
	}
}

func (c *Connection) sendBatchedEvents(events []Event) {
	if len(events) == 0 {
		return
	}

	// Merge events into a single message
	var sb strings.Builder
	validEvents := 0
	for i, event := range events {
		if time.Now().After(event.timeout) {
			// Handle timeout events immediately
			var b strings.Builder
			b.Grow(40 + 8)
			b.WriteString(event.id)
			b.WriteString(" TIMEOUT")
			event.callback <- b.String()
			continue
		}

		if i > 0 {
			sb.WriteString("\n")
		}
		sb.Write(event.data)
		validEvents++
	}

	// If no valid events to send, return early
	if validEvents == 0 {
		return
	}

	// Send the batched data to the network
	batchedData := sb.String()
	c.NetworkConn.SetWriteDeadline(time.Now().Add(200 * time.Millisecond))
	_, err := c.NetworkConn.Write([]byte(batchedData))

	slog.Debug("Sending batched events",
		slog.String("id", c.ID),
		slog.Int("event_count", len(events)),
		slog.Int("total_size", len(batchedData)))

	// Handle each event's callback individually
	for _, event := range events {
		var b strings.Builder
		b.Grow(40 + 8)
		b.WriteString(event.id)

		if err != nil {
			b.WriteString(" ERROR")
			event.callback <- b.String()
			continue
		}

		b.WriteString(" OK")
		event.callback <- b.String()
	}
}

func (c *Connection) Close() error {
	close(c.Events)
	return c.NetworkConn.Close()
}
