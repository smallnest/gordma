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

## Default device / GID

Defaults assume a typical multi-NIC RoCE host (`show_gids` layout):

- `mlx5_0` → CPU network (`xgbe0`)
- `mlx5_1`..`mlx5_8` → GPU network (`xgbe1`..`xgbe8`)
- RoCE v2 lives at **GID index 3** on every device.

Each example defaults to **`-d mlx5_1 -x 3`** (first GPU NIC, RoCE v2). Override
with `-d`/`-x`; use `-d mlx5_0` for the CPU network. Two-node example
(echo-msg over the CPU NIC):

```sh
# node A (server)
go run ./examples/echo-msg -l 0.0.0.0:18515 -d mlx5_0 -x 3

# node B (client) — dial node A's xgbe0 IP
go run ./examples/echo-msg 192.0.2.1:18515 -d mlx5_0 -x 3
```

Each example takes `-l ADDR` for server mode or a positional `HOST:PORT` for
client mode (see the per-directory README for exact flags).
