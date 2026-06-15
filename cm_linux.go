//go:build linux && cgo

package gordma

/*
#cgo LDFLAGS: -lrdmacm -libverbs
#include <stdlib.h>
#include <string.h>
#include <rdma/rdma_cma.h>

// gordma_resolve_addr parses "host:port" via getaddrinfo-free path is complex;
// instead we accept a pre-resolved sockaddr from Go. Helpers below wrap the
// rdma_cm calls that are awkward to call directly from cgo.

static struct rdma_cm_id *gordma_create_id(struct rdma_event_channel *ec) {
	struct rdma_cm_id *id = NULL;
	if (rdma_create_id(ec, &id, NULL, RDMA_PS_TCP) != 0) {
		return NULL;
	}
	return id;
}
*/
import "C"

import (
	"fmt"
	"net"
	"strconv"
	"time"
	"unsafe"
)

// cmID is the platform handle stored in CMConn on Linux.
type cmID struct {
	ec     *C.struct_rdma_event_channel
	id     *C.struct_rdma_cm_id
	ownsEC bool // Dial-created conns own their ec; accepted conns share the listener's ec.
}

// sockaddrIn builds a C sockaddr_in/in6 from a Go *net.TCPAddr into the given
// buffer and returns its length. RoCE v2 paths use IPv4 commonly; we support
// both v4 and v6.
func fillSockaddr(sa *C.struct_sockaddr_storage, addr *net.TCPAddr) error {
	C.memset(unsafe.Pointer(sa), 0, C.sizeof_struct_sockaddr_storage)
	ip4 := addr.IP.To4()
	if ip4 != nil {
		sin := (*C.struct_sockaddr_in)(unsafe.Pointer(sa))
		sin.sin_family = C.AF_INET
		sin.sin_port = C.uint16_t(htons(uint16(addr.Port)))
		C.memcpy(unsafe.Pointer(&sin.sin_addr), unsafe.Pointer(&ip4[0]), 4)
		return nil
	}
	ip6 := addr.IP.To16()
	if ip6 == nil {
		return fmt.Errorf("gordma: invalid IP %v", addr.IP)
	}
	sin6 := (*C.struct_sockaddr_in6)(unsafe.Pointer(sa))
	sin6.sin6_family = C.AF_INET6
	sin6.sin6_port = C.uint16_t(htons(uint16(addr.Port)))
	C.memcpy(unsafe.Pointer(&sin6.sin6_addr), unsafe.Pointer(&ip6[0]), 16)
	return nil
}

func htons(v uint16) uint16 { return (v<<8)&0xff00 | v>>8 }

func resolveTCPAddr(addr string) (*net.TCPAddr, error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("gordma: bad address %q: %w", addr, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("gordma: bad port in %q: %w", addr, err)
	}
	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		return nil, fmt.Errorf("gordma: cannot resolve %q: %w", host, err)
	}
	return &net.TCPAddr{IP: ips[0], Port: port}, nil
}

// Listener accepts incoming rdma_cm connections.
type Listener struct {
	ec *C.struct_rdma_event_channel
	id *C.struct_rdma_cm_id
}

// Listen binds an rdma_cm id to addr ("host:port") and starts listening.
func Listen(addr string) (*Listener, error) {
	tcpAddr, err := resolveTCPAddr(addr)
	if err != nil {
		return nil, err
	}
	ec := C.rdma_create_event_channel()
	if ec == nil {
		return nil, fmt.Errorf("gordma: rdma_create_event_channel failed: %w", lastErrno())
	}
	id := C.gordma_create_id(ec)
	if id == nil {
		C.rdma_destroy_event_channel(ec)
		return nil, fmt.Errorf("gordma: rdma_create_id failed: %w", lastErrno())
	}
	var sa C.struct_sockaddr_storage
	if err := fillSockaddr(&sa, tcpAddr); err != nil {
		C.rdma_destroy_id(id)
		C.rdma_destroy_event_channel(ec)
		return nil, err
	}
	if C.rdma_bind_addr(id, (*C.struct_sockaddr)(unsafe.Pointer(&sa))) != 0 {
		C.rdma_destroy_id(id)
		C.rdma_destroy_event_channel(ec)
		return nil, fmt.Errorf("gordma: rdma_bind_addr failed: %w", lastErrno())
	}
	if C.rdma_listen(id, 8) != 0 {
		C.rdma_destroy_id(id)
		C.rdma_destroy_event_channel(ec)
		return nil, fmt.Errorf("gordma: rdma_listen failed: %w", lastErrno())
	}
	return &Listener{ec: ec, id: id}, nil
}

