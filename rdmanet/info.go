package rdmanet

// ConnInfo describes an established connection for perftest-style reporting
// (mirroring the header ib_send_bw prints). It is returned by Conn.Info and
// RawConn.Info after the connection is up. Fields that a given connection
// method cannot supply are left zero (see the per-method notes on Info).
type ConnInfo struct {
	// Device is the resolved RDMA device name (e.g. "mlx5_1"). Empty when the
	// connection was made via rdma_cm, which selects the device internally.
	Device string
	// LinkLayer is "Ethernet" (RoCE) or "InfiniBand"; empty on the rdma_cm path.
	LinkLayer string
	// MTU is the active path MTU in bytes; zero on the rdma_cm path.
	MTU int
	// GIDIndex is the local GID table index used for bring-up.
	GIDIndex int
	// Local and Remote are the two endpoints' RC addressing.
	Local, Remote EndpointAddr
}

// EndpointAddr is one endpoint's RC addressing — the LID/QPN/PSN/GID quartet
// ib_send_bw prints for the local and remote address. (It is distinct from the
// UD-oriented Addr type.) On the rdma_cm path only QPN is populated (PSN/LID/
// GID are negotiated inside librdmacm).
type EndpointAddr struct {
	LID uint16
	QPN uint32
	PSN uint32
	GID [16]byte
}
