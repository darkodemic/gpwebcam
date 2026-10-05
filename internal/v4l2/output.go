package v4l2

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

// pixFormat mirrors struct v4l2_pix_format from linux/videodev2.h.
type pixFormat struct {
	Width        uint32
	Height       uint32
	PixelFormat  uint32
	Field        uint32
	BytesPerLine uint32
	SizeImage    uint32
	Colorspace   uint32
	Priv         uint32
	Flags        uint32
	YCbCrEnc     uint32
	Quantization uint32
	XferFunc     uint32
}

// format mirrors struct v4l2_format. The union after Type is 8-byte aligned
// and 200 bytes long on 64-bit Linux (208 bytes in total, checked against
// the kernel headers).
type format struct {
	Type uint32
	_    uint32
	Pix  pixFormat
	_    [200 - unsafe.Sizeof(pixFormat{})]byte
}

const (
	// vidiocSFmt is _IOWR('V', 5, struct v4l2_format).
	vidiocSFmt = 3<<30 | uint32(unsafe.Sizeof(format{}))<<16 | 'V'<<8 | 5

	bufTypeVideoOutput = 2
	fieldNone          = 1
	// pixFmtYU12 is V4L2_PIX_FMT_YUV420: planar YUV 4:2:0, ffmpeg's yuv420p.
	pixFmtYU12 = 'Y' | 'U'<<8 | '1'<<16 | '2'<<24
)

// Output is a v4l2loopback device opened as the producer. While it is open
// and has received a frame, the device offers video capture, so
// applications list it even when it was loaded with exclusive_caps=1.
type Output struct {
	f    *os.File
	size int
}

// OpenOutput opens path as the producer of width x height YU12 frames.
func OpenOutput(path string, width, height int) (*Output, error) {
	if width <= 0 || height <= 0 || width%2 != 0 || height%2 != 0 {
		return nil, fmt.Errorf("frame size %dx%d: must be positive and even", width, height)
	}
	if _, err := CheckLoopback(path); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	size := width * height * 3 / 2
	fm := format{Type: bufTypeVideoOutput, Pix: pixFormat{
		Width:        uint32(width),
		Height:       uint32(height),
		PixelFormat:  pixFmtYU12,
		Field:        fieldNone,
		BytesPerLine: uint32(width),
		SizeImage:    uint32(size),
	}}
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), uintptr(vidiocSFmt), uintptr(unsafe.Pointer(&fm))); errno != 0 {
		f.Close()
		return nil, fmt.Errorf("%s: VIDIOC_S_FMT %dx%d YU12: %w (is another program writing to it?)", path, width, height, errno)
	}
	return &Output{f: f, size: size}, nil
}

// FrameSize is the length in bytes of one frame.
func (o *Output) FrameSize() int { return o.size }

// WriteFrame writes one complete frame.
func (o *Output) WriteFrame(frame []byte) error {
	if len(frame) != o.size {
		return fmt.Errorf("frame is %d bytes, want %d", len(frame), o.size)
	}
	_, err := o.f.Write(frame)
	return err
}

// Close releases the device; it then stops offering video capture.
func (o *Output) Close() error { return o.f.Close() }
