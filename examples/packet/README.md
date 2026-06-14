# packet

UD datagram send/receive via `rdmanet.PacketConn` (`WriteTo`/`ReadFrom`),
analogous to `net.UDPConn`. UD is connectionless, so the client addresses the
server by its `Addr` string (GID%QPN), which the server prints on startup.

```sh
# server — prints its Addr, e.g. fe80:...:0001%0x1a2b
go run .

# client — pass the printed server Addr
go run . 'fe80:0000:0000:0000:0000:0000:0000:0001%0x1a2b'
# server -> got "hello datagram" from QPN ...
```

For automated discovery instead of copy-pasting the Addr, see `../registry`.
Requires RDMA hardware; prints a friendly message on unsupported platforms.

> Defaults: `-d mlx5_1 -x 3` (gajl H20 first GPU NIC xgbe1, RoCE v2). Use `-d mlx5_0` for the CPU network (xgbe0). See [../README.md](../README.md).
