package server

import (
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"

	"github.com/dankomiocevic/ghoti/internal/auth"
	"github.com/dankomiocevic/ghoti/internal/cluster"
	"github.com/dankomiocevic/ghoti/internal/config"
	"github.com/dankomiocevic/ghoti/internal/errors"
	"github.com/dankomiocevic/ghoti/internal/slots"
)

type Server struct {
	listener      net.Listener
	slotsArray    [1000]slots.Slot
	usersMap      map[string]auth.User
	quit          chan interface{}
	wg            sync.WaitGroup
	connections   *ConnectionManager
	cluster       cluster.Cluster
	telnetSupport bool
}

func NewServer(config *config.Config, cluster cluster.Cluster) *Server {
	s := &Server{
		quit:    make(chan interface{}),
		cluster: cluster,
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
	s.connections = NewManager()
	s.telnetSupport = (config.Protocol == "telnet")
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
			connection := s.connections.Add(conn, s.telnetSupport)
			slog.Debug("Connection received",
				slog.String("id", connection.Id),
				slog.String("remote_addr", conn.RemoteAddr().String()),
			)

			s.wg.Add(1)
			go func() {
				s.handleUserConnection(connection)
				s.wg.Done()
			}()
		}
	}
}

func (s *Server) Stop() {
	slog.Debug("Closing main listener")
	close(s.quit)
	s.listener.Close()
	connList := s.connections.DeleteAll()

	for _, c := range connList {
		slog.Debug("Closing connection",
			slog.String("id", c.Id),
			slog.String("remote_addr", c.NetworkConn.RemoteAddr().String()),
		)

		close(c.Quit)
	}

	s.wg.Wait()
}

// TODO: Refactor this into smaller functions
// TODO: Improve error handling
// TODO: Remove missing direct conn.Write calls
func (s *Server) handleUserConnection(conn Connection) {
	defer s.connections.Delete(conn.Id)
	defer conn.Close()
	slog.Debug("Handling user connection",
		slog.String("remote_addr", conn.Id),
		slog.String("remote_addr", conn.NetworkConn.RemoteAddr().String()),
	)

	go conn.EventProcessor()
	c := conn.NetworkConn
	for {
		select {
		case <-conn.Quit:
			return
		default:
		}

		msg, err := conn.ReadMessage(s.telnetSupport)
		if err != nil {
			switch err.(type) {
			case errors.TranscientError:
				continue
			case errors.PermanentError:
				return
			default:
				slog.Error("Unidentified error reading message",
					slog.String("id", conn.Id),
					slog.Any("error", err))
			}
		}

		if msg.Command != 'q' && !s.cluster.IsLeader() {
			res := errors.Error("NOT_LEADER")
			err = conn.SendEvent(res.Response() + s.cluster.GetLeader())
			if err != nil {
				switch err.(type) {
				case errors.PermanentError:
					return
				}
			}
			slog.Debug("Request made to node that was not leader",
				slog.String("id", conn.Id),
				slog.String("remote_addr", conn.NetworkConn.RemoteAddr().String()),
			)
			continue
		}

		if msg.Command == 'u' {
			err = processUsername(s, &conn, msg)
			if err != nil {
				switch err.(type) {
				case errors.PermanentError:
					return
				}
			}
			continue
		}

		if msg.Command == 'p' {
			err = processPassword(s, &conn, msg)
			if err != nil {
				switch err.(type) {
				case errors.PermanentError:
					return
				}
			}
			continue
		}

		current_slot := s.slotsArray[msg.Slot]
		if current_slot == nil {
			res := errors.Error("MISSING_SLOT")
			err = conn.SendEvent(res.Response())
			if err != nil {
				switch err.(type) {
				case errors.PermanentError:
					return
				}
			}
			slog.Debug("Missing slot",
				slog.Int("slot", msg.Slot),
				slog.String("id", conn.Id),
				slog.String("remote_addr", conn.NetworkConn.RemoteAddr().String()),
			)
			continue
		}

		var value string
		if msg.Command == 'w' {
			if !current_slot.CanWrite(&conn.LoggedUser) {
				slog.Info("Connection trying to write on slot without permission",
					slog.Int("slot", msg.Slot),
					slog.String("id", conn.Id),
					slog.String("remote_addr", conn.NetworkConn.RemoteAddr().String()),
				)
				res := errors.Error("WRITE_PERMISSION")
				err = conn.SendEvent(res.Response())
				if err != nil {
					switch err.(type) {
					case errors.PermanentError:
						return
					}
				}
				continue
			}

			value, err = current_slot.Write(msg.Value, c)

			if err != nil {
				res := errors.Error("WRITE_FAILED")
				slog.Error("Error writing in slot",
					slog.Int("slot", msg.Slot),
					slog.Any("error", err),
				)
				err = conn.SendEvent(res.Response())
				if err != nil {
					switch err.(type) {
					case errors.PermanentError:
						return
					}
				}
				continue
			} else {
				slog.Debug("Value written in slot",
					slog.Int("slot", msg.Slot),
					slog.String("value", msg.Value),
					slog.String("id", conn.Id),
					slog.String("remote_addr", conn.NetworkConn.RemoteAddr().String()),
				)
			}

		} else if msg.Command == 'q' {
			slog.Debug("Client disconnected",
				slog.String("id", conn.Id),
				slog.String("remote_addr", conn.NetworkConn.RemoteAddr().String()),
			)
			return
		} else if msg.Command == 'r' {
			if current_slot.CanRead(&conn.LoggedUser) {
				value = current_slot.Read()
			} else {
				slog.Info("Connection trying to read on slot without permission",
					slog.Int("slot", msg.Slot),
					slog.String("id", conn.Id),
					slog.String("remote_addr", conn.NetworkConn.RemoteAddr().String()),
				)
				res := errors.Error("READ_PERMISSION")
				err = conn.SendEvent(res.Response())
				if err != nil {
					slog.Error("Error sending event to connection",
						slog.String("id", conn.Id),
						slog.Any("error", err))
					switch err.(type) {
					case errors.TranscientError:
						continue
					case errors.PermanentError:
						return
					default:
						slog.Error("Unidentified error writing message",
							slog.String("id", conn.Id),
							slog.Any("error", err))
					}
				}
				continue
			}
		}

		err = sendSlotData(msg, conn, value)
		if err != nil {
			slog.Error("Error sending event to connection",
				slog.String("id", conn.Id),
				slog.Any("error", err))
			switch err.(type) {
			case errors.TranscientError:
				continue
			case errors.PermanentError:
				return
			default:
				slog.Error("Unidentified error writing message",
					slog.String("id", conn.Id),
					slog.Any("error", err))
			}
		}
	}
}

