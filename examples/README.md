# rdmanet examples

Runnable, self-contained examples for the high-level `rdmanet` API, one feature
per directory. Each is an independent `main` package with its own README.

### Core features

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

### Scenarios

| Directory | Scenario |
|-----------|----------|
| [file-transfer](file-transfer/) | Stream a file with `io.Copy` over `Conn`; graceful EOF |
| [large-message](large-message/) | Multi-MiB message: fragmentation/reassembly + sizing tuning |
| [rpc](rpc/) | Request/response loop (`RecvMsg`→handle→`SendMsg`) |
| [chat](chat/) | Full-duplex: concurrent send + receive goroutines |
| [connection-styles](connection-styles/) | rc-cm vs rc-handshake vs ud, one `-mode` flag |
| [multi-client](multi-client/) | RC server `Accept` loop serving many clients |
| [ud-broadcast](ud-broadcast/) | One UD sender → many registry-discovered peers (AH cache) |
| [bench-throughput](bench-throughput/) | `SendMsg` vs `SendBatch` Gb/s self-test |
| [tuning](tuning/) | Every `Option` with a flag + guidance |

All examples build on every platform (`go build ./examples/...`). They require
RDMA hardware to run; on unsupported platforms they print
`RDMA not supported on this platform` and exit cleanly.

## Default device / GID

Defaults assume a typical multi-NIC RoCE host (`show_gids` layout):

- `mlx5_0` → CPU network (`xgbe0`)
- `mlx5_1`..`mlx5_8` → GPU network (`xgbe1`..`xgbe8`)
- RoCE v2 lives at **GID index 3** on every device.

Each example defaults to **`-d mlx5_1 -x 3`** (first GPU NIC, RoCE v2). Override
with `-d`/`-x`; use `-d mlx5_0` for the CPU network.

The two-node sample IPs used throughout the per-example READMEs are:

| Node | CPU net (`mlx5_0` / xgbe0) | GPU net (`mlx5_1` / xgbe1) |
|------|----------------------------|----------------------------|
| node1 (server) | `10.214.180.34` | `33.0.226.25` |
| node2 (client) | `10.214.180.35` | `33.0.226.27` |

Default examples use `mlx5_1`, so the client dials node1's GPU IP
`33.0.226.25`. Two-node example over the **GPU** NIC (the default):

```sh
# node1 (server)
go run ./examples/echo-msg -l 0.0.0.0:18515

# node2 (client) — dial node1's xgbe1 (GPU) IP
go run ./examples/echo-msg 33.0.226.25:18515
```

Over the **CPU** NIC instead (`-d mlx5_0`, dial node1's xgbe0 IP):

```sh
go run ./examples/echo-msg -l 0.0.0.0:18515 -d mlx5_0
go run ./examples/echo-msg 10.214.180.34:18515 -d mlx5_0
```

Each example takes `-l ADDR` for server mode or a positional `HOST:PORT` for
client mode (see the per-directory README for exact flags).
