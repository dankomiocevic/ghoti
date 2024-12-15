package server

import (
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/dankomiocevic/ghoti/internal/auth"
)

type ConnectionManager struct {
	lock        sync.RWMutex
	connections map[string]Connection
}

func NewManager() *ConnectionManager {
	return &ConnectionManager{
		lock:        sync.RWMutex{},
		connections: make(map[string]Connection),
	}
}

func (c *ConnectionManager) Add(conn net.Conn, telnetSupport bool) Connection {
	c.lock.Lock()
	defer c.lock.Unlock()

	var id string
	for {
		id = uuid.New().String()
		_, ok := c.connections[id]
		if !ok {
			break
		}
	}

	//TODO: Add this to config
	timeoutDuration := 200 * time.Millisecond

	bufferSize := 41
	if telnetSupport {
		bufferSize = bufferSize + 2
	}

	buf := make([]byte, bufferSize)
	connection := Connection{
		Id:          id,
		Quit:        make(chan interface{}),
		Events:      make(chan Event, 128),
		NetworkConn: conn,
		LoggedUser:  auth.User{},
		Username:    "",
		IsLogged:    false,
		Callback:    make(chan string),
		Buffer:      buf,
		Timeout:     timeoutDuration,
	}

	c.connections[connection.Id] = connection
	return connection
}

func (c *ConnectionManager) DeleteAll() []Connection {
	c.lock.Lock()
	defer c.lock.Unlock()

	output := make([]Connection, 0, len(c.connections))
	for k, v := range c.connections {
		slog.Debug("Removing connection",
			slog.String("id", v.Id),
			slog.String("remote_addr", v.NetworkConn.RemoteAddr().String()),
		)

		output = append(output, v)
		delete(c.connections, k)
	}

	return output
}

func (c *ConnectionManager) Delete(id string) {
	c.lock.Lock()
	defer c.lock.Unlock()

	slog.Debug("Removing connection",
		slog.String("id", id),
	)

	_, ok := c.connections[id]
	if !ok {
		slog.Debug("Connection already deleted",
			slog.String("id", id),
		)

		return
	}

	delete(c.connections, id)
}
