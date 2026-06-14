# ud-broadcast

One UD sender fans a datagram out to several receivers discovered through the
registry. The sender looks up each name and `WriteTo`s it; the `PacketConn`
caches an `AddressHandle` per destination, so repeated sends to the same peer
reuse it.

```sh
# 1. registry (pure Go, no RDMA needed)
go run . -registry 0.0.0.0:9100

# 2. several receivers, each under a distinct name
go run . -r 10.214.180.34:9100 -name node1
go run . -r 10.214.180.34:9100 -name node2
go run . -r 10.214.180.34:9100 -name node3

# 3. sender fans out to all of them
go run . -r 10.214.180.34:9100 -to node1,node2,node3 -send
```

Shows UD's connectionless one-to-many, per-destination AH caching, and registry
discovery of multiple peers.

> Defaults: `-d mlx5_1 -x 3` (first GPU NIC xgbe1, RoCE v2). Use `-d mlx5_0` for the CPU network (xgbe0). See [../README.md](../README.md).
