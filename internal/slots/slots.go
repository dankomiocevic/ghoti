package slots

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/dankomiocevic/ghoti/internal/auth"
	"github.com/spf13/viper"
)

type Slot interface {
	Read() string
	Write(string, net.Conn) (string, error)
	CanRead(*auth.User) bool
	CanWrite(*auth.User) bool
}

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

type timeoutSlot struct {
	users   map[string]string
	value   string
	owner   net.Conn
	timeout time.Duration
	ttl     time.Time
	mu      sync.Mutex
}

func (m *timeoutSlot) Read() string {
	return m.value
}

func (m *timeoutSlot) Write(data string, from net.Conn) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	timeNow := time.Now()
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

	return "", errors.New("Permission denied to write slot")
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

func GetSlot(v *viper.Viper) (Slot, error) {
	kind := v.GetString("kind")
	usersConfig := v.GetStringMap("users")

	users := make(map[string]string)
	if usersConfig != nil {
		for key, value := range usersConfig {
			users[key] = fmt.Sprintf("%v", value)
		}
	}

	if kind == "simple_memory" {
		return &memorySlot{value: "", users: users}, nil
	}

	if kind == "timeout_memory" {
		// TODO: validate this data
		timeoutConfig := v.GetInt("timeout")
		return &timeoutSlot{value: "", timeout: time.Duration(timeoutConfig) * time.Second, ttl: time.Time{}, users: users}, nil
	}

	return nil, errors.New("Invalid kind of slot")
}
