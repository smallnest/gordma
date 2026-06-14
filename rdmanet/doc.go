// Package rdmanet provides a net-style, high-level API over the low-level RDMA
// verbs object model exposed by the parent gordma package.
//
// Where gordma mirrors rdma-core's verbs objects (Device, Context, PD, MR, CQ,
// QP, AH, CompChannel) and requires the caller to register memory, post work
// requests, poll completion queues and drive the QP state machine by hand,
// rdmanet hides all of that behind a small set of net-like primitives:
//
//   - Conn — a reliable-connected (RC) endpoint.
//   - PacketConn — an unreliable-datagram (UD) endpoint.
//   - Listener — accepts incoming RC connections.
//
// # Message semantics and the stream adapter
//
// RC is a message-oriented transport, so Conn's primary API is message-based:
// SendMsg/RecvMsg (and RecvMsgBuf) preserve message boundaries — one SendMsg is
// received as exactly one RecvMsg, regardless of size (large messages are
// transparently fragmented and reassembled). Layered on top is a byte-stream
// adapter, Read/Write, which makes Conn satisfy io.ReadWriteCloser so existing
// io.Reader/io.Writer code can consume it; Read returns bytes across message
// boundaries. Do not mix Read with RecvMsg on the same Conn — the stream reader
// may buffer part of a message a subsequent RecvMsg would not see. Pick one
// receive style per Conn.
//
// # Connection establishment
//
// Connections are established via the RDMA connection manager (librdmacm) by
// default, using net-style "host:port" addresses through Dial/Listen. Supplying
// WithHandshake switches to the TCP out-of-band handshake provided by the
// gordma/handshake package, which builds the verbs resources directly and
// exchanges QPN/PSN/GID/RKey over TCP. Both paths yield the same Conn type.
//
// # Datagrams and addressing
//
// PacketConn offers UD datagram semantics (ReadFrom/WriteTo), preserving
// message boundaries; a datagram larger than the path MTU is rejected rather
// than truncated. UD is connectionless, so peers are named by Addr
// (GID/QPN/QKey), which formats as "gid%qpn[#qkey]" via String and parses back
// with ResolveAddr. The optional Registry (NewRegistry / PacketConn.Register /
// LookupAddr) is a lightweight name→Addr directory for discovering peers
// out-of-band; callers may instead distribute Addr.String() by any means.
//
// # Performance knobs
//
// Batch APIs (Conn.SendBatch/RecvBatch, PacketConn.WriteToBatch/ReadFromBatch)
// amortize per-call overhead by posting/collecting several messages under one
// lock. Zero-copy sends (Conn.AllocBuffer + SendBuffer) transmit straight from
// a caller-owned, pre-registered Buffer with no bounce-buffer copy. WithPollMode
// selects how completions are drained: PollEvent (the default) blocks on a
// completion channel for low CPU use, while PollBusy spins for lowest latency at
// the cost of a busy core. Other options tune the device, port, GID index,
// queue depth, and per-slot buffer size.
//
// # Platform support
//
// Like the parent package, rdmanet performs real RDMA only on Linux builds with
// cgo and libibverbs/librdmacm available. On other platforms (or with
// CGO_ENABLED=0) a stub implementation is compiled instead: every entry point
// returns gordma.ErrNotSupported rather than crashing, so code that imports
// rdmanet still builds and runs everywhere. Use gordma.Supported to check at
// runtime.
package rdmanet
