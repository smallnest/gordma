# chat

Full-duplex chat: each side runs a **send goroutine** (stdin → peer) and a
**receive loop** (peer → stdout) over the same `Conn` at once. Demonstrates the
concurrency contract (one concurrent reader + one concurrent writer are safe)
and that closing the connection wakes a blocked `RecvMsg` with `io.EOF`.

```sh
go run . -l 0.0.0.0:18515   # server
go run . 192.0.2.1:18515    # client
```

Type lines and press enter to send; Ctrl-D (stdin EOF) closes the connection,
and the peer prints `peer closed`.

> Defaults: `-d mlx5_1 -x 3` (first GPU NIC xgbe1, RoCE v2). Use `-d mlx5_0` for the CPU network (xgbe0). See [../README.md](../README.md).
