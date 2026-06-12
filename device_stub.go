//go:build !linux || !cgo

package gordma

// Device represents an RDMA device. On unsupported platforms it carries only
// the information needed to satisfy the API; all operations return
// ErrNotSupported.
type Device struct {
	info DeviceInfo
}

// GetDeviceList returns ErrNotSupported on platforms without RDMA. The returned
// free function is a no-op.
func GetDeviceList() ([]*Device, func(), error) {
	return nil, func() {}, ErrNotSupported
}

// Info returns the (empty) device information.
func (d *Device) Info() DeviceInfo { return d.info }

// Name returns the device name.
func (d *Device) Name() string { return d.info.Name }

// Open returns ErrNotSupported on platforms without RDMA.
func (d *Device) Open() (*Context, error) { return nil, ErrNotSupported }

// Context is an opened device context. On unsupported platforms it is inert.
type Context struct{}

// Close is a no-op on unsupported platforms.
func (c *Context) Close() error { return nil }

// QueryDevice returns ErrNotSupported.
func (c *Context) QueryDevice() (DeviceAttr, error) { return DeviceAttr{}, ErrNotSupported }

// QueryPort returns ErrNotSupported.
func (c *Context) QueryPort(port int) (PortAttr, error) { return PortAttr{}, ErrNotSupported }

// QueryGID returns ErrNotSupported.
func (c *Context) QueryGID(port, index int) (GID, error) { return GID{}, ErrNotSupported }
