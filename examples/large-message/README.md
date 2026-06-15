# large-message

Send a single multi-megabyte message and verify it arrives intact. One
`SendMsg` is transparently fragmented into many frames under credit flow
control; one `RecvMsg` reassembles them back into a single message with its
boundary preserved. Both sides print the SHA-256 so you can confirm integrity.

```sh
go run . -l 0.0.0.0:18515              # server -> received N bytes, sha256=...
go run . 33.0.226.25:18515 --size 16      # client sends a 16 MiB message
```

Tune the per-frame bounce buffer and queue depth with `--buf` (KiB) and
`--depth`. Shows fragmentation/reassembly (#36) and sizing options.

> Defaults: `-d mlx5_1 -x 3` (first GPU NIC xgbe1, RoCE v2). Use `-d mlx5_0` for the CPU network (xgbe0). See [../README.md](../README.md).
