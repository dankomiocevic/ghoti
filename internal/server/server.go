package server

import (
	"bufio"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"

	"github.com/dankomiocevic/ghoti/internal/auth"
	"github.com/dankomiocevic/ghoti/internal/config"
	"github.com/dankomiocevic/ghoti/internal/errors"
	"github.com/dankomiocevic/ghoti/internal/slots"
)

type Server struct {
	listener   net.Listener
	slotsArray [1000]slots.Slot
	usersMap   map[string]auth.User
	quit       chan interface{}
	wg         sync.WaitGroup
}

func NewServer(config *config.Config) *Server {
	s := &Server{
		quit: make(chan interface{}),
	}

	slog.Info("Starting server...")
	slog.Debug("Opening tcp for listening", slog.String("tcp", config.TcpAddr))
	l, err := net.Listen("tcp", config.TcpAddr)
	if err != nil {
		slog.Error("Could not start server", slog.Any("error", err))
		panic(err)
	}
	s.listener = l
	s.slotsArray = config.Slots
	s.usersMap = config.Users
	s.wg.Add(1)

	go s.serve()
	return s
}

func (s *Server) serve() {
	defer s.wg.Done()

	slog.Debug("Starting loop to accept connections")
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.quit:
				return
			default:
				slog.Error("Error accepting connection", slog.Any("error", err))
			}
		} else {
			slog.Debug("Connection received")
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
	defer conn.Close()
	slog.Debug("Handling user connection", slog.String("remote_addr", conn.NetworkConn.RemoteAddr().String()))

	c := conn.NetworkConn
	buf := make([]byte, 41)

	for {
		size, err := bufio.NewReader(c).Read(buf)
		if err != nil {
			slog.Error("Error receiving data from connection", slog.Any("error", err))
			slog.Debug("Disconnecting", slog.String("remote_addr", conn.NetworkConn.RemoteAddr().String()))
			return
		}

		msg, err := ParseMessage(size, buf)
		if err != nil {
			res := errors.Error("WRONG_FORMAT")
			slog.Debug(
				"Wrong message format received",
				slog.String("remote_addr", conn.NetworkConn.RemoteAddr().String()),
			)

			c.Write([]byte(res.Response()))
			continue
		} else {
			slog.Debug("Received message",
				slog.String("msg", msg.Raw),
				slog.String("remote_addr", conn.NetworkConn.RemoteAddr().String()),
			)
		}

		if msg.Command == 'u' {
			err := auth.ValidateUsername(msg.Value)
			if err != nil {
				res := errors.Error("WRONG_USER")
				c.Write([]byte(res.Response()))
				slog.Debug("Invalid user received",
					slog.String("user", msg.Value),
					slog.String("remote_addr", conn.NetworkConn.RemoteAddr().String()),
				)
				slog.Debug("Disconnecting", slog.String("remote_addr", conn.NetworkConn.RemoteAddr().String()))
				return
			} else {
				conn.LoggedUser = auth.User{}
				conn.Username = msg.Value
				conn.IsLogged = false

				var sb strings.Builder
				sb.WriteString("v")
				sb.WriteString(conn.Username)
				sb.WriteString("\n")
				c.Write([]byte(sb.String()))
				slog.Debug("Username set for connection",
					slog.String("user", conn.Username),
					slog.String("remote_addr", conn.NetworkConn.RemoteAddr().String()),
				)
			}
			continue
		}

		if msg.Command == 'p' {
			user, err := auth.GetUser(conn.Username, msg.Value)
			if err != nil {
				res := errors.Error("WRONG_PASS")
				c.Write([]byte(res.Response()))
				slog.Debug("Invalid password received",
					slog.String("remote_addr", conn.NetworkConn.RemoteAddr().String()),
				)
				slog.Debug("Disconnecting", slog.String("remote_addr", conn.NetworkConn.RemoteAddr().String()))
				return
			} else {
				if s.usersMap[user.Name].Password != user.Password {
					res := errors.Error("WRONG_LOGIN")
					c.Write([]byte(res.Response()))
					slog.Warn("Invalid login received",
						slog.String("remote_addr", conn.NetworkConn.RemoteAddr().String()),
					)
					slog.Debug("Disconnecting", slog.String("remote_addr", conn.NetworkConn.RemoteAddr().String()))
					return
				} else {
					conn.LoggedUser = user
					conn.IsLogged = true

					var sb strings.Builder
					sb.WriteString("v")
					sb.WriteString(conn.Username)
					sb.WriteString("\n")
					c.Write([]byte(sb.String()))
					slog.Debug("User logged in for connection",
						slog.String("user", conn.Username),
						slog.String("remote_addr", conn.NetworkConn.RemoteAddr().String()),
					)
				}
			}
			continue
		}

		current_slot := s.slotsArray[msg.Slot]
		if current_slot == nil {
			res := errors.Error("MISSING_SLOT")
			c.Write([]byte(res.Response()))
			slog.Debug("Missing slot",
				slog.Int("slot", msg.Slot),
				slog.String("remote_addr", conn.NetworkConn.RemoteAddr().String()),
			)
			continue
		}

		var value string
		if msg.Command == 'w' {
			if !current_slot.CanWrite(&conn.LoggedUser) {
				slog.Info("Connection trying to write on slot without permission",
					slog.Int("slot", msg.Slot),
					slog.String("remote_addr", conn.NetworkConn.RemoteAddr().String()),
				)
				res := errors.Error("WRITE_PERMISSION")
				c.Write([]byte(res.Response()))
				continue
			}

			value, err = current_slot.Write(msg.Value, c)

			if err != nil {
				res := errors.Error("WRITE_FAILED")
				slog.Error("Error writing in slot",
					slog.Int("slot", msg.Slot),
					slog.Any("error", err),
				)
				c.Write([]byte(res.Response()))
				continue
			} else {
				slog.Debug("Value written in slot",
					slog.Int("slot", msg.Slot),
					slog.String("value", msg.Value),
					slog.String("remote_addr", conn.NetworkConn.RemoteAddr().String()),
				)
			}

		} else if msg.Command == 'q' {
			slog.Debug("Client disconnected",
				slog.String("remote_addr", conn.NetworkConn.RemoteAddr().String()),
			)
			return
		} else if msg.Command == 'r' {
			if current_slot.CanRead(&conn.LoggedUser) {
				value = current_slot.Read()
			} else {
				slog.Info("Connection trying to read on slot without permission",
					slog.Int("slot", msg.Slot),
					slog.String("remote_addr", conn.NetworkConn.RemoteAddr().String()),
				)
				res := errors.Error("READ_PERMISSION")
				c.Write([]byte(res.Response()))
				continue
			}
		}

		var sb strings.Builder
		sb.WriteString("v")
		sb.WriteString(fmt.Sprintf("%03d", msg.Slot))
		sb.WriteString(value)
		sb.WriteString("\n")
		c.Write([]byte(sb.String()))
		slog.Debug("Value read from slot",
			slog.Int("slot", msg.Slot),
			slog.String("value", value),
			slog.String("remote_addr", conn.NetworkConn.RemoteAddr().String()),
		)
	}
}
