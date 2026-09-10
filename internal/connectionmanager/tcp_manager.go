package connectionmanager

import (
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/dankomiocevic/ghoti/internal/errs"
	"github.com/dankomiocevic/ghoti/internal/telemetry"
)

type TCPManager struct {
	lock        sync.RWMutex
	connections map[string]Connection
	listener    net.Listener
	wg          sync.WaitGroup
	quit        chan interface{}
}

func NewTCPManager() *TCPManager {
	return &TCPManager{
		quit:        make(chan interface{}),
		lock:        sync.RWMutex{},
		connections: make(map[string]Connection),
	}
}

func (c *TCPManager) GetAddr() string {
	return c.listener.Addr().String()
}

func (c *TCPManager) StartListening(tcpAddr string) error {
	l, err := net.Listen("tcp", tcpAddr)
	if err != nil {
		return err
	}

	c.listener = l
	return nil
}

func (c *TCPManager) ServeConnections(callback CallbackFn) error {
	for {
		conn, err := c.listener.Accept()
		if err != nil {
			select {
			case <-c.quit:
				slog.Debug("Stop serving connections")
				return nil
			default:
				slog.Error("Error accepting connection", slog.Any("error", err))
			}
		} else {
			connection := c.Add(conn, 41)
			slog.Debug("Connection received",
				slog.String("id", connection.ID),
				slog.String("remote_addr", conn.RemoteAddr().String()),
			)

			c.wg.Add(1)
			go func() {
				c.handleUserConnection(callback, connection)
				c.wg.Done()
			}()
		}
	}
}

func (c *TCPManager) handleUserConnection(callback CallbackFn, conn Connection) {
	defer c.Delete(conn.ID)
	defer conn.Close()
	slog.Debug("Handling user connection",
		slog.String("remote_addr", conn.ID),
		slog.String("remote_addr", conn.NetworkConn.RemoteAddr().String()),
	)

	go conn.EventProcessor()
	for {
		select {
		case <-conn.Quit:
			slog.Debug("Connection quit",
				slog.String("remote_addr", conn.ID),
				slog.String("remote_addr", conn.NetworkConn.RemoteAddr().String()),
			)
			return
		default:
		}

		size, err := conn.ReceiveMessage()
		if err != nil {
			slog.Debug(err.Error(),
				slog.String("remote_addr", conn.ID),
				slog.String("remote_addr", conn.NetworkConn.RemoteAddr().String()),
			)
			switch err.(type) {
			case errs.TranscientError:
				continue
			case errs.PermanentError:
				return
			default:
				slog.Error("Unidentified error reading message",
					slog.String("id", conn.ID),
					slog.Any("error", err))
				return
			}
		}

		if conn.Buffer[size-1] != 10 {
			res := errs.Error("PARSE_ERROR")
			slog.Debug("Message not terminated with newline",
				slog.String("remote_addr", conn.ID),
				slog.String("remote_addr", conn.NetworkConn.RemoteAddr().String()),
			)
			conn.SendEvent(res.Response("xxx"))
			continue
		}
		size--

		err = callback(size, conn.Buffer, &conn)
		if err != nil {
			switch err.(type) {
			case errs.TranscientError:
				slog.Error(err.Error(),
					slog.String("id", conn.ID))
				continue
			case errs.PermanentError:
				slog.Error(err.Error(),
					slog.String("id", conn.ID))
				return
			default:
				slog.Error("Unidentified error type",
					slog.String("id", conn.ID),
					slog.Any("error", err))
			}
		}
	}
}

func (c *TCPManager) Add(conn net.Conn, bufferSize int) Connection {
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

	connection := NewConnection(id, conn, bufferSize, timeoutDuration)

	c.connections[connection.ID] = connection
	telemetry.IncrConnectedClients()
	return connection
}

func (c *TCPManager) Delete(id string) {
	c.lock.Lock()
	defer c.lock.Unlock()

	_, ok := c.connections[id]
	if !ok {
		slog.Debug("Connection already deleted",
			slog.String("id", id),
		)

		return
	}

	delete(c.connections, id)
	telemetry.DecrConnectedClients()
}

func (c *TCPManager) Close() {
	close(c.quit)

	slog.Debug("Closing listener")
	c.listener.Close()

	c.lock.Lock()
	conns := make([]Connection, 0, len(c.connections))
	for _, conn := range c.connections {
		conns = append(conns, conn)
	}
	c.lock.Unlock()

	for _, conn := range conns {
		slog.Debug("Closing connection",
			slog.String("id", conn.ID),
			slog.String("remote_addr", conn.NetworkConn.RemoteAddr().String()),
		)

		close(conn.Quit)
	}

	slog.Info("Waiting for connections to be drained")
	c.wg.Wait()
}

func (c *TCPManager) Multicast(data string, targets []net.Conn, timeout time.Duration) (MulticastResult, error) {
	c.lock.RLock()
	connections := make([]Connection, 0, len(c.connections))
	for _, conn := range c.connections {
		connections = append(connections, conn)
	}
	c.lock.RUnlock()

	return multicastToConnections(connections, targets, data, timeout), nil
}

func (c *TCPManager) Broadcast(data string) (string, error) {
	// Take a snapshot of the connections under the lock: iterating the map
	// directly races with the connections being added and removed as clients
	// come and go.
	c.lock.RLock()
	connections := make([]Connection, 0, len(c.connections))
	for _, conn := range c.connections {
		connections = append(connections, conn)
	}
	c.lock.RUnlock()

	// The callback channel has room for every response and is never closed:
	// the event processors may answer late, after this deadline is over, and
	// they must never block or send on a closed channel.
	callback := make(chan string, len(connections))
	dataBytes := []byte(data)

	eventID := uuid.NewString()
	event := Event{
		id:       eventID,
		data:     dataBytes,
		callback: callback,
		timeout:  time.Now().Add(200 * time.Millisecond),
	}

	sent := 0
	received := 0
	errors := 0

	for _, conn := range connections {
		sent++
		if !conn.Enqueue(event) {
			errors++
		}
	}

	// A single timer for the whole wait: time.After inside the loop would
	// allocate one throwaway timer per response, and keep every one of them
	// alive until the deadline passes.
	timer := time.NewTimer(200 * time.Millisecond)
	defer timer.Stop()
	okResponse := eventID + " OK"

	// Wait for all the missing responses
outerLoop:
	for received+errors < sent {
		select {
		case response := <-callback:
			if response == okResponse {
				received++
			} else {
				errors++
			}
		case <-timer.C:
			// Deliveries we never got confirmation for are failures, not
			// silent successes.
			errors = sent - received
			break outerLoop
		}
	}

	var sb strings.Builder
	sb.WriteString(strconv.Itoa(received))
	sb.WriteString("/")
	sb.WriteString(strconv.Itoa(sent))
	sb.WriteString("/")
	sb.WriteString(strconv.Itoa(errors))
	return sb.String(), nil
}
