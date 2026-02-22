package slots

import (
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/dankomiocevic/ghoti/internal/auth"
)

type tokenBucketSlot struct {
	users        map[string]string
	value        int
	size         int
	period       int64
	rate         int
	window       int64
	tokensPerReq int
	mu           sync.Mutex
}

func newTokenBucketSlot(periodString string, bucketSize, refreshRate, tokensPerReq int, users map[string]string) (*tokenBucketSlot, error) {
	if bucketSize < 1 {
		return nil, fmt.Errorf("Bucket size must be bigger than zero")
	}

	if refreshRate > bucketSize {
		return nil, fmt.Errorf("Refresh rate cannot be bigger than the bucket size")
	}

	if refreshRate < 1 {
		return nil, fmt.Errorf("Refresh rate cannot be zero")
	}

	if tokensPerReq > bucketSize {
		return nil, fmt.Errorf("Tokens per request cannot be bigger than the bucket size")
	}

	if tokensPerReq < 1 {
		return nil, fmt.Errorf("Tokens per request cannot be zero")
	}

	var period int64
	switch periodString {
	case "second":
		period = 1
	case "minute":
		period = 60
	case "hour":
		period = 3600
	default:
		return nil, fmt.Errorf("Period value is invalid on token_bucket slot: %s", periodString)
	}

	return &tokenBucketSlot{value: refreshRate, size: bucketSize, period: period, rate: refreshRate, window: currentWindow(period), tokensPerReq: tokensPerReq, users: users}, nil
}

func currentWindow(period int64) int64 {
	currentTime := time.Now().Unix()
	return currentTime / period
}

func (m *tokenBucketSlot) Read() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	current := currentWindow(m.period)
	if current != m.window {
		m.window = current
		m.value = min(m.size, m.value+m.rate)
	}

	retVal := min(m.value, m.tokensPerReq)

	m.value -= retVal
	return strconv.Itoa(retVal)
}

func (m *tokenBucketSlot) CanRead(u *auth.User) bool {
	if len(m.users) == 0 {
		return true
	}

	return m.users[u.Name] == "r" || m.users[u.Name] == "a"
}

func (m *tokenBucketSlot) CanWrite(u *auth.User) bool {
	return false
}

func (m *tokenBucketSlot) Write(data string, from net.Conn) (string, error) {
	return "", fmt.Errorf("Token bucket slots cannot be used to write")
}
