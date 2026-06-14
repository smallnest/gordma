# rdmanet examples

Runnable, self-contained examples for the high-level `rdmanet` API, one feature
per directory. Each is an independent `main` package with its own README.

| Directory | Feature |
|-----------|---------|
| [echo-msg](echo-msg/) | RC message semantics (`SendMsg`/`RecvMsg`) |
| [echo-stream](echo-stream/) | Byte-stream adapter (`Read`/`Write`, `io.ReadWriteCloser`) |
| [handshake-dial](handshake-dial/) | TCP out-of-band handshake establishment (`WithHandshake`) |
| [packet](packet/) | UD datagrams (`PacketConn`, `WriteTo`/`ReadFrom`) |
| [batch](batch/) | Amortized I/O (`SendBatch`/`RecvBatch`) |
| [zerocopy](zerocopy/) | Zero-copy send (`AllocBuffer`/`SendBuffer`) |
| [registry](registry/) | Out-of-band UD address discovery (`NewRegistry`/`LookupAddr`) |
| [pollmode](pollmode/) | CQ poll mode selection (`WithPollMode`) |

All examples build on every platform (`go build ./examples/...`). They require
RDMA hardware to run; on unsupported platforms they print
`RDMA not supported on this platform` and exit cleanly.

Each example takes `-l ADDR` for server mode or a positional `HOST:PORT` for
client mode (see the per-directory README for exact flags).
