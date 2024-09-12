package server

import (
	"bufio"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/dankomiocevic/ghoti/internal/config"
	"github.com/dankomiocevic/ghoti/internal/slots"

	"github.com/spf13/viper"
)

func generateConfig() *config.Config {
	c := config.DefaultConfig()

	viper.Set("slot_000.kind", "simple_memory")
	slot_zero, _ := slots.GetSlot(viper.Sub("slot_000"))
	c.Slots[0] = slot_zero
	slot_one, _ := slots.GetSlot(viper.Sub("slot_000"))
	c.Slots[1] = slot_one
	slot_two, _ := slots.GetSlot(viper.Sub("slot_000"))
	c.Slots[2] = slot_two

	viper.Set("slot_001.kind", "timeout_memory")
	viper.Set("slot_001.timeout", 60)
	slot_three, _ := slots.GetSlot(viper.Sub("slot_001"))
	c.Slots[3] = slot_three

	viper.Set("users.pepe", "passw0rd")

	return c
}

func runServer(t *testing.T) (*Server, net.Conn) {
	// start the TCP Server
	s := NewServer(generateConfig())

	// wait for the TCP Server to start
	time.Sleep(time.Duration(100) * time.Millisecond)

	// connect to the TCP Server
	conn, err := net.Dial("tcp", ":9090")
	if err != nil {
		t.Fatalf("couldn't connect to the server: %v", err)
	}

	return s, conn
}

func sendData(t *testing.T, conn net.Conn, data string) string {
	if _, err := conn.Write([]byte(data)); err != nil {
		t.Fatalf("couldn't send request: %v", err)
	}

	reader := bufio.NewReader(conn)
	response, err := reader.ReadBytes(byte('\n'))

	if err != nil {
		t.Fatalf("couldn't read server response: %v", err)
	}

	return string(response)
}

func TestWrite(t *testing.T) {
	s, conn := runServer(t)
	defer s.Stop()
	defer conn.Close()

	response := sendData(t, conn, "w000Hello\n")

	if response != "v000Hello\n" {
		t.Fatalf("unexpected server response: %s", response)
	}
}

func TestWriteInOtherSlot(t *testing.T) {
	s, conn := runServer(t)
	defer s.Stop()
	defer conn.Close()

	response := sendData(t, conn, "w001Hello\n")

	if response != "v001Hello\n" {
		t.Fatalf("unexpected server response: %s", response)
	}
}

func TestRead(t *testing.T) {
	s, conn := runServer(t)
	defer s.Stop()
	defer conn.Close()

	// First we store a value
	sendData(t, conn, "w000HelloAgain\n")

	// Then, we read that value
	response := sendData(t, conn, "r000\n")

	if response != "v000HelloAgain\n" {
		t.Fatalf("unexpected server response: %s", response)
	}
}

func TestUnsupportedCommand(t *testing.T) {
	s, conn := runServer(t)
	defer s.Stop()
	defer conn.Close()

	response := sendData(t, conn, "a000Hello\n")
	if !strings.HasPrefix(response, "e") {
		t.Fatalf("unexpected server response: %s", response)
	}
}

func TestMessageTooLong(t *testing.T) {
	s, conn := runServer(t)
	defer s.Stop()
	defer conn.Close()

	response := sendData(t, conn, "w000HelloWithAReallyLongMessageThatShouldBeWrong\n")

	if !strings.HasPrefix(response, "e") {
		t.Fatalf("unexpected server response: %s", response)
	}
}

func TestMessageTooShort(t *testing.T) {
	s, conn := runServer(t)
	defer s.Stop()
	defer conn.Close()

	response := sendData(t, conn, "r0\n")
	if !strings.HasPrefix(response, "e") {
		t.Fatalf("unexpected server response: %s", response)
	}
}

func TestMessageNotTerminated(t *testing.T) {
	s, conn := runServer(t)
	defer s.Stop()
	defer conn.Close()

	response := sendData(t, conn, "r0")
	if !strings.HasPrefix(response, "e") {
		t.Fatalf("unexpected server response: %s", response)
	}
}

func TestReadInANonConfiguredSlot(t *testing.T) {
	s, conn := runServer(t)
	defer s.Stop()
	defer conn.Close()

	response := sendData(t, conn, "r123")
	if !strings.HasPrefix(response, "e") {
		t.Fatalf("unexpected server response: %s", response)
	}
}

func TestWriteInANonConfiguredSlot(t *testing.T) {
	s, conn := runServer(t)
	defer s.Stop()
	defer conn.Close()

	response := sendData(t, conn, "w123TEST")
	if !strings.HasPrefix(response, "e") {
		t.Fatalf("unexpected server response: %s", response)
	}
}

// Tests for timeout memory slot

func TestWriteTimeoutMemory(t *testing.T) {
	s, conn := runServer(t)
	defer s.Stop()
	defer conn.Close()

	response := sendData(t, conn, "w003HelloTimeout\n")

	if response != "v003HelloTimeout\n" {
		t.Fatalf("unexpected server response: %s", response)
	}
}

func TestWriteTimeoutNotOwner(t *testing.T) {
	s, conn := runServer(t)
	defer s.Stop()
	defer conn.Close()

	sendData(t, conn, "w003HelloOwner\n")

	// connect to the TCP Server
	connOther, err := net.Dial("tcp", ":9090")
	if err != nil {
		t.Fatalf("couldn't connect to the server: %v", err)
	}
	defer connOther.Close()
	response := sendData(t, connOther, "w003HelloOther\n")

	if !strings.HasPrefix(response, "e") {
		t.Fatalf("unexpected server response: %s", response)
	}
}

func TestReadTimeout(t *testing.T) {
	s, conn := runServer(t)
	defer s.Stop()
	defer conn.Close()

	sendData(t, conn, "w003HelloTimeout\n")

	response := sendData(t, conn, "r003\n")

	if response != "v003HelloTimeout\n" {
		t.Fatalf("unexpected server response: %s", response)
	}
}

// Tests for login

func TestLogin(t *testing.T) {
	s, conn := runServer(t)
	defer s.Stop()
	defer conn.Close()

	// Login as pepe/passw0rd
	sendData(t, conn, "upepe\n")
	response := sendData(t, conn, "ppassw0rd\n")

	if response != "vpepe\n" {
		t.Fatalf("unexpected server response: %s", response)
	}
}

func TestUser(t *testing.T) {
	s, conn := runServer(t)
	defer s.Stop()
	defer conn.Close()

	// Login as pepe/passw0rd
	response := sendData(t, conn, "upepe\n")

	if response != "vpepe\n" {
		t.Fatalf("Server did not return the username: %s", response)
	}
}

func TestInvalidUsername(t *testing.T) {
	s, conn := runServer(t)
	defer s.Stop()
	defer conn.Close()

	// Login as pepe/passw0rd
	response := sendData(t, conn, "upepe!\n")

	if response != "e\n" {
		t.Fatalf("Server did not return error")
	}
}

func TestEmptyPassword(t *testing.T) {
	s, conn := runServer(t)
	defer s.Stop()
	defer conn.Close()

	// Login as pepe/passw0rd
	response := sendData(t, conn, "p\n")

	if response != "e\n" {
		t.Fatalf("Server did not return error")
	}
}
