# pollmode

Selecting the CQ completion-draining strategy via
`WithPollMode(PollBusy|PollEvent)`:

- `event` (default): block on a completion channel — low CPU, slightly higher
  latency. Best for many connections / idle-friendly servers.
- `busy`: dedicate a goroutine to spin-polling — lowest latency, burns a core.

```sh
go run . -l 0.0.0.0:18515 --poll=busy    # server, busy-poll
go run . 33.0.226.25:18515 --poll=event     # client, event-driven -> echo: ping
```

Both modes are functionally identical; only the latency/CPU profile differs.
Requires RDMA hardware; prints a friendly message on unsupported platforms.

> Defaults: `-d mlx5_1 -x 3` (first GPU NIC xgbe1, RoCE v2). Use `-d mlx5_0` for the CPU network (xgbe0). See [../README.md](../README.md).
