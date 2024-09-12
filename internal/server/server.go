package server

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"sync"

	"github.com/dankomiocevic/ghoti/internal/auth"
	"github.com/dankomiocevic/ghoti/internal/config"
	"github.com/dankomiocevic/ghoti/internal/slots"
)

type Server struct {
	listener   net.Listener
	slotsArray [1000]slots.Slot
	usersArray map[string]auth.User
	quit       chan interface{}
	wg         sync.WaitGroup
}

func NewServer(config *config.Config) *Server {
	s := &Server{
		quit: make(chan interface{}),
	}

	l, err := net.Listen("tcp", config.TcpAddr)
	if err != nil {
		panic(err)
	}
	s.listener = l
	s.slotsArray = config.Slots
	s.usersArray = config.Users
	s.wg.Add(1)

	go s.serve()
	return s
}

func (s *Server) serve() {
	defer s.wg.Done()

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.quit:
				return
			default:
				fmt.Println("accept error", err)
			}
		} else {
			s.wg.Add(1)
			go func() {
				s.handleUserConnection(Connection{NetworkConn: conn, LoggedUser: auth.User{}, Username: "", IsLogged: false})
				s.wg.Done()
			}()
		}
	}
}

func (s *Server) Stop() {
	close(s.quit)
	s.listener.Close()
	s.wg.Wait()
}

func (s *Server) handleUserConnection(conn Connection) {
	defer conn.NetworkConn.Close()

	c := conn.NetworkConn
	buf := make([]byte, 41)

	for {
		size, err := bufio.NewReader(c).Read(buf)
		if err != nil {
			return
		}

		msg, err := ParseMessage(size, buf)
		if err != nil {
			c.Write([]byte("e\n"))
			continue
		}

		if msg.Command == 'u' {
			err := auth.ValidateUsername(msg.Value)
			if err != nil {
				c.Write([]byte("e\n"))
				// TODO: Close connection
			} else {
				conn.LoggedUser = auth.User{}
				conn.Username = msg.Value
				conn.IsLogged = false

				var sb strings.Builder
				sb.WriteString("v")
				sb.WriteString(conn.Username)
				sb.WriteString("\n")
				c.Write([]byte(sb.String()))
			}
			continue
		}

		if msg.Command == 'p' {
			user, err := auth.GetUser(conn.Username, msg.Value)
			if err != nil {
				c.Write([]byte("e\n"))
				// TODO: Close connection
			} else {
				conn.LoggedUser = user
				conn.IsLogged = true

				var sb strings.Builder
				sb.WriteString("v")
				sb.WriteString(conn.Username)
				sb.WriteString("\n")
				c.Write([]byte(sb.String()))
			}
			continue
		}

		current_slot := s.slotsArray[msg.Slot]
		if current_slot == nil {
			c.Write([]byte("e\n"))
			continue
		}

		var value string
		if msg.Command == 'w' {
			value, err = current_slot.Write(msg.Value, c)

			if err != nil {
				c.Write([]byte("e\n"))
				continue
			}
		} else if msg.Command == 'q' {
			// TODO: Close connection
		} else {
			value = current_slot.Read()
		}

		var sb strings.Builder
		sb.WriteString("v")
		sb.WriteString(fmt.Sprintf("%03d", msg.Slot))
		sb.WriteString(value)
		sb.WriteString("\n")
		c.Write([]byte(sb.String()))
	}
}
