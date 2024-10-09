package run

import (
	"bufio"
	"net"
	"testing"
	"time"

	"github.com/dankomiocevic/ghoti/cmd"
)

func TestSingleNode(t *testing.T) {
	rootCmd := cmd.NewRootCommand()
	cmd := NewRunCommand()
	rootCmd.AddCommand(cmd)

	rootCmd.SetArgs([]string{"run", "--addr", "localhost:9876"})

	go func() {
		rootCmd.Execute()
	}()

	time.Sleep(time.Duration(100) * time.Millisecond)
	// connect to the TCP Server
	conn, err := net.Dial("tcp", ":9876")
	if err != nil {
		t.Fatalf("couldn't connect to the server: %v", err)
	}

	if _, err := conn.Write([]byte("r000\n")); err != nil {
		t.Fatalf("couldn't send request: %v", err)
	}

	reader := bufio.NewReader(conn)
	response, err := reader.ReadBytes(byte('\n'))
	if string(response[0]) == "e" {
		t.Fatalf("received error response: %v", string(response))
	}
}
