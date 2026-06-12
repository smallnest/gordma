package gordma

// GRHLength is the size in bytes of the Global Routing Header that prefixes
// every datagram received on a UD QP. Receivers must allocate this many extra
// bytes at the front of their receive buffer and skip past it to reach the
// payload.
const GRHLength = 40

// UDConnParams carries the information needed to address a remote UD endpoint.
// UD is connectionless, so unlike RC there is no per-peer connection: each send
// targets a remote QPN + QKey through an AddressHandle.
type UDConnParams struct {
	// QKey must match on both ends for datagrams to be accepted.
	QKey uint32
	// PortNum is the local 1-based port number.
	PortNum int
}

// AHAttr describes how to reach a remote UD endpoint, used to build an
// AddressHandle. RoCE uses the global route (GID); InfiniBand uses the DLID.
type AHAttr struct {
	// IsRoCE selects GID-based addressing over LID-based.
	IsRoCE bool
	// DestLID is the remote LID (InfiniBand).
	DestLID uint16
	// DestGID is the remote GID (RoCE).
	DestGID GID
	// SGIDIndex is the local GID table index (RoCE).
	SGIDIndex int
	// HopLimit for the global route header (RoCE).
	HopLimit uint8
	// PortNum is the local 1-based port number.
	PortNum int
}
