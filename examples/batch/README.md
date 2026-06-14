# batch

Amortized message I/O via `SendBatch`/`RecvBatch` — post/collect many messages
per call instead of one at a time.

```sh
go run . -l 0.0.0.0:18515   # server -> received 8 messages
go run . 10.0.0.1:18515     # client sends a batch of 8
```

Requires RDMA hardware; prints a friendly message on unsupported platforms.
