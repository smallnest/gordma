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

- **Other platforms (build only):** the package compiles on macOS, Windows and
  other non-Linux targets, and on Linux with `CGO_ENABLED=0`, via build-tag
  isolated stubs (`*_stub.go`, guarded by `//go:build !linux || !cgo`). RDMA
  calls return `gordma.ErrNotSupported` at runtime instead of crashing, so you
  can develop and run unit tests off-target. Use `gordma.Supported()` to check
  at runtime. The exported API is identical across both builds.

  ```sh
  GOOS=darwin  GOARCH=arm64 go build ./...   # stub
  GOOS=windows GOARCH=amd64 go build ./...   # stub
  CGO_ENABLED=0 go build ./...               # stub (even on Linux)
  ```

## Build

```sh
go build ./...   # works on Linux (real) and macOS (stub)
go vet ./...
go test ./...    # hardware-independent unit tests
```

## Quick start (library)

```go
package main

import (
	"log"

	"github.com/smallnest/gordma"
)

func main() {
	if !gordma.Supported() {
		log.Fatal("RDMA not supported on this platform")
	}

	devs, free, err := gordma.GetDeviceList()
	if err != nil {
		log.Fatal(err)
	}
	defer free()
	if len(devs) == 0 {
		log.Fatal("no RDMA devices found")
	}

	ctx, err := devs[0].Open()
	if err != nil {
		log.Fatal(err)
	}
	defer ctx.Close()

	pd, err := ctx.AllocPD()
	if err != nil {
		log.Fatal(err)
	}
	defer pd.Close()

	// Register a pinned, GC-safe buffer, create a CQ and an RC QP, then
	// connect via the TCP handshake (see handshake package) or rdma_cm
	// (gordma.Dial / gordma.Listen). See the cmd/ tools for full examples.
}
```

Two connection styles are available:

- **TCP out-of-band handshake** (perftest default): `handshake.Listen`/`Dial`
  exchange QPN/PSN/GID/LID/RKey/addr, then you drive
  `QP.ModifyToInit/RTR/RTS` yourself.
- **rdma_cm** (`-R`): `gordma.Listen`/`gordma.Dial` return a `CMConn` whose QP
  is already in RTS.

## Tools

Six perftest-style tools live under `cmd/` and mirror their C counterparts.
Each runs as a **server** (no peer address) or **client** (peer address):

| Tool | Mirrors | Transport | Connect |
|------|---------|-----------|---------|
| `go-send_bw`  | `ib_send_bw`  | RC, UD | TCP or rdma_cm |
| `go-send_lat` | `ib_send_lat` | RC, UD | TCP or rdma_cm |
| `go-write_bw` | `ib_write_bw` | RC     | TCP |
| `go-write_lat`| `ib_write_lat`| RC     | TCP |
| `go-read_bw`  | `ib_read_bw`  | RC     | TCP |
| `go-read_lat` | `ib_read_lat` | RC     | TCP |

Common flags: `-s` size, `-n` iterations, `-d` device, `-i` HCA port,
`-p` TCP handshake port, `-c RC|UD`, `-R` (use rdma_cm), `-t` tx-depth
(default 128), `-x` GID index, `--output=histogram` (latency tools).

```sh
go build -o bin/ ./cmd/...

# Server, then client (RoCE v2: pick the RoCE v2 GID index with -x):
./bin/go-send_bw -s 65536 -n 5000 -x 3
./bin/go-send_bw -s 65536 -n 5000 -x 3 <server-ip>:18515

# Latency with a full histogram:
./bin/go-send_lat --output=histogram <server-ip>:18515
```

## Testing & CI

CI (`.github/workflows/ci.yml`) runs on Linux: `go vet`, `go build` (both cgo
and `CGO_ENABLED=0` stub), and `go test -race` for the hardware-independent
unit tests, plus cross-compilation of the stub for darwin/windows.

Hardware-dependent integration tests are isolated and require an RDMA NIC; run
them manually on a machine with RDMA devices (or a dedicated runner). The unit
tests in this repo never touch hardware and pass on any platform.

## License

See [LICENSE](LICENSE).
