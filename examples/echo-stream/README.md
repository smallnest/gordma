# echo-stream

RC byte-stream echo using `rdmanet`'s `io.ReadWriteCloser` adapter
(`Read`/`Write`) layered over the message transport.

```sh
go run . -l 0.0.0.0:18515   # server
go run . 10.0.0.1:18515     # client -> echo: streamed bytes
```

Requires RDMA hardware; prints a friendly message on unsupported platforms.