func sendSlotData(msg Message, conn Connection, value string) error {
	var sb strings.Builder
	sb.WriteString("v")
	sb.WriteString(fmt.Sprintf("%03d", msg.Slot))
	sb.WriteString(value)
	sb.WriteString("\n")
	err := conn.SendEvent(sb.String())
	if err != nil {
		return err
	}
	slog.Debug("Value read from slot",
		slog.Int("slot", msg.Slot),
		slog.String("value", value),
		slog.String("id", conn.Id),
		slog.String("remote_addr", conn.NetworkConn.RemoteAddr().String()),
	)
	return nil
}

func processUsername(s *Server, conn *Connection, msg Message) error {
	err := auth.ValidateUsername(msg.Value)
	if err != nil {
		res := errors.Error("WRONG_USER")
		conn.SendEvent(res.Response())
		slog.Debug("Invalid user received",
			slog.String("user", msg.Value),
			slog.String("id", conn.Id),
			slog.String("remote_addr", conn.NetworkConn.RemoteAddr().String()),
		)
		slog.Debug("Disconnecting", slog.String("remote_addr", conn.NetworkConn.RemoteAddr().String()))
		return errors.PermanentError{Err: "Bad password"}
	} else {
		conn.LoggedUser = auth.User{}
		conn.Username = msg.Value
		conn.IsLogged = false

		var sb strings.Builder
		sb.WriteString("v")
		sb.WriteString(conn.Username)
		sb.WriteString("\n")
		err = conn.SendEvent(sb.String())
		if err != nil {
			return err
		}
		slog.Debug("Username set for connection",
			slog.String("user", conn.Username),
			slog.String("id", conn.Id),
			slog.String("remote_addr", conn.NetworkConn.RemoteAddr().String()),
		)
	}
	return nil
}

func processPassword(s *Server, conn *Connection, msg Message) error {
	user, err := auth.GetUser(conn.Username, msg.Value)
	if err != nil {
		res := errors.Error("WRONG_PASS")
		conn.SendEvent(res.Response())
		slog.Debug("Invalid password received",
			slog.String("id", conn.Id),
			slog.String("remote_addr", conn.NetworkConn.RemoteAddr().String()),
		)
		slog.Debug("Disconnecting",
			slog.String("id", conn.Id),
			slog.String("remote_addr", conn.NetworkConn.RemoteAddr().String()),
		)
		return errors.PermanentError{Err: "Bad password"}
	} else {
		if s.usersMap[user.Name].Password != user.Password {
			res := errors.Error("WRONG_LOGIN")
			conn.SendEvent(res.Response())
			slog.Warn("Invalid login received",
				slog.String("id", conn.Id),
				slog.String("remote_addr", conn.NetworkConn.RemoteAddr().String()),
			)
			slog.Debug("Disconnecting", slog.String("remote_addr", conn.NetworkConn.RemoteAddr().String()))
			return errors.PermanentError{Err: "Invalid login"}
		} else {
			conn.LoggedUser = user
			conn.IsLogged = true

			var sb strings.Builder
			sb.WriteString("v")
			sb.WriteString(conn.Username)
			sb.WriteString("\n")
			err = conn.SendEvent(sb.String())
			if err != nil {
				return err
			}
			slog.Debug("User logged in for connection",
				slog.String("user", conn.Username),
				slog.String("id", conn.Id),
				slog.String("remote_addr", conn.NetworkConn.RemoteAddr().String()),
			)
		}
	}
	return nil
}
