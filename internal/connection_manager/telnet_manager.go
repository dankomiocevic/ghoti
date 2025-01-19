package connection_manager

import (
	"log/slog"
	"net"

	"github.com/dankomiocevic/ghoti/internal/errors"
)

type TelnetManager struct {
	tcpManager *TcpManager
}

func NewTelnetManager() *TelnetManager {
	return &TelnetManager{
		tcpManager: NewTcpManager(),
	}
}

func (m *TelnetManager) GetAddr() string {
	return m.tcpManager.listener.Addr().String()
}

func (m *TelnetManager) StartListening(tcpAddr string) error {
	return m.tcpManager.StartListening(tcpAddr)
}

func (m *TelnetManager) ServeConnections(callback CallbackFn) error {
	c := m.tcpManager
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
			connection := c.Add(conn, 43)
			slog.Debug("Connection received",
				slog.String("id", connection.Id),
				slog.String("remote_addr", conn.RemoteAddr().String()),
			)

			c.wg.Add(1)
			go func() {
				m.handleUserConnection(callback, connection)
				c.wg.Done()
			}()
		}
	}
}

func (m *TelnetManager) handleUserConnection(callback CallbackFn, conn Connection) {
	c := m.tcpManager
	defer c.Delete(conn.Id)
	defer conn.Close()
	slog.Debug("Handling user connection",
		slog.String("remote_addr", conn.Id),
		slog.String("remote_addr", conn.NetworkConn.RemoteAddr().String()),
	)

	go conn.EventProcessor()
	for {
		select {
		case <-conn.Quit:
			slog.Debug("Connection quit",
				slog.String("remote_addr", conn.Id),
				slog.String("remote_addr", conn.NetworkConn.RemoteAddr().String()),
			)
			return
		default:
		}

		size, err := conn.ReceiveMessage()
		if err != nil {
			slog.Debug(err.Error(),
				slog.String("remote_addr", conn.Id),
				slog.String("remote_addr", conn.NetworkConn.RemoteAddr().String()),
			)
			switch err.(type) {
			case errors.TranscientError:
				continue
			case errors.PermanentError:
				return
			default:
				slog.Error("Unidentified error reading message",
					slog.String("id", conn.Id),
					slog.Any("error", err))
				return
			}
		}

		// If the message is a telnet message, check if it finishes with a carriage return
		// and line feed (CRLF), return an error otherwise
		if conn.Buffer[size-2] != 13 || conn.Buffer[size-1] != 10 {
			res := errors.Error("PARSE_ERROR")
			slog.Debug("Message not terminated with CRLF",
				slog.String("remote_addr", conn.Id),
				slog.String("remote_addr", conn.NetworkConn.RemoteAddr().String()),
			)
			conn.SendEvent(res.Response())
			continue
		}
		conn.Buffer[size-2] = 10
		size -= 2

		err = callback(size, conn.Buffer, &conn)
		if err != nil {
			switch err.(type) {
			case errors.TranscientError:
				slog.Error(err.Error(),
					slog.String("id", conn.Id))
				continue
			case errors.PermanentError:
				slog.Error(err.Error(),
					slog.String("id", conn.Id))
				return
			default:
				slog.Error("Unidentified error type",
					slog.String("id", conn.Id),
					slog.Any("error", err))
			}
		}
	}
}

func (m *TelnetManager) Add(conn net.Conn, bufferSize int) Connection {
	return m.tcpManager.Add(conn, bufferSize)
}

func (m *TelnetManager) Delete(id string) {
	m.tcpManager.Delete(id)
}

func (m *TelnetManager) Close() {
	m.tcpManager.Close()
}
