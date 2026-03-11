package connectionmanager

type CallbackFn func(int, []byte, *Connection) error

type ConnectionManager interface {
	StartListening(string) error
	ServeConnections(CallbackFn) error
	Broadcast(string) (string, error)
	Delete(string)
	GetAddr() string
	Close()
}

// StreamingSlot is an optional interface that slot types can implement to indicate
// they support long-lived subscriptions. When an HTTP GET request targets a slot
// that implements this interface, the HTTP manager opens an SSE stream instead of
// returning an immediate value.
type StreamingSlot interface {
	IsStreaming() bool
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
