package connection_manager

import (
	"bufio"
	"io"
	"log/slog"
	"net"
	"strings"
	"time"

	"github.com/dankomiocevic/ghoti/internal/auth"
	"github.com/dankomiocevic/ghoti/internal/errors"

	"github.com/google/uuid"
)

type Event struct {
	id       string
	data     []byte
	timeout  time.Time
	callback chan string
}

type Connection struct {
	Id          string
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
			return 0, errors.TranscientError{Err: "Timeout receiving data"}
		}

		if err == io.EOF {
			return 0, errors.PermanentError{Err: "Connection closed"}
		}

		slog.Error("Error receiving data from connection", slog.Any("error", err))
		slog.Debug("Disconnecting",
			slog.String("id", c.Id),
			slog.String("remote_addr", c.NetworkConn.RemoteAddr().String()),
		)
		return 0, errors.PermanentError{Err: "Connection closed"}
	}

	return size, nil
}

func (c *Connection) SendEvent(data string) error {
	eventId := uuid.NewString()
	event := Event{
		id:       eventId,
		data:     []byte(data),
		callback: c.Callback,
		timeout:  time.Now().Add(200 * time.Millisecond),
	}

	slog.Debug("Sending event",
		slog.String("id", c.Id),
		slog.Any("event", event))

	// Send event to the channel and return an error if the channel is full
	select {
	case c.Events <- event:
	default:
		return errors.PermanentError{Err: "Could not send event, channel full"}
	}

	// Wait for the callback to be called
	select {
	case response := <-c.Callback:
		slog.Debug("Callback received", slog.String("response", response))
		switch response {
		case eventId + " OK":
			return nil
		case eventId + " TIMEOUT":
			return errors.TranscientError{Err: "Timeout waiting for response"}
		case eventId + " ERROR":
			return errors.TranscientError{Err: "Error sending event"}
		default:
			return errors.TranscientError{Err: "Unknown response for event " + eventId + ": " + response}
		}
	case <-time.After(200 * time.Millisecond):
		return errors.TranscientError{Err: "Timeout waiting for callback"}
	}
}

func (c *Connection) EventProcessor() {
	var b strings.Builder
	for event := range c.Events {
		b.Reset()
		b.Grow(40 + 8)
		b.WriteString(event.id)

		if time.Now().After(event.timeout) {
			b.WriteString(" TIMEOUT")
			event.callback <- b.String()
			continue
		}

		c.NetworkConn.SetWriteDeadline(event.timeout)
		_, err := c.NetworkConn.Write(event.data)
		if err != nil { //TODO: Improve error handling to differentiate error types
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
