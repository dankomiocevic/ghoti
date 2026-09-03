package connectionmanager

import (
	"net"
	"strings"
	"time"

	"github.com/google/uuid"
)

// MulticastResult contains the outcome of a multicast delivery.
// Failed contains the targets that could not receive the message, either
// because the delivery failed or because they are not connected anymore.
type MulticastResult struct {
	Received int
	Sent     int
	Errors   int
	Failed   []net.Conn
}

// multicastToConnections sends the data to the connections that are included in
// the targets list and waits up to the timeout for all the confirmations.
//
// Every target gets its own event identifier, this way the response can be
// matched back to the connection that failed. Targets that are not present in
// the connections list are counted as failures because the client is gone.
func multicastToConnections(connections []Connection, targets []net.Conn, data string, timeout time.Duration) MulticastResult {
	result := MulticastResult{}
	if len(targets) == 0 {
		return result
	}

	pending := make(map[net.Conn]struct{}, len(targets))
	for _, t := range targets {
		pending[t] = struct{}{}
	}

	// The channel is never closed and has room for every response, this way the
	// event processors can never block sending a response we are not reading.
	callback := make(chan string, len(targets))
	dataBytes := []byte(data)
	deadline := time.Now().Add(timeout)
	events := make(map[string]net.Conn, len(targets))

	for _, conn := range connections {
		if _, ok := pending[conn.NetworkConn]; !ok {
			continue
		}
		delete(pending, conn.NetworkConn)

		eventID := uuid.NewString()
		event := Event{
			id:       eventID,
			data:     dataBytes,
			callback: callback,
			timeout:  deadline,
		}

		result.Sent++
		select {
		case conn.Events <- event:
			events[eventID] = conn.NetworkConn
		default:
			result.Errors++
			result.Failed = append(result.Failed, conn.NetworkConn)
		}
	}

	// The remaining targets are not connected anymore, they still burn a
	// delivery attempt so they can be de-registered.
	for target := range pending {
		result.Sent++
		result.Errors++
		result.Failed = append(result.Failed, target)
	}

	for len(events) > 0 {
		select {
		case response := <-callback:
			id, status, found := strings.Cut(response, " ")
			if !found {
				continue
			}

			conn, ok := events[id]
			if !ok {
				continue
			}
			delete(events, id)

			if status == "OK" {
				result.Received++
			} else {
				result.Errors++
				result.Failed = append(result.Failed, conn)
			}
		case <-time.After(time.Until(deadline)):
			for _, conn := range events {
				result.Errors++
				result.Failed = append(result.Failed, conn)
			}
			return result
		}
	}

	return result
}
