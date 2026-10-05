// Package v4l2 checks that the output device is a v4l2loopback device the
// current user can write to, before ffmpeg is started.
package v4l2

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

// MaxDeviceNumber bounds the /dev/videoN number gpwebcam accepts.
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

// DefaultLabel is the card_label gpwebcam's module configuration gives its
// loopback device.
const DefaultLabel = "GoPro"

// FindByLabel returns /dev/videoN for the one video4linux device whose name,
// the card_label of a v4l2loopback device, is label. sysfs is the sysfs
// mount point, normally "/sys".
func FindByLabel(sysfs, label string) (string, error) {
	dir := filepath.Join(sysfs, "class", "video4linux")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("list video devices: %w", err)
	}
	var found []string
	for _, e := range entries {
		n, err := strconv.Atoi(strings.TrimPrefix(e.Name(), "video"))
		if err != nil || !strings.HasPrefix(e.Name(), "video") || n < 0 || n > MaxDeviceNumber {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name(), "name"))
		if err != nil || strings.TrimSpace(string(b)) != label {
			continue
		}
		found = append(found, fmt.Sprintf("/dev/video%d", n))
	}
	switch len(found) {
	case 0:
		return "", fmt.Errorf("no video device named %q; is v4l2loopback loaded with card_label=%s (see /usr/lib/modprobe.d/99-gpwebcam.conf), or choose one with -video-nr?", label, label)
	case 1:
		return found[0], nil
	default:
		return "", fmt.Errorf("several video devices named %q (%s); choose one with -video-nr", label, strings.Join(found, ", "))
	}
}
