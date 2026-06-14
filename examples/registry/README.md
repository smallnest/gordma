# registry

Out-of-band UD address discovery via the optional `rdmanet` registry: a UD
endpoint registers its `Addr` under a name, peers look it up — no manual Addr
copy-paste.

```sh
# 1. registry server (pure Go, no RDMA needed)
go run . -registry 0.0.0.0:9100

# 2. UD server registers under "nodeA"
go run . -r 127.0.0.1:9100 -name nodeA

# 3. client looks up "nodeA" and sends to it
go run . -r 127.0.0.1:9100 -name nodeA -send
# nodeA -> got "hello via registry"
```

The registry is line-delimited JSON over TCP (same spirit as the `handshake`
package). RDMA endpoints require hardware; the registry server itself does not.
