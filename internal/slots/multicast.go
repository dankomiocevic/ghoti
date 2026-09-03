package slots

import (
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dankomiocevic/ghoti/internal/auth"
	"github.com/dankomiocevic/ghoti/internal/connectionmanager"
)

type multicastSlot struct {
	users      map[string]string
	value      string
	slotID     string
	timeout    time.Duration
	deregTries int
	members    map[net.Conn]int
	mu         sync.RWMutex
	manager    connectionmanager.ConnectionManager
}

func newMulticastSlot(users map[string]string, conn connectionmanager.ConnectionManager, id string, timeout time.Duration, deregTries int) *multicastSlot {
	return &multicastSlot{
		users:      users,
		value:      "",
		manager:    conn,
		slotID:     id,
		timeout:    timeout,
		deregTries: deregTries,
		members:    make(map[net.Conn]int),
	}
}

func (m *multicastSlot) Read() string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.value
}

func (m *multicastSlot) CanRead(u *auth.User) bool {
	if len(m.users) == 0 {
		return true
	}

	return m.users[u.Name] == "r" || m.users[u.Name] == "a"
}

func (m *multicastSlot) CanWrite(u *auth.User) bool {
	if len(m.users) == 0 {
		return true
	}

	return m.users[u.Name] == "w" || m.users[u.Name] == "a"
}

// Register adds the connection to the group of clients that will receive the
// multicast messages. It is idempotent: subscribing again resets the failure
// counter for the client.
func (m *multicastSlot) Register(from net.Conn) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.members[from] = 0
	return "1", nil
}

// Deregister removes the connection from the group. Deregistering a
// connection that is not registered is a no-op.
func (m *multicastSlot) Deregister(from net.Conn) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.members, from)
	return "0", nil
}

func (m *multicastSlot) Write(data string, from net.Conn) (string, error) {
	m.mu.Lock()
	m.value = data
	targets := make([]net.Conn, 0, len(m.members))
	for conn := range m.members {
		targets = append(targets, conn)
	}
	m.mu.Unlock()

	if len(targets) == 0 {
		return "0/0/0", nil
	}

	result, err := m.manager.Multicast(m.buildEvent(data), targets, m.timeout)
	if err != nil {
		return "", err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	failed := make(map[net.Conn]struct{}, len(result.Failed))
	for _, conn := range result.Failed {
		failed[conn] = struct{}{}
	}

	for _, conn := range targets {
		if _, ok := m.members[conn]; !ok {
			// Already removed by a concurrent register/deregister.
			continue
		}

		if _, ok := failed[conn]; ok {
			m.members[conn]++
			if m.members[conn] >= m.deregTries {
				delete(m.members, conn)
			}
			continue
		}

		m.members[conn] = 0
	}

	var sb strings.Builder
	sb.WriteString(strconv.Itoa(result.Received))
	sb.WriteString("/")
	sb.WriteString(strconv.Itoa(result.Sent))
	sb.WriteString("/")
	sb.WriteString(strconv.Itoa(result.Errors))
	return sb.String(), nil
}

func (m *multicastSlot) buildEvent(data string) string {
	var sb strings.Builder
	sb.WriteString("a")
	sb.WriteString(m.slotID)
	sb.WriteString(data)
	sb.WriteString("\n")
	return sb.String()
}
