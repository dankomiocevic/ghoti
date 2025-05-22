package server

import (
	"bufio"
	"math/rand/v2"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dankomiocevic/ghoti/internal/cluster"
	"github.com/dankomiocevic/ghoti/internal/config"
	"github.com/dankomiocevic/ghoti/internal/connection_manager"
	"github.com/dankomiocevic/ghoti/internal/slots"

	"github.com/spf13/viper"
)

func generateConfig(port string) *config.Config {
	c := config.DefaultConfig()

	if viper.IsSet("protocol") {
		c.Protocol = viper.GetString("protocol")
	}

	c.Connections = connection_manager.GetConnectionManager(c.Protocol)

	viper.Set("addr", "localhost:"+port)
	c.TcpAddr = viper.GetString("addr")

	viper.Set("slot_000.kind", "simple_memory")
	slot_zero, _ := slots.GetSlot(viper.Sub("slot_000"), c.Connections, "000")
	c.Slots[0] = slot_zero
	slot_one, _ := slots.GetSlot(viper.Sub("slot_000"), c.Connections, "001")
	c.Slots[1] = slot_one
	slot_two, _ := slots.GetSlot(viper.Sub("slot_000"), c.Connections, "002")
	c.Slots[2] = slot_two

	viper.Set("slot_001.kind", "timeout_memory")
	viper.Set("slot_001.timeout", 60)
	slot_three, _ := slots.GetSlot(viper.Sub("slot_001"), c.Connections, "003")
	c.Slots[3] = slot_three
	viper.Set("slot_004.kind", "simple_memory")
	viper.Set("slot_004.users.pepe", "r")
	viper.Set("slot_004.users.bobby", "w")
	viper.Set("slot_004.users.sammy", "a")
	slot_four, _ := slots.GetSlot(viper.Sub("slot_004"), c.Connections, "004")
	c.Slots[4] = slot_four

	viper.Set("users.pepe", "passw0rd")
	viper.Set("users.bobby", "otherPassw0rd")
	viper.Set("users.sammy", "samPassw0rd")
	c.LoadUsers()

	return c
}

func runServer(t *testing.T) (*Server, net.Conn) {
	port := "9" + strconv.Itoa(rand.IntN(899)+100)
	// start the TCP Server
	s := NewServer(generateConfig(port), cluster.NewEmptyCluster())

	// wait for the TCP Server to start
	time.Sleep(time.Duration(100) * time.Millisecond)

	// connect to the TCP Server
	conn, err := net.Dial("tcp", ":"+port)
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

	response := sendData(t, conn, "r0")
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
	connOther, err := net.Dial("tcp", s.connections.GetAddr())
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

	if response != "exxx002\n" {
		t.Fatalf("Server did not return error")
	}
}

func TestEmptyPassword(t *testing.T) {
	s, conn := runServer(t)
	defer s.Stop()
	defer conn.Close()

	// Login with empty password
	response := sendData(t, conn, "p000\n")

	if response != "exxx003\n" {
		t.Fatalf("Server did not return error: %s", response)
	}
}

func TestWrongPassword(t *testing.T) {
	s, conn := runServer(t)
	defer s.Stop()
	defer conn.Close()

	// Login as pepe/passw0rd
	sendData(t, conn, "upepe\n")
	response := sendData(t, conn, "p12345\n")

	if response != "exxx004\n" {
		t.Fatalf("Server did not return error: %s", response)
	}
}

func TestAccessForbidden(t *testing.T) {
	s, conn := runServer(t)
	defer s.Stop()
	defer conn.Close()

	// Read on Slot 4 that is password protected
	response := sendData(t, conn, "r004\n")

	if response != "e004008\n" {
		t.Fatalf("Server did not return error: %s", response)
	}

	// Write on Slot 4 that is password protected
	response_two := sendData(t, conn, "w004Something\n")

	if response_two != "e004006\n" {
		t.Fatalf("Server did not return error: %s", response)
	}
}

func TestReadOnly(t *testing.T) {
	s, conn := runServer(t)
	defer s.Stop()
	defer conn.Close()

	sendData(t, conn, "upepe\n")
	sendData(t, conn, "ppassw0rd\n")

	// Read on Slot 4 that is password protected
	response := sendData(t, conn, "r004\n")

	if response != "v004\n" {
		t.Fatalf("Server did not return the value: %s", response)
	}

	// Write on Slot 4 that is password protected
	response_two := sendData(t, conn, "w004Something\n")

	if response_two != "e004006\n" {
		t.Fatalf("Server did not return an error")
	}
}

func TestWriteOnly(t *testing.T) {
	s, conn := runServer(t)
	defer s.Stop()
	defer conn.Close()

	sendData(t, conn, "ubobby\n")
	sendData(t, conn, "potherPassw0rd\n")

	// Read on Slot 4 that is password protected
	response := sendData(t, conn, "r004\n")

	if response != "e004008\n" {
		t.Fatalf("Server did not return an error")
	}

	// Write on Slot 4 that is password protected
	response_two := sendData(t, conn, "w004Something\n")

	if response_two != "v004Something\n" {
		t.Fatalf("Server did not return the value: %s", response_two)
	}
}

func TestAllAccess(t *testing.T) {
	s, conn := runServer(t)
	defer s.Stop()
	defer conn.Close()

	sendData(t, conn, "usammy\n")
	sendData(t, conn, "psamPassw0rd\n")

	// Write on Slot 4 that is password protected
	response_two := sendData(t, conn, "w004SomethingSam\n")

	if response_two != "v004SomethingSam\n" {
		t.Fatalf("Server did not return the value: %s", response_two)
	}

	// Read on Slot 4 that is password protected
	response := sendData(t, conn, "r004\n")

	if response != "v004SomethingSam\n" {
		t.Fatalf("Server did not return the value: %s", response)
	}
}

func TestTelnetSupport(t *testing.T) {
	viper.Set("protocol", "telnet")

	s, conn := runServer(t)
	defer s.Stop()
	defer conn.Close()
	defer viper.Set("protocol", "standard")

	response := sendData(t, conn, "w000Hello\r\n")

	if response != "v000Hello\n" {
		t.Fatalf("unexpected server response: [%s]", response)
	}
}
