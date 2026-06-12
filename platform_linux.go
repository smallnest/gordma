//go:build linux && cgo

package gordma

/*
#cgo LDFLAGS: -libverbs -lrdmacm
#include <infiniband/verbs.h>
#include <rdma/rdma_cma.h>
*/
import "C"

// supported is true on Linux builds with cgo, where libibverbs and librdmacm
// are linked. The blank reference to C keeps the cgo preamble live so that the
// linker flags are honored even before any verbs call exists.
const supported = true

// cgoVerbsLinked is referenced to ensure the cgo import is not pruned by the
// compiler while the wrapper is still being built up issue by issue.
var _ = C.IBV_QPT_RC
