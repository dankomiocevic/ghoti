package slots

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/dankomiocevic/ghoti/internal/auth"
)

type timeoutSlot struct {
	users   map[string]string
	value   string
	owner   net.Conn
	timeout time.Duration
	ttl     time.Time
	mu      sync.RWMutex
}

func newTimeoutSlot(timeout int, users map[string]string) (*timeoutSlot, error) {
	if timeout < 1 {
		return nil, fmt.Errorf("timeout value in timeout_memory slot must be bigger than zero")
	}

	return &timeoutSlot{value: "", timeout: time.Duration(timeout) * time.Second, ttl: time.Time{}, users: users}, nil
}

func (m *timeoutSlot) Read() string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.value
}

func (m *timeoutSlot) Write(data string, from net.Conn) (string, error) {
	timeNow := time.Now()

	m.mu.Lock()
	defer m.mu.Unlock()

	if timeNow.After(m.ttl) {
		m.owner = from
		m.value = data
		m.ttl = timeNow.Add(m.timeout)

		return m.value, nil
	}

	if from == m.owner {
		m.value = data
		m.ttl = timeNow.Add(m.timeout)

		return m.value, nil
	}

	return "", errors.New("permission denied to write slot")
}

func (m *timeoutSlot) CanRead(u *auth.User) bool {
	if len(m.users) == 0 {
		return true
	}

	return m.users[u.Name] == "r" || m.users[u.Name] == "a"
}

func (m *timeoutSlot) CanWrite(u *auth.User) bool {
	if len(m.users) == 0 {
		return true
	}

	return m.users[u.Name] == "w" || m.users[u.Name] == "a"
}
