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

Defaults are tuned for the gajl H20 GPU nodes (`show_gids` layout):

- `mlx5_0` → CPU network (`xgbe0`), e.g. `10.214.180.34` / `10.214.180.35`
- `mlx5_1`..`mlx5_8` → GPU network (`xgbe1`..`xgbe8`)
- RoCE v2 lives at **GID index 3** on every device.

Each example defaults to **`-d mlx5_1 -x 3`** (first GPU NIC, RoCE v2). Override
with `-d`/`-x`; use `-d mlx5_0` for the CPU network. Two-node example
(echo-msg over the CPU NIC):

```sh
# node gpu001 (server)
go run ./examples/echo-msg -l 0.0.0.0:18515 -d mlx5_0 -x 3

# node gpu002 (client) — dial gpu001's xgbe0 IP
go run ./examples/echo-msg 10.214.180.34:18515 -d mlx5_0 -x 3
```

Each example takes `-l ADDR` for server mode or a positional `HOST:PORT` for
client mode (see the per-directory README for exact flags).
