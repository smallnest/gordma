package perftest

import "errors"

// errNoPeer is returned when an RDMA Write/Read run is attempted without peer
// endpoint info (RKey/RemoteAddr), which only the TCP-handshake path provides.
var errNoPeer = errors.New("perftest: missing peer endpoint info (RKey/RemoteAddr); RDMA Write/Read requires the TCP handshake path")

// errBufferTooSmall is returned when the registered buffer cannot hold the
// configured message size.
var errBufferTooSmall = errors.New("perftest: registered buffer smaller than message size")
