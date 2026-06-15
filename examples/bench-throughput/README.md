# bench-throughput

A tiny self-contained throughput comparison: the client sends N messages twice —
once one-at-a-time (`SendMsg`) and once batched (`SendBatch`) — and reports
Gb/s and Mmsg/s for each, so you can see the amortization benefit of batching.
The server drains both passes with `RecvBatch`.

```sh
go run . -l 0.0.0.0:18515                     # server
go run . 33.0.226.25:18515 -n 100000 -s 1024 -b 32
# -> SendMsg  : 100000 x 1024 B in ... -> X Gb/s, Y Mmsg/s
#    SendBatch: 100000 x 1024 B in ... -> X' Gb/s, Y' Mmsg/s
```

For a fuller benchmark with latency percentiles use the `cmd/go_rdmanet_bw` and
`cmd/go_rdmanet_lat` tools.

> Defaults: `-d mlx5_1 -x 3` (first GPU NIC xgbe1, RoCE v2). Use `-d mlx5_0` for the CPU network (xgbe0). See [../README.md](../README.md).
