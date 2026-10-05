package v4l2

import (
	"testing"
	"unsafe"
)

func TestQuerycapNumber(t *testing.T) {
	if vidiocQuerycap != 0x80685600 {
		t.Fatalf("VIDIOC_QUERYCAP = %#x, want 0x80685600", vidiocQuerycap)
	}
}

func TestDevicePath(t *testing.T) {
	if p, err := DevicePath(42); err != nil || p != "/dev/video42" {
		t.Errorf("DevicePath(42) = %q, %v", p, err)
	}
	for _, n := range []int{-1, MaxDeviceNumber + 1} {
		if _, err := DevicePath(n); err == nil {
			t.Errorf("DevicePath(%d) = nil error", n)
		}
	}
}

func TestFormatLayout(t *testing.T) {
	// Values from linux/videodev2.h on x86_64.
	if s := unsafe.Sizeof(format{}); s != 208 {
		t.Errorf("sizeof(v4l2_format) = %d, want 208", s)
	}
	if o := unsafe.Offsetof(format{}.Pix); o != 8 {
		t.Errorf("offsetof(v4l2_format, fmt) = %d, want 8", o)
	}
	if vidiocSFmt != 0xc0d05605 {
		t.Errorf("VIDIOC_S_FMT = %#x, want 0xc0d05605", vidiocSFmt)
	}
	if pixFmtYU12 != 0x32315559 {
		t.Errorf("V4L2_PIX_FMT_YUV420 = %#x, want 0x32315559", pixFmtYU12)
	}
}
