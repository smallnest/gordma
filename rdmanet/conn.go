package rdmanet

import "io"

// Conn satisfies io.ReadWriteCloser on every build: the byte-stream adapter
// (Read/Write) layered over the message transport, plus Close. The concrete
// Conn struct and its methods are defined per-build in conn_linux.go and
// conn_stub.go; this assertion holds for both.
var _ io.ReadWriteCloser = (*Conn)(nil)
