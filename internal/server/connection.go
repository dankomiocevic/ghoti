package server

import (
	"net"

	"github.com/dankomiocevic/ghoti/internal/auth"
)

type Connection struct {
	NetworkConn net.Conn
	LoggedUser  auth.User
	IsLogged    bool
	Username    string
}
