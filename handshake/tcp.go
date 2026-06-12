package handshake

import (
	"net"
	"time"
)

// Server listens on a TCP address and, on Accept, exchanges EndpointInfo with
// a single connecting client. It models the perftest "server" side of the
// out-of-band handshake.
type Server struct {
	ln net.Listener
}

// Listen starts a TCP listener on addr (e.g. ":18515" or "0.0.0.0:18515").
func Listen(addr string) (*Server, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	return &Server{ln: ln}, nil
}

// Addr returns the listener's network address (useful when addr used port 0).
func (s *Server) Addr() net.Addr { return s.ln.Addr() }

// Accept waits for one client, exchanges info, and returns the peer's
// EndpointInfo. The server sends local first, then reads the client's.
// A zero timeout means no deadline.
func (s *Server) Accept(local EndpointInfo, timeout time.Duration) (EndpointInfo, error) {
	conn, err := s.ln.Accept()
	if err != nil {
		return EndpointInfo{}, err
	}
	defer conn.Close()
	return exchange(conn, local, timeout)
}

// Close stops the listener.
func (s *Server) Close() error {
	if s == nil || s.ln == nil {
		return nil
	}
	return s.ln.Close()
}

// Dial connects to a server at addr, exchanges info, and returns the peer's
// EndpointInfo. The client sends local first, then reads the server's — the
// same write-then-read order as the server, which is deadlock-free for these
// tiny payloads.
func Dial(addr string, local EndpointInfo, timeout time.Duration) (EndpointInfo, error) {
	var conn net.Conn
	var err error
	if timeout > 0 {
		conn, err = net.DialTimeout("tcp", addr, timeout)
	} else {
		conn, err = net.Dial("tcp", addr)
	}
	if err != nil {
		return EndpointInfo{}, err
	}
	defer conn.Close()
	return exchange(conn, local, timeout)
}
