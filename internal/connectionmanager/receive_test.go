package connectionmanager

import (
	"bufio"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/dankomiocevic/ghoti/internal/errs"
)

// pipeConnection returns a Connection wired to one end of an in-memory
// pipe and the client end that the test writes into.
func pipeConnection(t *testing.T, bufferSize int, timeout time.Duration) (Connection, net.Conn) {
	t.Helper()
	server, client := net.Pipe()
	t.Cleanup(func() {
		server.Close()
		client.Close()
	})
	return NewConnection("test", server, bufferSize, timeout), client
}

// write sends data from a goroutine because a net.Pipe write blocks until
// the other side reads it.
func write(conn net.Conn, data string) {
	go conn.Write([]byte(data))
}

func receive(t *testing.T, conn *Connection) string {
	t.Helper()
	size, err := conn.ReceiveMessage()
	if err != nil {
		t.Fatalf("ReceiveMessage returned an error: %v", err)
	}
	return string(conn.Buffer[:size])
}

func TestReceiveMessageReturnsOneMessageWhenTwoArriveTogether(t *testing.T) {
	conn, client := pipeConnection(t, 41, time.Second)

	write(client, "r001\nr002\n")

	if got := receive(t, &conn); got != "r001\n" {
		t.Fatalf("first message: got %q, want %q", got, "r001\n")
	}
	if got := receive(t, &conn); got != "r002\n" {
		t.Fatalf("second message: got %q, want %q", got, "r002\n")
	}
}

func TestReceiveMessageJoinsAMessageSplitAcrossReads(t *testing.T) {
	conn, client := pipeConnection(t, 41, time.Second)

	go func() {
		client.Write([]byte("w003hel"))
		time.Sleep(20 * time.Millisecond)
		client.Write([]byte("lo\n"))
	}()

	if got := receive(t, &conn); got != "w003hello\n" {
		t.Fatalf("got %q, want %q", got, "w003hello\n")
	}
}

func TestReceiveMessageKeepsPartialMessageAcrossTimeout(t *testing.T) {
	conn, client := pipeConnection(t, 41, 50*time.Millisecond)

	write(client, "w003hel")
	// Let the first fragment land, then wait for the deadline to expire.
	time.Sleep(10 * time.Millisecond)
	_, err := conn.ReceiveMessage()
	if !errors.As(err, new(errs.TranscientError)) {
		t.Fatalf("expected a transient timeout error, got %v", err)
	}

	write(client, "lo\n")
	if got := receive(t, &conn); got != "w003hello\n" {
		t.Fatalf("got %q, want %q", got, "w003hello\n")
	}
}

func TestReceiveMessageRejectsOversizedLineAndRecovers(t *testing.T) {
	conn, client := pipeConnection(t, 8, time.Second)

	write(client, "w001this line is far too long\nr002\n")

	_, err := conn.ReceiveMessage()
	if !errors.Is(err, ErrMessageTooLong) {
		t.Fatalf("expected ErrMessageTooLong, got %v", err)
	}
	if got := receive(t, &conn); got != "r002\n" {
		t.Fatalf("message after the oversized one: got %q, want %q", got, "r002\n")
	}
}

func TestReceiveMessageReportsClosedConnection(t *testing.T) {
	conn, client := pipeConnection(t, 41, time.Second)

	client.Close()

	_, err := conn.ReceiveMessage()
	if !errors.As(err, new(errs.PermanentError)) {
		t.Fatalf("expected a permanent error, got %v", err)
	}
}

// The end-to-end check: commands that arrive in the same segment must reach
// the callback one at a time, never as a single merged message.
func TestTCPManagerDeliversCoalescedCommandsSeparately(t *testing.T) {
	m := NewTCPManager()
	if err := m.StartListening("127.0.0.1:0"); err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	received := make(chan string, 16)
	go m.ServeConnections(func(size int, buf []byte, c *Connection) error {
		received <- string(buf[:size])
		return nil
	})

	client, err := net.Dial("tcp", m.GetAddr())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	if _, err := client.Write([]byte("w001abc\nr002\nr003\n")); err != nil {
		t.Fatal(err)
	}

	want := []string{"w001abc", "r002", "r003"}
	for _, w := range want {
		select {
		case got := <-received:
			if got != w {
				t.Fatalf("got %q, want %q", got, w)
			}
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for %q", w)
		}
	}
}

func TestTCPManagerAnswersOversizedMessageWithParseError(t *testing.T) {
	m := NewTCPManager()
	if err := m.StartListening("127.0.0.1:0"); err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	received := make(chan string, 16)
	go m.ServeConnections(func(size int, buf []byte, c *Connection) error {
		received <- string(buf[:size])
		return nil
	})

	client, err := net.Dial("tcp", m.GetAddr())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	tooLong := "w001" + strings.Repeat("x", 60) + "\n"
	if _, err := client.Write([]byte(tooLong + "r002\n")); err != nil {
		t.Fatal(err)
	}

	client.SetReadDeadline(time.Now().Add(time.Second))
	response, err := bufio.NewReader(client).ReadString('\n')
	if err != nil {
		t.Fatalf("reading the response: %v", err)
	}
	if want := errs.Error("PARSE_ERROR").Response("xxx"); response != want {
		t.Fatalf("got response %q, want %q", response, want)
	}

	select {
	case got := <-received:
		if got != "r002" {
			t.Fatalf("got %q, want %q", got, "r002")
		}
	case <-time.After(time.Second):
		t.Fatal("the command after the oversized one never arrived")
	}
}
