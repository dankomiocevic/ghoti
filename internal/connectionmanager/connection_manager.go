package connectionmanager

import (
	"net"
	"time"
)

type CallbackFn func(int, []byte, *Connection) error

type ConnectionManager interface {
	// SetMaxConnections caps how many client connections may be open at
	// once; zero or less means no cap. It must be called before
	// StartListening, which is where the cap is applied.
	SetMaxConnections(int)
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
