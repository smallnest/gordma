# handshake-dial

RC connection established via the TCP out-of-band handshake
(`rdmanet.WithHandshake()`) rather than rdma_cm — useful where rdma_cm is
unavailable or for perftest-style interop.

```sh
go run . -l 0.0.0.0:18515   # server
go run . 192.0.2.1:18515     # client -> echo: via tcp handshake
```

Requires RDMA hardware; prints a friendly message on unsupported platforms.

> Defaults: `-d mlx5_1 -x 3` (first GPU NIC xgbe1, RoCE v2). Use `-d mlx5_0` for the CPU network (xgbe0). See [../README.md](../README.md).
