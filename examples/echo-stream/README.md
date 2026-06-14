# echo-stream

RC byte-stream echo using `rdmanet`'s `io.ReadWriteCloser` adapter
(`Read`/`Write`) layered over the message transport.

```sh
go run . -l 0.0.0.0:18515   # server
go run . 10.214.180.34:18515     # client -> echo: streamed bytes
```

Requires RDMA hardware; prints a friendly message on unsupported platforms.

> Defaults: `-d mlx5_1 -x 3` (gajl H20 first GPU NIC xgbe1, RoCE v2). Use `-d mlx5_0` for the CPU network (xgbe0). See [../README.md](../README.md).
