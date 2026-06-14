# zerocopy

Zero-copy send via a pre-registered `Buffer`: fill `buf.Bytes()` and call
`SendBuffer`, which DMAs the payload straight from registered memory with no
bounce-buffer copy.

```sh
go run . -l 0.0.0.0:18515   # server -> received "zero-copy payload..." 
go run . 192.0.2.1:18515     # client sends a 64-byte registered buffer
```

The buffer sends its full allocated length (64 bytes here, zero-padded after the
payload). Requires RDMA hardware; prints a friendly message on unsupported
platforms.

> Defaults: `-d mlx5_1 -x 3` (first GPU NIC xgbe1, RoCE v2). Use `-d mlx5_0` for the CPU network (xgbe0). See [../README.md](../README.md).
