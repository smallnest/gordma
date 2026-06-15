# rpc

Minimal request/response over RC: the client sends a request and blocks for the
reply; the server loops `RecvMsg → handle → SendMsg`. The handler here just
uppercases the request. When the client closes, the server's `RecvMsg` returns
`io.EOF` and it exits.

```sh
go run . -l 0.0.0.0:18515                  # server
go run . 33.0.226.25:18515 --req hello -n 3   # client
# -> reply: HELLO-0
#    reply: HELLO-1
#    reply: HELLO-2
```

Shows the one-question-one-answer pattern and a long-lived server loop.

> Defaults: `-d mlx5_1 -x 3` (first GPU NIC xgbe1, RoCE v2). Use `-d mlx5_0` for the CPU network (xgbe0). See [../README.md](../README.md).
