package rdmanet

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"
)

// The registry is a lightweight, out-of-band name→Addr directory for UD
// endpoints, in the same spirit as the handshake package: pure Go (no cgo) over
// a line-delimited JSON TCP protocol, so it is independently unit-testable
// without RDMA hardware. It is an optional convenience — callers may instead
// distribute Addr.String() by any means and rebuild via ResolveAddr.
//
// Wire protocol (one JSON object per line, request then single-line response):
//
//	register: {"op":"register","name":"x","addr":"<Addr.String()>"}  -> {"ok":true}
//	lookup:   {"op":"lookup","name":"x"}                              -> {"ok":true,"addr":"..."} | {"ok":false,"error":"..."}

const registryRWTimeout = 5 * time.Second

type registryRequest struct {
	Op   string `json:"op"`
	Name string `json:"name"`
	Addr string `json:"addr,omitempty"`
}

type registryResponse struct {
	OK    bool   `json:"ok"`
	Addr  string `json:"addr,omitempty"`
	Error string `json:"error,omitempty"`
}

// Registry is an in-memory name→Addr directory served over TCP. Create one with
// NewRegistry; stop it with Close. It is safe for concurrent clients.
type Registry struct {
	ln   net.Listener
	mu   sync.Mutex
	tbl  map[string]string // name -> Addr.String()
	done chan struct{}
}

// NewRegistry starts a registry server listening on addr (e.g. ":9100" or
// "127.0.0.1:0"). The server runs in a background goroutine until Close.
func NewRegistry(addr string) (*Registry, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	r := &Registry{
		ln:   ln,
		tbl:  make(map[string]string),
		done: make(chan struct{}),
	}
	go r.serve()
	return r, nil
}

// Addr returns the registry's listen address (useful when addr used port 0).
func (r *Registry) Addr() net.Addr { return r.ln.Addr() }

// Close stops the registry server.
func (r *Registry) Close() error {
	select {
	case <-r.done:
	default:
		close(r.done)
	}
	return r.ln.Close()
}

func (r *Registry) serve() {
	for {
		conn, err := r.ln.Accept()
		if err != nil {
			select {
			case <-r.done:
				return
			default:
				return
			}
		}
		go r.handle(conn)
	}
}

func (r *Registry) handle(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	_ = conn.SetReadDeadline(time.Now().Add(registryRWTimeout))
	br := bufio.NewReader(conn)
	line, err := br.ReadBytes('\n')
	if err != nil {
		return
	}
	var req registryRequest
	if err := json.Unmarshal(line, &req); err != nil {
		writeRegistryResponse(conn, registryResponse{OK: false, Error: "bad request"})
		return
	}
	switch req.Op {
	case "register":
		if req.Name == "" || req.Addr == "" {
			writeRegistryResponse(conn, registryResponse{OK: false, Error: "missing name/addr"})
			return
		}
		r.mu.Lock()
		r.tbl[req.Name] = req.Addr
		r.mu.Unlock()
		writeRegistryResponse(conn, registryResponse{OK: true})
	case "lookup":
		r.mu.Lock()
		addr, ok := r.tbl[req.Name]
		r.mu.Unlock()
		if !ok {
			writeRegistryResponse(conn, registryResponse{OK: false, Error: "not found"})
			return
		}
		writeRegistryResponse(conn, registryResponse{OK: true, Addr: addr})
	default:
		writeRegistryResponse(conn, registryResponse{OK: false, Error: "unknown op"})
	}
}

func writeRegistryResponse(conn net.Conn, resp registryResponse) {
	_ = conn.SetWriteDeadline(time.Now().Add(registryRWTimeout))
	b, _ := json.Marshal(resp)
	b = append(b, '\n')
	_, _ = conn.Write(b)
}

// registerAddr is the shared client helper: it sends a register request for
// name→addr to the registry at registryAddr.
func registerAddr(registryAddr, name string, addr *Addr) error {
	if addr == nil {
		return fmt.Errorf("rdmanet: cannot register nil Addr")
	}
	resp, err := registryRoundTrip(registryAddr, registryRequest{
		Op:   "register",
		Name: name,
		Addr: addr.String(),
	})
	if err != nil {
		return err
	}
	if !resp.OK {
		return fmt.Errorf("rdmanet: register failed: %s", resp.Error)
	}
	return nil
}

// LookupAddr queries the registry at registryAddr for the Addr registered under
// name. It is the client counterpart of PacketConn.Register and is pure Go.
func LookupAddr(registryAddr, name string) (*Addr, error) {
	resp, err := registryRoundTrip(registryAddr, registryRequest{Op: "lookup", Name: name})
	if err != nil {
		return nil, err
	}
	if !resp.OK {
		return nil, fmt.Errorf("rdmanet: lookup %q failed: %s", name, resp.Error)
	}
	return ResolveAddr(resp.Addr)
}

// registryRoundTrip dials registryAddr, sends one request line, and reads one
// response line.
func registryRoundTrip(registryAddr string, req registryRequest) (registryResponse, error) {
	conn, err := net.DialTimeout("tcp", registryAddr, registryRWTimeout)
	if err != nil {
		return registryResponse{}, err
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetWriteDeadline(time.Now().Add(registryRWTimeout))
	b, _ := json.Marshal(req)
	b = append(b, '\n')
	if _, err := conn.Write(b); err != nil {
		return registryResponse{}, err
	}
	_ = conn.SetReadDeadline(time.Now().Add(registryRWTimeout))
	line, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil {
		return registryResponse{}, err
	}
	var resp registryResponse
	if err := json.Unmarshal(line, &resp); err != nil {
		return registryResponse{}, err
	}
	return resp, nil
}
