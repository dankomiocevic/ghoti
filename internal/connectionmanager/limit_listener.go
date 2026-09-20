package connectionmanager

import (
	"net"
	"sync"
)

// limitListener caps how many accepted connections may be open at once.
//
// When the cap is reached Accept parks until a connection closes, so the
// extra clients wait in the kernel accept queue without costing this process
// a file descriptor or a goroutine. That is the same behaviour as
// golang.org/x/net/netutil.LimitListener, inlined so a few lines of code do
// not pull in the module. A max of zero or less returns l unchanged.
func limitListener(l net.Listener, max int) net.Listener {
	if max <= 0 {
		return l
	}
	return &limitedListener{
		Listener: l,
		slots:    make(chan struct{}, max),
		done:     make(chan struct{}),
	}
}

type limitedListener struct {
	net.Listener
	slots chan struct{}
	// done is closed by Close so an Accept waiting for a slot returns like
	// one waiting on the socket would; the serving loops rely on that error
	// to stop.
	done      chan struct{}
	closeOnce sync.Once
}

func (l *limitedListener) Accept() (net.Conn, error) {
	select {
	case l.slots <- struct{}{}:
	case <-l.done:
		return nil, net.ErrClosed
	}
	c, err := l.Listener.Accept()
	if err != nil {
		<-l.slots
		return nil, err
	}
	return &limitedConn{Conn: c, release: func() { <-l.slots }}, nil
}

func (l *limitedListener) Close() error {
	l.closeOnce.Do(func() { close(l.done) })
	return l.Listener.Close()
}

// limitedConn hands its slot back exactly once, however many times it is
// closed: the managers and the HTTP server may both close a connection.
type limitedConn struct {
	net.Conn
	once    sync.Once
	release func()
}

func (c *limitedConn) Close() error {
	err := c.Conn.Close()
	c.once.Do(c.release)
	return err
}
