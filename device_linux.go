//go:build linux && cgo

package gordma

/*
#include <stdlib.h>
#include <string.h>
#include <infiniband/verbs.h>

// mtu_to_bytes converts an enum ibv_mtu value to a byte count.
static int mtu_to_bytes(enum ibv_mtu m) {
	switch (m) {
	case IBV_MTU_256:  return 256;
	case IBV_MTU_512:  return 512;
	case IBV_MTU_1024: return 1024;
	case IBV_MTU_2048: return 2048;
	case IBV_MTU_4096: return 4096;
	default: return 0;
	}
}

// gordma_query_port wraps ibv_query_port so the static-inline in modern
// rdma-core's verbs.h (which forwards to ___ibv_query_port expecting a
// _compat_ibv_port_attr*) is expanded here in C. Calling ibv_query_port
// directly from cgo binds the underlying symbol and trips a type mismatch.
static int gordma_query_port(struct ibv_context *ctx, uint8_t port,
                             struct ibv_port_attr *attr) {
	return ibv_query_port(ctx, port, attr);
}
*/
import "C"

import (
	"fmt"
	"unsafe"
)

// Device represents an available RDMA device that can be opened.
type Device struct {
	dev  *C.struct_ibv_device
	info DeviceInfo
}

// list keeps the C device list alive so we can free it on FreeDeviceList.
type deviceList struct {
	arr **C.struct_ibv_device
}

// GetDeviceList returns the RDMA devices present on the system. The returned
// devices remain valid until FreeDeviceList is called. If no devices are
// found it returns an empty slice and a nil error.
func GetDeviceList() ([]*Device, func(), error) {
	var num C.int
	arr := C.ibv_get_device_list(&num)
	if arr == nil {
		return nil, func() {}, fmt.Errorf("gordma: ibv_get_device_list failed: %w", lastErrno())
	}
	dl := &deviceList{arr: arr}
	free := func() { C.ibv_free_device_list(dl.arr) }

	n := int(num)
	devs := make([]*Device, 0, n)
	// arr is a NULL-terminated array of *ibv_device. Walk it with unsafe.Add
	// so the pointer arithmetic stays in unsafe.Pointer form (vet rejects
	// round-tripping through a stored uintptr).
	ptrSize := unsafe.Sizeof(uintptr(0))
	for i := 0; i < n; i++ {
		dp := *(**C.struct_ibv_device)(unsafe.Add(unsafe.Pointer(arr), uintptr(i)*ptrSize))
		if dp == nil {
			break
		}
		d := &Device{
			dev: dp,
			info: DeviceInfo{
				Name: C.GoString(C.ibv_get_device_name(dp)),
				GUID: uint64(C.ibv_get_device_guid(dp)),
			},
		}
		devs = append(devs, d)
	}
	return devs, free, nil
}

// Info returns the static information discovered for this device.
func (d *Device) Info() DeviceInfo { return d.info }

// Name returns the device name (e.g. "mlx5_0").
func (d *Device) Name() string { return d.info.Name }

// Context is an opened RDMA device context. It is the root from which PDs,
// CQs and QPs are created. Close releases the underlying ibv_context.
type Context struct {
	ctx *C.struct_ibv_context
	dev *Device
}

// Open opens this device and returns a Context. The caller must Close it.
func (d *Device) Open() (*Context, error) {
	ctx := C.ibv_open_device(d.dev)
	if ctx == nil {
		return nil, fmt.Errorf("gordma: ibv_open_device(%s) failed: %w", d.info.Name, lastErrno())
	}
	c := &Context{ctx: ctx, dev: d}
	// Populate NumPorts as a side effect; ignore errors here since Open
	// succeeding is what matters and callers can QueryDevice explicitly.
	_, _ = c.QueryDevice()
	return c, nil
}

// Close releases the device context. It is idempotent.
func (c *Context) Close() error {
	if c == nil || c.ctx == nil {
		return nil
	}
	if rc := C.ibv_close_device(c.ctx); rc != 0 {
		return fmt.Errorf("gordma: ibv_close_device failed: %w", lastErrno())
	}
	c.ctx = nil
	return nil
}

// QueryDevice returns device-wide attributes.
func (c *Context) QueryDevice() (DeviceAttr, error) {
	if c == nil || c.ctx == nil {
		return DeviceAttr{}, ErrClosed
	}
	var a C.struct_ibv_device_attr
	if rc := C.ibv_query_device(c.ctx, &a); rc != 0 {
		return DeviceAttr{}, fmt.Errorf("gordma: ibv_query_device failed: %w", errnoFromRC(rc))
	}
	c.dev.info.NumPorts = int(a.phys_port_cnt)
	return DeviceAttr{
		FirmwareVersion: C.GoString(&a.fw_ver[0]),
		NodeGUID:        uint64(a.node_guid),
		SysImageGUID:    uint64(a.sys_image_guid),
		MaxQP:           int(a.max_qp),
		MaxQPWR:         int(a.max_qp_wr),
		MaxCQ:           int(a.max_cq),
		MaxCQE:          int(a.max_cqe),
		MaxMR:           int(a.max_mr),
		MaxPD:           int(a.max_pd),
		MaxSGE:          int(a.max_sge),
		VendorID:        uint32(a.vendor_id),
		VendorPartID:    uint32(a.vendor_part_id),
	}, nil
}

// QueryPort returns attributes for the given 1-based port number.
func (c *Context) QueryPort(port int) (PortAttr, error) {
	if c == nil || c.ctx == nil {
		return PortAttr{}, ErrClosed
	}
	var a C.struct_ibv_port_attr
	if rc := C.gordma_query_port(c.ctx, C.uint8_t(port), &a); rc != 0 {
		return PortAttr{}, fmt.Errorf("gordma: ibv_query_port(%d) failed: %w", port, errnoFromRC(rc))
	}
	link := "Unknown"
	switch a.link_layer {
	case C.IBV_LINK_LAYER_ETHERNET:
		link = "Ethernet"
	case C.IBV_LINK_LAYER_INFINIBAND:
		link = "InfiniBand"
	}
	return PortAttr{
		State:       PortState(a.state),
		MaxMTU:      int(C.mtu_to_bytes(a.max_mtu)),
		ActiveMTU:   int(C.mtu_to_bytes(a.active_mtu)),
		LID:         uint16(a.lid),
		GIDTableLen: int(a.gid_tbl_len),
		LinkLayer:   link,
	}, nil
}

// QueryGID returns the GID at the given index on the given 1-based port.
func (c *Context) QueryGID(port, index int) (GID, error) {
	if c == nil || c.ctx == nil {
		return GID{}, ErrClosed
	}
	var cg C.union_ibv_gid
	if rc := C.ibv_query_gid(c.ctx, C.uint8_t(port), C.int(index), &cg); rc != 0 {
		return GID{}, fmt.Errorf("gordma: ibv_query_gid(port=%d,index=%d) failed: %w", port, index, errnoFromRC(rc))
	}
	var g GID
	C.memcpy(unsafe.Pointer(&g[0]), unsafe.Pointer(&cg), C.size_t(len(g)))
	return g, nil
}