// Close stops listening and releases resources.
func (l *Listener) Close() error {
	if l == nil {
		return nil
	}
	if l.id != nil {
		C.rdma_destroy_id(l.id)
		l.id = nil
	}
	if l.ec != nil {
		C.rdma_destroy_event_channel(l.ec)
		l.ec = nil
	}
	return nil
}

// waitEvent blocks for the next CM event of the expected type, returning the
// event's cm_id. The event is acked before returning.
func getCMEvent(ec *C.struct_rdma_event_channel, want C.enum_rdma_cm_event_type) (*C.struct_rdma_cm_id, error) {
	var ev *C.struct_rdma_cm_event
	if C.rdma_get_cm_event(ec, &ev) != 0 {
		return nil, fmt.Errorf("gordma: rdma_get_cm_event failed: %w", lastErrno())
	}
	gotType := ev.event
	id := ev.id
	C.rdma_ack_cm_event(ev)
	if gotType != want {
		return nil, fmt.Errorf("gordma: unexpected CM event %d, want %d", gotType, want)
	}
	return id, nil
}

// buildConn creates a PD/CQ/QP on the given cm_id and returns a CMConn. The QP
// is created through rdma_create_qp so the CM drives the state transitions.
func buildConn(ec *C.struct_rdma_event_channel, id *C.struct_rdma_cm_id, depth int, ownsEC bool) (*CMConn, error) {
	pdC := C.ibv_alloc_pd(id.verbs)
	if pdC == nil {
		return nil, fmt.Errorf("gordma: ibv_alloc_pd failed: %w", lastErrno())
	}
	cqC := C.ibv_create_cq(id.verbs, C.int(depth), nil, nil, 0)
	if cqC == nil {
		C.ibv_dealloc_pd(pdC)
		return nil, fmt.Errorf("gordma: ibv_create_cq failed: %w", lastErrno())
	}
	var ia C.struct_ibv_qp_init_attr
	ia.send_cq = cqC
	ia.recv_cq = cqC
	ia.qp_type = C.IBV_QPT_RC
	ia.cap.max_send_wr = C.uint32_t(depth)
	ia.cap.max_recv_wr = C.uint32_t(depth)
	ia.cap.max_send_sge = 1
	ia.cap.max_recv_sge = 1
	if C.rdma_create_qp(id, pdC, &ia) != 0 {
		C.ibv_destroy_cq(cqC)
		C.ibv_dealloc_pd(pdC)
		return nil, fmt.Errorf("gordma: rdma_create_qp failed: %w", lastErrno())
	}
	conn := &CMConn{
		pd: &PD{pd: pdC},
		cq: &CQ{cq: cqC},
		qp: &QP{qp: id.qp, typ: QPTypeRC},
		id: cmID{ec: ec, id: id, ownsEC: ownsEC},
	}
	return conn, nil
}

// Accept waits for one incoming connection and completes establishment. The
// returned CMConn has its QP in RTS.
func (l *Listener) Accept() (*CMConn, error) {
	if l == nil || l.id == nil {
		return nil, ErrClosed
	}
	connID, err := getCMEvent(l.ec, C.RDMA_CM_EVENT_CONNECT_REQUEST)
	if err != nil {
		return nil, err
	}
	conn, err := buildConn(l.ec, connID, 128, false)
	if err != nil {
		C.rdma_destroy_id(connID)
		return nil, err
	}
	var cp C.struct_rdma_conn_param
	C.memset(unsafe.Pointer(&cp), 0, C.sizeof_struct_rdma_conn_param)
	cp.responder_resources = 1
	cp.initiator_depth = 1
	cp.rnr_retry_count = 7
	if C.rdma_accept(connID, &cp) != 0 {
		_ = conn.Close()
		return nil, fmt.Errorf("gordma: rdma_accept failed: %w", lastErrno())
	}
	if _, err := getCMEvent(l.ec, C.RDMA_CM_EVENT_ESTABLISHED); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return conn, nil
}

