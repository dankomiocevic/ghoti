package slots

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/dankomiocevic/ghoti/internal/auth"
)

type leakyBucketSlot struct {
	users  map[string]string
	value  int64
	size   int64
	rate   int
	window int64
	mu     sync.Mutex
}

func newLeakyBucketSlot(bucketSize, refreshRate int, users map[string]string) (*leakyBucketSlot, error) {
	if bucketSize < 1 {
		return nil, fmt.Errorf("bucket size must be bigger than zero")
	}

	if refreshRate < 1 {
		return nil, fmt.Errorf("refresh rate cannot be zero")
	}

	return &leakyBucketSlot{value: 0, size: int64(bucketSize), rate: refreshRate, window: currentWindowMillis(refreshRate), users: users}, nil
}

func currentWindowMillis(rate int) int64 {
	return time.Now().UnixMilli() / int64(rate)
}

func (m *leakyBucketSlot) Read() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	current := currentWindowMillis(m.rate)
	windowDiff := current - m.window
	if windowDiff > m.size {
		m.window = current
		m.value = 0
	} else {
		m.window = current
		m.value = max(0, m.value-windowDiff)
	}

	if m.value == m.size {
		return "0"
	}
	m.value = min(m.size, m.value+1)
	return "1"
}

func (m *leakyBucketSlot) CanRead(u *auth.User) bool {
	if len(m.users) == 0 {
		return true
	}

	return m.users[u.Name] == "r" || m.users[u.Name] == "a"
}

func (m *leakyBucketSlot) CanWrite(u *auth.User) bool {
	return false
}

func (m *leakyBucketSlot) Write(data string, from net.Conn) (string, error) {
	return "", fmt.Errorf("token bucket slots cannot be used to write")
}
