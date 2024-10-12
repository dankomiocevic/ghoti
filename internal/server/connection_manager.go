package server

import (
	"log/slog"
	"net"
	"sync"

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

func (c *ConnectionManager) Add(conn net.Conn) Connection {
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

	connection := Connection{
		Id:          id,
		Quit:        make(chan interface{}),
		NetworkConn: conn,
		LoggedUser:  auth.User{},
		Username:    "",
		IsLogged:    false,
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
