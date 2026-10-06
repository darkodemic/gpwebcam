package v4l2

import (
	"os"
	"path/filepath"
	"strings"
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

func TestFindByLabel(t *testing.T) {
	root := t.TempDir()
	dev := func(name, label string) {
		d := filepath.Join(root, "class", "video4linux", name)
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, "name"), []byte(label+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	dev("video0", "UVC Camera (1234:5678)")
	dev("video1", "UVC Camera (1234:5678)")
	dev("video2", "OBS Virtual Camera")
	dev("video42", "GoPro")
	dev("v4l-subdev0", "GoPro") // not a videoN node

	if p, err := FindByLabel(root, "GoPro"); err != nil || p != "/dev/video42" {
		t.Errorf("FindByLabel(GoPro) = %q, %v", p, err)
	}
	if _, err := FindByLabel(root, "Nope"); err == nil {
		t.Error("missing label found")
	}
	dev("video43", "GoPro")
	if _, err := FindByLabel(root, "GoPro"); err == nil || !strings.Contains(err.Error(), "several") {
		t.Errorf("duplicate label: %v", err)
	}
}

func TestAccepted(t *testing.T) {
	yu12 := func(w, h, bpl uint32) pixFormat {
		return pixFormat{Width: w, Height: h, PixelFormat: pixFmtYU12, BytesPerLine: bpl}
	}
	for _, tc := range []struct {
		name string
		p    pixFormat
		w, h int
		ok   bool
	}{
		{"as asked", yu12(1920, 1080, 1920), 1920, 1080, true},
		{"kept by a reader", yu12(1280, 720, 1280), 1280, 720, true},
		{"no bytes per line", yu12(1280, 720, 0), 1280, 720, true},
		{"other pixel format", pixFormat{Width: 1280, Height: 720, PixelFormat: 'Y' | 'U'<<8 | 'Y'<<16 | 'V'<<24}, 0, 0, false},
		{"odd size", yu12(1281, 720, 1281), 0, 0, false},
		{"padded lines", yu12(1280, 720, 1344), 0, 0, false},
		{"empty", yu12(0, 0, 0), 0, 0, false},
	} {
		w, h, err := accepted(tc.p)
		if (err == nil) != tc.ok || w != tc.w || h != tc.h {
			t.Errorf("%s: got %dx%d, %v", tc.name, w, h, err)
		}
	}
}

func TestEventLayout(t *testing.T) {
	// Values from linux/videodev2.h, printed by a C program 2026-10-06.
	if s := unsafe.Sizeof(event{}); s != 136 {
		t.Errorf("sizeof(event) = %d, want 136", s)
	}
	if o := unsafe.Offsetof(event{}.Pending); o != 72 {
		t.Errorf("offsetof(event.Pending) = %d, want 72", o)
	}
	if s := unsafe.Sizeof(eventSubscription{}); s != 32 {
		t.Errorf("sizeof(eventSubscription) = %d, want 32", s)
	}
	if vidiocSubscribeEvent != 0x4020565a || vidiocDQEvent != 0x80885659 {
		t.Errorf("ioctl numbers %#x %#x, want 0x4020565a 0x80885659", vidiocSubscribeEvent, vidiocDQEvent)
	}
	var ev event
	ev.U[0] = 1
	if clientCount(ev) != 1 {
		t.Error("client count not read from the start of the payload")
	}
}
