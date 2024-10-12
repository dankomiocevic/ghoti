package server

import (
	"net"

	"github.com/dankomiocevic/ghoti/internal/auth"
)

type Connection struct {
	Id          string
	Quit        chan interface{}
	NetworkConn net.Conn
	LoggedUser  auth.User
	IsLogged    bool
	Username    string
}

func (c *Connection) Close() error {
	return c.NetworkConn.Close()
}
