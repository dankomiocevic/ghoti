package slots

import (
	"net"
	"sync"

	"github.com/dankomiocevic/ghoti/internal/auth"
)

type memorySlot struct {
	users map[string]string
	value string
	mu    sync.Mutex
}

func (m *memorySlot) Read() string {
	return m.value
}

func (m *memorySlot) CanRead(u *auth.User) bool {
	if len(m.users) == 0 {
		return true
	}

	return m.users[u.Name] == "r" || m.users[u.Name] == "a"
}

func (m *memorySlot) CanWrite(u *auth.User) bool {
	if len(m.users) == 0 {
		return true
	}

	return m.users[u.Name] == "w" || m.users[u.Name] == "a"
}

func (m *memorySlot) Write(data string, from net.Conn) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.value = data
	return m.value, nil
}
