# file-transfer

Stream a file from client to server using `Conn` as a plain
`io.ReadWriteCloser` — the client `io.Copy`s the file into the connection, the
server `io.Copy`s the connection into a file. Closing the client connection
sends a FIN so the server's copy terminates on `io.EOF`.

```sh
# server writes everything it receives to received.bin
go run . -l 0.0.0.0:18515 -out received.bin

# client streams payload.bin
go run . 33.0.226.25:18515 -in payload.bin
# -> sent N bytes / received N bytes -> received.bin
```

Shows the stream adapter for bulk data and graceful close → EOF.

> Defaults: `-d mlx5_1 -x 3` (first GPU NIC xgbe1, RoCE v2). Use `-d mlx5_0` for the CPU network (xgbe0). See [../README.md](../README.md).
