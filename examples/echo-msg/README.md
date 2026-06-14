# echo-msg

Minimal RC message-semantics echo over `rdmanet` (`SendMsg`/`RecvMsg`).

```sh
# server (node A)
go run . -l 0.0.0.0:18515

# client (node B)
go run . 33.0.226.25:18515
# -> echo: hello rdmanet
```

Requires RDMA hardware. On unsupported platforms it prints
`RDMA not supported on this platform` and exits cleanly.

> Defaults: `-d mlx5_1 -x 3` (first GPU NIC xgbe1, RoCE v2). Use `-d mlx5_0` for the CPU network (xgbe0). See [../README.md](../README.md).
