package server

import (
	"bufio"
	"io"
	"log/slog"
	"net"
	"time"

	"github.com/dankomiocevic/ghoti/internal/auth"
	"github.com/dankomiocevic/ghoti/internal/errors"

	"github.com/google/uuid"
)

type Event struct {
	id       string
	data     string
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

func (c *Connection) ReadMessage(telnetSupport bool) (Message, error) {
	reader := bufio.NewReader(c.NetworkConn)
	// Set the connection timeout in the future
	c.NetworkConn.SetReadDeadline(time.Now().Add(c.Timeout))
	size, err := reader.Read(c.Buffer)
	if err != nil {
		// If the error was a timeout, continue receiving data in
		// next loop
		if opErr, ok := err.(*net.OpError); ok && opErr.Timeout() {
			return Message{}, errors.TranscientError{Err: "Timeout receiving data"}
		}

		if err == io.EOF {
			return Message{}, errors.PermanentError{Err: "Connection closed"}
		}

		slog.Error("Error receiving data from connection", slog.Any("error", err))
		slog.Debug("Disconnecting",
			slog.String("id", c.Id),
			slog.String("remote_addr", c.NetworkConn.RemoteAddr().String()),
		)
		return Message{}, errors.PermanentError{Err: "Connection closed"}
	}

	msg, err := ParseMessage(size, c.Buffer, telnetSupport)
	if err != nil {
		res := errors.Error("WRONG_FORMAT")
		slog.Debug(
			"Wrong message format received",
			slog.String("id", c.Id),
			slog.String("remote_addr", c.NetworkConn.RemoteAddr().String()),
		)

		c.NetworkConn.Write([]byte(res.Response()))
		return Message{}, errors.TranscientError{Err: "Wrong message format"}
	} else {
		slog.Debug("Received message",
			slog.String("msg", msg.Raw),
			slog.String("id", c.Id),
			slog.String("remote_addr", c.NetworkConn.RemoteAddr().String()),
		)
	}
	return msg, nil
}

func (c *Connection) SendEvent(data string) error {
	eventId := uuid.NewString()
	event := Event{
		id:       eventId,
		data:     data,
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
		if response == eventId+" OK" {
			return nil
		} else if response == eventId+" TIMEOUT" {
			return errors.TranscientError{Err: "Timeout waiting for response"}
		} else if response == eventId+" ERROR" {
			return errors.TranscientError{Err: "Error sending event"}
		} else {
			return errors.TranscientError{Err: "Unknown response for event " + eventId + ": " + response}
		}
	case <-time.After(200 * time.Millisecond):
		return errors.TranscientError{Err: "Timeout waiting for callback"}
	}
}

func (c *Connection) EventProcessor() {
	for event := range c.Events {
		if time.Now().After(event.timeout) {
			event.callback <- event.id + " TIMEOUT"
			continue
		}

		c.NetworkConn.SetWriteDeadline(event.timeout)
		_, err := c.NetworkConn.Write([]byte(event.data))
		if err != nil { //TODO: Improve error handling to differentiate error types
			event.callback <- event.id + " ERROR"
			continue
		}

		event.callback <- event.id + " OK"
	}
}

func (c *Connection) Close() error {
	close(c.Events)
	return c.NetworkConn.Close()
}