// Dial resolves addr, establishes an rdma_cm connection, and returns a CMConn
// whose QP is in RTS. A zero timeout uses DefaultCMTimeout.
func Dial(addr string, timeout time.Duration) (*CMConn, error) {
	if timeout <= 0 {
		timeout = DefaultCMTimeout
	}
	tcpAddr, err := resolveTCPAddr(addr)
	if err != nil {
		return nil, err
	}
	ec := C.rdma_create_event_channel()
	if ec == nil {
		return nil, fmt.Errorf("gordma: rdma_create_event_channel failed: %w", lastErrno())
	}
	id := C.gordma_create_id(ec)
	if id == nil {
		C.rdma_destroy_event_channel(ec)
		return nil, fmt.Errorf("gordma: rdma_create_id failed: %w", lastErrno())
	}
	var sa C.struct_sockaddr_storage
	if err := fillSockaddr(&sa, tcpAddr); err != nil {
		C.rdma_destroy_id(id)
		C.rdma_destroy_event_channel(ec)
		return nil, err
	}
	toMs := C.int(timeout / time.Millisecond)
	if C.rdma_resolve_addr(id, nil, (*C.struct_sockaddr)(unsafe.Pointer(&sa)), toMs) != 0 {
		C.rdma_destroy_id(id)
		C.rdma_destroy_event_channel(ec)
		return nil, fmt.Errorf("gordma: rdma_resolve_addr failed: %w", lastErrno())
	}
	if _, err := getCMEvent(ec, C.RDMA_CM_EVENT_ADDR_RESOLVED); err != nil {
		C.rdma_destroy_id(id)
		C.rdma_destroy_event_channel(ec)
		return nil, err
	}
	if C.rdma_resolve_route(id, toMs) != 0 {
		C.rdma_destroy_id(id)
		C.rdma_destroy_event_channel(ec)
		return nil, fmt.Errorf("gordma: rdma_resolve_route failed: %w", lastErrno())
	}
	if _, err := getCMEvent(ec, C.RDMA_CM_EVENT_ROUTE_RESOLVED); err != nil {
		C.rdma_destroy_id(id)
		C.rdma_destroy_event_channel(ec)
		return nil, err
	}
	conn, err := buildConn(ec, id, 128, true)
	if err != nil {
		C.rdma_destroy_id(id)
		C.rdma_destroy_event_channel(ec)
		return nil, err
	}
	var cp C.struct_rdma_conn_param
	C.memset(unsafe.Pointer(&cp), 0, C.sizeof_struct_rdma_conn_param)
	cp.responder_resources = 1
	cp.initiator_depth = 1
	cp.retry_count = 7
	cp.rnr_retry_count = 7
	if C.rdma_connect(id, &cp) != 0 {
		return nil, fmt.Errorf("gordma: rdma_connect failed: %w", lastErrno())
	}
	if _, err := getCMEvent(ec, C.RDMA_CM_EVENT_ESTABLISHED); err != nil {
		return nil, fmt.Errorf("gordma: connection not established (peer rejected?): %w", err)
	}
	return conn, nil
}

// Close tears down the connection and releases its QP/CQ/PD and CM resources.
func (c *CMConn) Close() error {
	if c == nil {
		return nil
	}
	if c.id.id != nil {
		C.rdma_disconnect(c.id.id)
		if c.qp != nil && c.qp.qp != nil {
			C.rdma_destroy_qp(c.id.id)
			c.qp.qp = nil
		}
	}
	if c.cq != nil {
		_ = c.cq.Close()
	}
	if c.pd != nil {
		_ = c.pd.Close()
	}
	if c.id.id != nil {
		C.rdma_destroy_id(c.id.id)
		c.id.id = nil
	}
	if c.id.ec != nil {
		if c.id.ownsEC {
			C.rdma_destroy_event_channel(c.id.ec)
		}
		c.id.ec = nil
	}
	return nil
}
