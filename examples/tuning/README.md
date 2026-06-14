# tuning

Every `rdmanet` option in one place, with a flag per option and a printed
resolved config, plus a small echo so you can experiment.

| Flag | Option | When it matters |
|------|--------|-----------------|
| `-d` | `WithDevice` | pick the NIC (CPU vs GPU network) |
| `-i` | `WithPort` | HCA port number (usually 1) |
| `-x` | `WithGIDIndex` | RoCE v2 GID index (3 here) |
| `-depth` | `WithQueueDepth` | outstanding WRs / flow-control credits — raise for throughput |
| `-buf` | `WithBufferSize` | per-frame bounce slot (KiB) — bigger frames, fewer fragments |
| `-poll` | `WithPollMode` | `event` (low CPU) vs `busy` (low latency) |
| `-handshake` | `WithHandshake` | TCP out-of-band establishment instead of rdma_cm |

```sh
go run . -l 0.0.0.0:18515 -depth 256 -buf 256 -poll busy
go run . 33.0.226.25:18515 -depth 256 -buf 256 -poll busy
# config: device=mlx5_1 port=1 gidIndex=3 depth=256 buf=256KiB poll=busy handshake=false
# echo: tuned
```

Memory note: the RC data path registers `2 × depth × buf` bytes of bounce
buffers per connection (e.g. 256 × 256 KiB × 2 ≈ 128 MiB).

> Defaults: `-d mlx5_1 -x 3` (first GPU NIC xgbe1, RoCE v2). Use `-d mlx5_0` for the CPU network (xgbe0). See [../README.md](../README.md).
