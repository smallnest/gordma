package gordma

import "fmt"

// GID is a 16-byte RoCE/InfiniBand global identifier.
type GID [16]byte

// String renders the GID in the conventional colon-separated 8x16-bit form.
func (g GID) String() string {
	return fmt.Sprintf("%02x%02x:%02x%02x:%02x%02x:%02x%02x:%02x%02x:%02x%02x:%02x%02x:%02x%02x",
		g[0], g[1], g[2], g[3], g[4], g[5], g[6], g[7],
		g[8], g[9], g[10], g[11], g[12], g[13], g[14], g[15])
}

// DeviceInfo describes an RDMA device discovered via GetDeviceList.
type DeviceInfo struct {
	// Name is the device name, e.g. "mlx5_0".
	Name string
	// GUID is the node GUID in host byte order.
	GUID uint64
	// NumPorts is the number of physical ports on the device.
	NumPorts int
}

// DeviceAttr holds device-wide capability attributes returned by
// Context.QueryDevice.
type DeviceAttr struct {
	FirmwareVersion string
	NodeGUID        uint64
	SysImageGUID    uint64
	MaxQP           int
	MaxQPWR         int
	MaxCQ           int
	MaxCQE          int
	MaxMR           int
	MaxPD           int
	MaxSGE          int
	VendorID        uint32
	VendorPartID    uint32
}

// PortState mirrors enum ibv_port_state.
type PortState int

const (
	PortNop       PortState = 0
	PortDown      PortState = 1
	PortInit      PortState = 2
	PortArmed     PortState = 3
	PortActive    PortState = 4
	PortActiveDef PortState = 5
)

// String returns a human-readable port state.
func (s PortState) String() string {
	switch s {
	case PortNop:
		return "NOP"
	case PortDown:
		return "DOWN"
	case PortInit:
		return "INIT"
	case PortArmed:
		return "ARMED"
	case PortActive:
		return "ACTIVE"
	case PortActiveDef:
		return "ACTIVE_DEFER"
	default:
		return fmt.Sprintf("PortState(%d)", int(s))
	}
}

// PortAttr holds per-port attributes returned by Context.QueryPort.
type PortAttr struct {
	State PortState
	// MaxMTU and ActiveMTU are expressed in bytes (e.g. 1024, 4096).
	MaxMTU      int
	ActiveMTU   int
	LID         uint16
	GIDTableLen int
	// LinkLayer is "Ethernet" (RoCE) or "InfiniBand".
	LinkLayer string
}
