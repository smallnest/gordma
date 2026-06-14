# handshake-dial

RC connection established via the TCP out-of-band handshake
(`rdmanet.WithHandshake()`) rather than rdma_cm — useful where rdma_cm is
unavailable or for perftest-style interop.

```sh
go run . -l 0.0.0.0:18515   # server
go run . 10.0.0.1:18515     # client -> echo: via tcp handshake
```

Requires RDMA hardware; prints a friendly message on unsupported platforms.
