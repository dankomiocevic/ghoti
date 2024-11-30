package slots

import (
	"fmt"
	"net"
	"strconv"
	"sync"

	"github.com/dankomiocevic/ghoti/internal/auth"
)

type tickerSlot struct {
	users  map[string]string
	value  int64
	size   int64
	rate   int
	window int64
	mu     sync.Mutex
}

func newTickerSlot(refreshRate, initialValue int, users map[string]string) (*tickerSlot, error) {
	if refreshRate < 1 {
		return nil, fmt.Errorf("Refresh rate cannot be zero")
	}

	if initialValue < 0 {
		return nil, fmt.Errorf("Initial value cannot be negative")
	}

	return &tickerSlot{value: int64(initialValue), rate: refreshRate, window: currentWindowMillis(refreshRate), users: users}, nil
}

func (m *tickerSlot) Read() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	current := currentWindowMillis(m.rate)
	windowDiff := current - m.window
	m.window = current
	m.value = max(0, m.value-windowDiff)

	return strconv.Itoa(int(m.value))
}

func (m *tickerSlot) CanRead(u *auth.User) bool {
	if len(m.users) == 0 {
		return true
	}

	return m.users[u.Name] == "r" || m.users[u.Name] == "a"
}

func (m *tickerSlot) CanWrite(u *auth.User) bool {
	if len(m.users) == 0 {
		return true
	}

	return m.users[u.Name] == "w" || m.users[u.Name] == "a"
}

func (m *tickerSlot) Write(data string, from net.Conn) (string, error) {
	dataInt, err := strconv.Atoi(data)
	if err != nil {
		return "", fmt.Errorf("Data must be an integer")
	}

	if dataInt < 0 {
		return "", fmt.Errorf("Data cannot be negative")
	}

	m.window = currentWindowMillis(m.rate)
	m.value = int64(dataInt)
	return strconv.Itoa(dataInt), nil
}
