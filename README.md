# gordma

Idiomatic Go bindings for RDMA — a cgo wrapper around **libibverbs** and
**librdmacm** (the user-space libraries from
[rdma-core](https://github.com/linux-rdma/rdma-core)), plus perftest-style
bandwidth/latency example tools modeled on
[perftest](https://github.com/linux-rdma/perftest).

Module path: `github.com/smallnest/gordma`.

## Status

Work in progress. The library exposes the core verbs object model (Device,
Context, PD, MR, CQ, QP, AH, CompChannel) and a set of `cmd/` tools
(`go-send_bw/lat`, `go-write_bw/lat`, `go-read_bw/lat`).

## Requirements

- **Runtime (full functionality):** Linux with RDMA hardware (e.g. Mellanox/NVIDIA
  NICs over RoCE v2), Go 1.22+, and the development headers/libraries:

  ```sh
  # Debian/Ubuntu
  sudo apt-get install libibverbs-dev librdmacm-dev
  ```

- **Other platforms (build only):** the package compiles on macOS and other
  non-Linux targets via build-tag-isolated stubs. RDMA calls return
  `gordma.ErrNotSupported` at runtime instead of crashing, so you can develop
  and run unit tests off-target. Use `gordma.Supported()` to check at runtime.

## Build

```sh
go build ./...   # works on Linux (real) and macOS (stub)
go vet ./...
go test ./...    # hardware-independent unit tests
```

## License

See [LICENSE](LICENSE).
