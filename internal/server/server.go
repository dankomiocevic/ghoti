package server

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/dankomiocevic/ghoti/internal/auth"
	"github.com/dankomiocevic/ghoti/internal/cluster"
	"github.com/dankomiocevic/ghoti/internal/config"
	"github.com/dankomiocevic/ghoti/internal/connection_manager"
	"github.com/dankomiocevic/ghoti/internal/errors"
	"github.com/dankomiocevic/ghoti/internal/slots"
)

type Server struct {
	slotsArray  [1000]slots.Slot
	usersMap    map[string]auth.User
	connections connection_manager.ConnectionManager
	cluster     cluster.Cluster
}

func NewServer(config *config.Config, cluster cluster.Cluster) *Server {
	s := &Server{
		cluster: cluster,
	}

	slog.Info("Starting server...")
	slog.Debug("Opening tcp for listening", slog.String("tcp", config.TCPAddr))

	s.connections = config.Connections
	s.connections.StartListening(config.TCPAddr)

	s.slotsArray = config.Slots
	s.usersMap = config.Users

	go s.connections.ServeConnections(s.HandleMessage)
	return s
}

func (s *Server) Stop() {
	s.connections.Close()
}

func (s *Server) HandleMessage(size int, data []byte, conn *connection_manager.Connection) error {
	msg, err := ParseMessage(size, data)
	if err != nil {
		res := errors.Error("PARSE_ERROR")
		slog.Debug("Error parsing message: "+err.Error(),
			slog.String("id", conn.ID),
			slog.String("remote_addr", conn.NetworkConn.RemoteAddr().String()),
		)
		return conn.SendEvent(res.Response("xxx"))
	}

	currentSlot := s.slotsArray[msg.Slot]

	if msg.Command == 'q' {
		slog.Debug("Client disconnected",
			slog.String("id", conn.ID),
			slog.String("remote_addr", conn.NetworkConn.RemoteAddr().String()),
		)
		return errors.PermanentError{Err: "Client disconnected"}
	}
	if !s.cluster.IsLeader() {
		res := errors.Error("NOT_LEADER")
		slog.Debug("Request made to node that was not leader",
			slog.String("id", conn.ID),
			slog.String("remote_addr", conn.NetworkConn.RemoteAddr().String()),
		)
		return conn.SendEvent(res.Response("xxx") + s.cluster.GetLeader())
	}

	if msg.Command == 'u' {
		return processUsername(conn, msg)
	}

	if msg.Command == 'p' {
		return processPassword(s, conn, msg)
	}

	if currentSlot == nil {
		res := errors.Error("MISSING_SLOT")
		slog.Debug("Missing slot",
			slog.Int("slot", msg.Slot),
			slog.String("id", conn.ID),
			slog.String("remote_addr", conn.NetworkConn.RemoteAddr().String()),
		)
		return conn.SendEvent(res.Response(fmt.Sprintf("%03d", msg.Slot)))
	}

	if msg.Command == 'w' {
		return processWrite(conn, currentSlot, msg)
	}

	if msg.Command == 'r' {
		return processRead(conn, currentSlot, msg)
	}
	return nil
}

func processRead(conn *connection_manager.Connection, currentSlot slots.Slot, msg Message) error {
	if currentSlot.CanRead(&conn.LoggedUser) {
		value := currentSlot.Read()
		err := sendSlotData(msg, conn, value)
		return err
	}
	slog.Error("Connection trying to read on slot without permission",
		slog.Int("slot", msg.Slot),
		slog.String("id", conn.ID),
		slog.String("remote_addr", conn.NetworkConn.RemoteAddr().String()),
	)
	res := errors.Error("READ_PERMISSION")
	err := conn.SendEvent(res.Response(fmt.Sprintf("%03d", msg.Slot)))
	return err
}

func processWrite(conn *connection_manager.Connection, currentSlot slots.Slot, msg Message) error {
	if !currentSlot.CanWrite(&conn.LoggedUser) {
		slog.Info("Connection trying to write on slot without permission",
			slog.Int("slot", msg.Slot),
			slog.String("id", conn.ID),
			slog.String("remote_addr", conn.NetworkConn.RemoteAddr().String()),
		)
		res := errors.Error("WRITE_PERMISSION")
		err := conn.SendEvent(res.Response(fmt.Sprintf("%03d", msg.Slot)))
		if err != nil {
			return err
		}
		return nil
	}

	value, err := currentSlot.Write(msg.Value, conn.NetworkConn)

	if err != nil {
		res := errors.Error("WRITE_FAILED")
		slog.Error("Error writing in slot",
			slog.Int("slot", msg.Slot),
			slog.Any("error", err),
		)
		err = conn.SendEvent(res.Response(fmt.Sprintf("%03d", msg.Slot)))
		return err
	}
	slog.Debug("Value written in slot",
		slog.Int("slot", msg.Slot),
		slog.String("value", msg.Value),
		slog.String("id", conn.ID),
		slog.String("remote_addr", conn.NetworkConn.RemoteAddr().String()),
	)
	err = sendSlotData(msg, conn, value)
	return err
}

func sendSlotData(msg Message, conn *connection_manager.Connection, value string) error {
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
		slog.String("id", conn.ID),
		slog.String("remote_addr", conn.NetworkConn.RemoteAddr().String()),
	)
	return nil
}

func processUsername(conn *connection_manager.Connection, msg Message) error {
	err := auth.ValidateUsername(msg.Value)
	if err != nil {
		res := errors.Error("WRONG_USER")
		conn.SendEvent(res.Response("xxx"))
		slog.Debug("Invalid user received",
			slog.String("user", msg.Value),
			slog.String("id", conn.ID),
			slog.String("remote_addr", conn.NetworkConn.RemoteAddr().String()),
		)
		slog.Debug("Disconnecting", slog.String("remote_addr", conn.NetworkConn.RemoteAddr().String()))
		return errors.PermanentError{Err: "Bad password"}
	}
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
		slog.String("id", conn.ID),
		slog.String("remote_addr", conn.NetworkConn.RemoteAddr().String()),
	)
	return nil
}

func processPassword(s *Server, conn *connection_manager.Connection, msg Message) error {
	user, err := auth.GetUser(conn.Username, msg.Value)
	if err != nil {
		res := errors.Error("WRONG_PASS")
		conn.SendEvent(res.Response("xxx"))
		slog.Debug("Invalid password received",
			slog.String("id", conn.ID),
			slog.String("remote_addr", conn.NetworkConn.RemoteAddr().String()),
		)
		slog.Debug("Disconnecting",
			slog.String("id", conn.ID),
			slog.String("remote_addr", conn.NetworkConn.RemoteAddr().String()),
		)
		return errors.PermanentError{Err: "Bad password"}
	}
	if s.usersMap[user.Name].Password != user.Password {
		res := errors.Error("WRONG_LOGIN")
		conn.SendEvent(res.Response("xxx"))
		slog.Warn("Invalid login received",
			slog.String("id", conn.ID),
			slog.String("remote_addr", conn.NetworkConn.RemoteAddr().String()),
		)
		slog.Debug("Disconnecting", slog.String("remote_addr", conn.NetworkConn.RemoteAddr().String()))
		return errors.PermanentError{Err: "Invalid login"}
	}
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
		slog.String("id", conn.ID),
		slog.String("remote_addr", conn.NetworkConn.RemoteAddr().String()),
	)
	return nil
}
