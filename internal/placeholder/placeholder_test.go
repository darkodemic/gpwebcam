package placeholder

import (
	"context"
	"os/exec"
	"testing"
)

func TestValidText(t *testing.T) {
	for _, ok := range []string{Title, NotConnected, Connecting, Retrying} {
		if !validText(ok) {
			t.Errorf("validText(%q) = false", ok)
		}
	}
	for _, bad := range []string{"", "a:b", "it's", "100%", `back\slash`, "a;b", "[x]", "new\nline"} {
		if validText(bad) {
			t.Errorf("validText(%q) = true", bad)
		}
	}
}

func TestRender(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	f, err := Render(context.Background(), "ffmpeg", 640, 360, NotConnected)
	if err != nil {
		t.Fatal(err)
	}
	if len(f) != 640*360*3/2 {
		t.Fatalf("frame is %d bytes", len(f))
	}
	// The text must have drawn bright pixels on the dark background.
	bright := 0
	for _, y := range f[:640*360] {
		if y > 0xc0 {
			bright++
		}
	}
	if bright == 0 {
		t.Error("no text drawn")
	}
	if _, err := Render(context.Background(), "ffmpeg", 640, 360, "50%"); err == nil {
		t.Error("unsafe text accepted")
	}
}

func TestBlank(t *testing.T) {
	if f := Blank(4, 2); len(f) != 12 || f[0] != 0x24 || f[11] != 0x80 {
		t.Errorf("Blank(4, 2) = %v", f)
	}
}
