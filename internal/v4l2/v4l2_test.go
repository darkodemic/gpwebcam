package v4l2

import "testing"

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
