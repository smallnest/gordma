# echo-msg

Minimal RC message-semantics echo over `rdmanet` (`SendMsg`/`RecvMsg`).

```sh
# server (node A)
go run . -l 0.0.0.0:18515

# client (node B)
go run . 10.0.0.1:18515
# -> echo: hello rdmanet
```

Requires RDMA hardware. On unsupported platforms it prints
`RDMA not supported on this platform` and exits cleanly.
