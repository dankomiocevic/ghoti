package connection_manager

type CallbackFn func(int, []byte, *Connection) error

type ConnectionManager interface {
	StartListening(string) error
	ServeConnections(CallbackFn) error
	Broadcast(string) (string, error)
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
	default:
		return nil
	}
}
