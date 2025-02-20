package connection_manager

import (
	"fmt"
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

// This test creates 100 connections to check the broadcast functionality
func TestBroadcast(t *testing.T) {
	var wg sync.WaitGroup

	servers := []net.Conn{}
	clients := []net.Conn{}

	for i := 0; i < 100; i++ {
		server, client := net.Pipe()
		servers = append(servers, server)
		clients = append(clients, client)
	}

	// Close one of the servers to make it fail
	servers[5].Close()

	connections := make(map[string]Connection)

	for i, server := range servers {
		id := fmt.Sprintf("%d", i+1)
		c := &Connection{
			Id:          id,
			Quit:        make(chan interface{}),
			Events:      make(chan Event, 10),
			NetworkConn: server,
			IsLogged:    false,
			Username:    "",
			Callback:    make(chan string, 10),
			Buffer:      make([]byte, 1024),
			Timeout:     200,
		}
		connections[id] = *c
		go c.EventProcessor()
	}

	// Create a new TcpManager
	manager := TcpManager{
		connections: connections,
		quit:        make(chan interface{}),
		lock:        sync.RWMutex{},
	}

	go func() {
		output, err := manager.Broadcast("Hello World")
		if err != nil {
			t.Errorf("Error broadcasting message: %s", err)
		}

		if output != "99/100/1" {
			t.Errorf("Expected '99/100/1', got %s", output)
		}
		for _, conn := range servers {
			conn.Close()
		}
	}()

	for i, conn := range clients {
		if i == 5 {
			continue
		}

		wg.Add(1)
		go func() {
			conn.SetDeadline(time.Now().Add(time.Second))
			value, _ := io.ReadAll(conn)
			if string(value) != "Hello World" {
				t.Errorf("Expected 'Hello World', got %s", string(value))
			}
			wg.Done()
		}()
	}

	wg.Wait()
}
