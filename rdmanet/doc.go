// Package rdmanet provides a net-style, high-level API over the low-level RDMA
// verbs object model exposed by the parent gordma package.
//
// Where gordma mirrors rdma-core's verbs objects (Device, Context, PD, MR, CQ,
// QP, AH, CompChannel) and requires the caller to register memory, post work
// requests, poll completion queues and drive the QP state machine by hand,
// rdmanet hides all of that behind a small set of net-like primitives:
//
//   - Conn — a reliable-connected (RC) endpoint with message semantics
//     (SendMsg/RecvMsg) and a byte-stream adapter (Read/Write) layered on top,
//     satisfying io.ReadWriteCloser.
//   - PacketConn — an unreliable-datagram (UD) endpoint with datagram
//     semantics (ReadFrom/WriteTo), preserving message boundaries.
//
// Connections are established either via the RDMA connection manager
// (librdmacm) by default — using net-style "host:port" addresses through
// Dial/Listen — or, when WithHandshake is supplied, via the TCP out-of-band
// handshake provided by the gordma/handshake package.
//
// # API shape, not interface contract
//
// rdmanet deliberately mirrors the shape of the net package (Dial, Listen,
// Accept, Close, Read, Write) for familiarity, but does NOT claim to implement
// the net.Conn / net.Listener / net.PacketConn contracts. In particular,
// deadline methods (SetDeadline and friends) are not provided in this version.
// Conn does satisfy io.ReadWriteCloser, so existing io.Reader/io.Writer code
// can consume it without modification.
//
// # Platform support
//
// Like the parent package, rdmanet performs real RDMA only on Linux builds
// with cgo and libibverbs/librdmacm available. On other platforms (or with
// CGO_ENABLED=0) a stub implementation is compiled instead: every entry point
// returns gordma.ErrNotSupported rather than crashing, so code that imports
// rdmanet still builds and runs everywhere. Use gordma.Supported to check at
// runtime.
package rdmanet
