package glibevent

import (
	"net"
	"testing"
	"time"
)

type ServerHandle struct{}

func (s *ServerHandle) OnBoot() bool {
	return true
}

// OnShutdown fires when the engine is being shut down, it is called right after
// all event-loops and connections are closed.
func (s *ServerHandle) OnShutdown() bool {
	return true
}

// OnClose fires when a connection has been closed.
// The parameter err is the last known connection error.
func (s *ServerHandle) OnClose(_ net.Conn) bool {
	return true
}

// OnTick fires immediately after the engine starts and will fire again
// following the duration specified by the delay return value.
func (s *ServerHandle) OnTick() (delay time.Duration, _ bool) {
	return 0, true
}

// OnTraffic fires when a local socket receives data from the peer and without decoder .
func (s *ServerHandle) OnReceive(conn net.Conn, data []byte) bool {
	return true
}

// OnTraffic fires when a local socket receives data from the peer and with decoder.
func (s *ServerHandle) OnTraffic(conn net.Conn, head, data []byte) bool {
	return true
}

// OnOpen fires when a new connection has been opened.
// The parameter out is the return value which is going to be sent back to the peer.
func (*ServerHandle) OnOpen(c net.Conn) (out []byte, action bool) {
	return nil, true
}

func TestServer(t *testing.T) {
	svr := NewServer(&ServerHandle{}, &DefaultDecoder{})
	svr.Server("tcp://127.0.0.1:10233")
}
