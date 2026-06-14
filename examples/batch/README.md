# batch

Amortized message I/O via `SendBatch`/`RecvBatch` — post/collect many messages
per call instead of one at a time.

```sh
go run . -l 0.0.0.0:18515   # server -> received 8 messages
go run . 33.0.226.25:18515     # client sends a batch of 8
```

Requires RDMA hardware; prints a friendly message on unsupported platforms.

> Defaults: `-d mlx5_1 -x 3` (first GPU NIC xgbe1, RoCE v2). Use `-d mlx5_0` for the CPU network (xgbe0). See [../README.md](../README.md).
