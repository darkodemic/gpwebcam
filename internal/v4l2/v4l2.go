// Package v4l2 checks that the output device is a v4l2loopback device the
// current user can write to, before ffmpeg is started.
package v4l2

import (
	"bytes"
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

// MaxDeviceNumber bounds the /dev/videoN number gw accepts.
const MaxDeviceNumber = 255

// LoopbackDriver is the driver name v4l2loopback reports in VIDIOC_QUERYCAP.
const LoopbackDriver = "v4l2 loopback"

// DevicePath returns /dev/videoN for n, or an error if n is out of range.
func DevicePath(n int) (string, error) {
	if n < 0 || n > MaxDeviceNumber {
		return "", fmt.Errorf("video device number %d: must be 0 to %d", n, MaxDeviceNumber)
	}
	return fmt.Sprintf("/dev/video%d", n), nil
}

// capability mirrors struct v4l2_capability from linux/videodev2.h.
type capability struct {
	Driver       [16]byte
	Card         [32]byte
	BusInfo      [32]byte
	Version      uint32
	Capabilities uint32
	DeviceCaps   uint32
	Reserved     [3]uint32
}

// vidiocQuerycap is _IOR('V', 0, struct v4l2_capability).
const vidiocQuerycap = 2<<30 | uint32(unsafe.Sizeof(capability{}))<<16 | 'V'<<8 | 0

// Info is what VIDIOC_QUERYCAP reports about a device.
type Info struct {
	Driver string
	Card   string // card_label of the v4l2loopback device
}

// CheckLoopback opens path for writing, as ffmpeg will, and verifies that
// the driver behind it is v4l2loopback.
func CheckLoopback(path string) (Info, error) {
	f, err := os.OpenFile(path, os.O_WRONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return Info{}, fmt.Errorf("%w (is v4l2loopback loaded with video_nr set, and may this user write to it?)", err)
	}
	defer f.Close()

	var c capability
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), uintptr(vidiocQuerycap), uintptr(unsafe.Pointer(&c))); errno != 0 {
		return Info{}, fmt.Errorf("%s: VIDIOC_QUERYCAP: %w", path, errno)
	}
	info := Info{Driver: cstr(c.Driver[:]), Card: cstr(c.Card[:])}
	if info.Driver != LoopbackDriver {
		return info, fmt.Errorf("%s is driven by %q, not v4l2loopback", path, info.Driver)
	}
	return info, nil
}

func cstr(b []byte) string {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		b = b[:i]
	}
	return string(b)
}
