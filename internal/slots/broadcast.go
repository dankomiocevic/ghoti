package slots

import (
	"net"
	"strings"
	"sync"

	"github.com/dankomiocevic/ghoti/internal/auth"
	"github.com/dankomiocevic/ghoti/internal/connection_manager"
)

type broadcastSlot struct {
	users   map[string]string
	value   string
	slotID  string
	mu      sync.RWMutex
	manager connection_manager.ConnectionManager
}

func newBroadcastSlot(users map[string]string, conn connection_manager.ConnectionManager, id string) *broadcastSlot {
	return &broadcastSlot{
		users:   users,
		value:   "",
		manager: conn,
		slotID:  id,
	}
}

func (m *broadcastSlot) Read() string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.value
}

func (m *broadcastSlot) CanRead(u *auth.User) bool {
	if len(m.users) == 0 {
		return true
	}

	return m.users[u.Name] == "r" || m.users[u.Name] == "a"
}

func (m *broadcastSlot) CanWrite(u *auth.User) bool {
	if len(m.users) == 0 {
		return true
	}

	return m.users[u.Name] == "w" || m.users[u.Name] == "a"
}

func (m *broadcastSlot) Write(data string, from net.Conn) (string, error) {
	m.mu.Lock()
	m.value = data
	m.mu.Unlock()

	var sb strings.Builder
	sb.WriteString("a")
	sb.WriteString(m.slotID)
	sb.WriteString(data)
	sb.WriteString("\n")
	response, err := m.manager.Broadcast(sb.String())
	if err != nil {
		return "", err
	}

	return response, nil
}
