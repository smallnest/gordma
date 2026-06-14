# connection-styles

One program showing every connection-establishment style `rdmanet` offers,
selected with `-mode`:

| `-mode` | Transport | Establishment |
|---------|-----------|---------------|
| `rc-cm` (default) | RC (reliable) | RDMA connection manager (rdma_cm) |
| `rc-handshake` | RC (reliable) | TCP out-of-band handshake (`WithHandshake`) |
| `ud` | UD (datagram) | connectionless — address peers by `Addr` string |

```sh
# RC over rdma_cm (or swap in -mode rc-handshake)
go run . -mode rc-cm -l 0.0.0.0:18515      # server
go run . -mode rc-cm 33.0.226.25:18515       # client -> echo: hello RC

# UD datagrams — server prints its Addr, client sends to it
go run . -mode ud                          # server -> UD server addr: <gid%qpn>
go run . -mode ud '<gid%qpn>'              # client
```

RDMA itself is always the data path; only how the endpoints find each other
differs. (There is no TCP/UDP socket data path here — RDMA does not fall back to
sockets; `rc-handshake` only uses TCP for the out-of-band QP-info exchange.)

> Defaults: `-d mlx5_1 -x 3` (first GPU NIC xgbe1, RoCE v2). Use `-d mlx5_0` for the CPU network (xgbe0). See [../README.md](../README.md).
