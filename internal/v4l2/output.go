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
	f             *os.File
	width, height int
	size          int
}

// OpenOutput opens path as the producer of YU12 frames, asking for width x
// height. While an application keeps the device open, v4l2loopback keeps
// the format it has and S_FMT succeeds with that one; Size then differs
// from the request, and frames must have the device's size.
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
	fm := format{Type: bufTypeVideoOutput, Pix: pixFormat{
		Width:        uint32(width),
		Height:       uint32(height),
		PixelFormat:  pixFmtYU12,
		Field:        fieldNone,
		BytesPerLine: uint32(width),
		SizeImage:    uint32(width * height * 3 / 2),
	}}
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), uintptr(vidiocSFmt), uintptr(unsafe.Pointer(&fm))); errno != 0 {
		f.Close()
		return nil, fmt.Errorf("%s: VIDIOC_S_FMT %dx%d YU12: %w (is another program writing to it?)", path, width, height, errno)
	}
	w, h, err := accepted(fm.Pix)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &Output{f: f, width: w, height: h, size: w * h * 3 / 2}, nil
}

// accepted checks the format S_FMT returned and gives its frame size.
func accepted(p pixFormat) (width, height int, err error) {
	if p.PixelFormat != pixFmtYU12 {
		return 0, 0, fmt.Errorf("the device keeps pixel format %q, not YU12, while an application uses it; close that application", fourCC(p.PixelFormat))
	}
	w, h := int(p.Width), int(p.Height)
	if w <= 0 || h <= 0 || w%2 != 0 || h%2 != 0 || (p.BytesPerLine != 0 && int(p.BytesPerLine) != w) {
		return 0, 0, fmt.Errorf("the device keeps an unusable format %dx%d, %d bytes per line", w, h, p.BytesPerLine)
	}
	return w, h, nil
}

func fourCC(v uint32) string {
	return string([]byte{byte(v), byte(v >> 8), byte(v >> 16), byte(v >> 24)})
}

// Size is the frame size the device accepted.
func (o *Output) Size() (width, height int) { return o.width, o.height }

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
