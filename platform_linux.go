//go:build linux && cgo

package gordma

/*
#cgo LDFLAGS: -libverbs -lrdmacm
#include <infiniband/verbs.h>
#include <rdma/rdma_cma.h>
*/
import "C"

// supported is true on Linux builds with cgo, where libibverbs and librdmacm
// are linked. The cgo preamble above carries the linker flags
// (-libverbs -lrdmacm) for the package's Linux files.
const supported = true
