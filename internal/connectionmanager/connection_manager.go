package connectionmanager

import (
	"net"
	"time"
)

type CallbackFn func(int, []byte, *Connection) error

type ConnectionManager interface {
	StartListening(string) error
	ServeConnections(CallbackFn) error
	Broadcast(string) (string, error)
	Multicast(data string, targets []net.Conn, timeout time.Duration) (MulticastResult, error)
	Delete(string)
	GetAddr() string
	Close()
}

func GetConnectionManager(protocol string) ConnectionManager {
	switch protocol {
	case "standard":
		return NewTCPManager()
	case "telnet":
		return NewTelnetManager()
	case "http":
		return NewHTTPManager()
	default:
		return nil
	}
}
