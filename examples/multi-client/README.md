# multi-client

An RC server that serves many clients in an `Accept` loop (rdma_cm). Each
accepted connection is handled independently — one echoed message — then closed
before the next `Accept`. Run the server once and the client repeatedly.

```sh
go run . -l 0.0.0.0:18515            # server (long-lived, Ctrl-C to stop)

go run . 33.0.226.25:18515 -msg first  # client 1 -> echo: first
go run . 33.0.226.25:18515 -msg second # client 2 -> echo: second
```

Shows the listener's continuous `Accept` and per-connection isolation.

> Defaults: `-d mlx5_1 -x 3` (first GPU NIC xgbe1, RoCE v2). Use `-d mlx5_0` for the CPU network (xgbe0). See [../README.md](../README.md).
