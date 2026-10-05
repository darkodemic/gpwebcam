package placeholder

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func statuses() []string {
	m := "GoPro HERO13 Black"
	return []string{NotConnected, NoVideo, NotAnswering, Problem, WaitingNetwork(m), Starting(m), Retrying(m)}
}

func TestValidText(t *testing.T) {
	for _, ok := range append([]string{Title}, statuses()...) {
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

func TestClean(t *testing.T) {
	for in, want := range map[string]string{
		"HERO13 Black":                  "HERO13 Black",
		"  GoPro\tHERO9  ":              "GoPro HERO9",
		"HERO'; drawtext=text='%{pts}'": "HERO drawtexttextpts",
		"\x00\x01":                      "",
		strings.Repeat("x", 60):         strings.Repeat("x", 40),
	} {
		if got := Clean(in); got != want {
			t.Errorf("Clean(%q) = %q, want %q", in, got, want)
		}
		if got := Clean(in); got != "" && !validText(got) {
			t.Errorf("Clean(%q) = %q is not valid text", in, got)
		}
	}
}

// leftmost returns the first column with text in rows y0..y1, or -1.
func leftmost(frame []byte, w, y0, y1 int) int {
	for x := 0; x < w; x++ {
		for y := y0; y < y1; y++ {
			if frame[y*w+x] > textLuma {
				return x
			}
		}
	}
	return -1
}

func TestPictures(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	const w, h = 640, 360
	ctx := context.Background()
	r, err := NewRenderer(ctx, "ffmpeg", w, h)
	if err != nil {
		t.Fatal(err)
	}

	still, err := r.Picture(ctx, NotConnected, false)
	if err != nil {
		t.Fatal(err)
	}
	f := still.Frame(0)
	if len(f) != w*h*3/2 {
		t.Fatalf("frame is %d bytes", len(f))
	}
	if leftmost(f, w, 0, r.y0) < 0 {
		t.Error("no title above the status band")
	}
	if leftmost(f, w, r.y0, r.y1) < 0 {
		t.Error("no status text in the band")
	}
	if !bytes.Equal(still.Frame(0), still.Frame(5*time.Second)) {
		t.Error("a still picture changes over time")
	}

	moving, err := r.Picture(ctx, Starting("GoPro HERO13 Black"), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(moving.bands) != 4 {
		t.Fatalf("%d animation steps, want 4", len(moving.bands))
	}
	var frames [][]byte
	for i := 0; i < 4; i++ {
		frames = append(frames, bytes.Clone(moving.Frame(time.Duration(i)*dotStep)))
	}
	left := leftmost(frames[0], w, r.y0, r.y1)
	for i, fr := range frames {
		if l := leftmost(fr, w, r.y0, r.y1); l != left {
			t.Errorf("step %d: text starts at column %d, want %d (the text moved)", i, l, left)
		}
		if !bytes.Equal(fr[:r.y0*w], frames[0][:r.y0*w]) {
			t.Errorf("step %d: rows above the status band changed", i)
		}
		if i > 0 && bytes.Equal(fr, frames[i-1]) {
			t.Errorf("step %d looks like step %d", i, i-1)
		}
	}
	if !bytes.Equal(moving.Frame(4*dotStep), frames[0]) {
		t.Error("the animation does not loop")
	}

	if _, err := r.Picture(ctx, "50%", false); err == nil {
		t.Error("unsafe text accepted")
	}
}

func TestBlank(t *testing.T) {
	if f := Blank(4, 2); len(f) != 12 || f[0] != 0x24 || f[11] != 0x80 {
		t.Errorf("Blank(4, 2) = %v", f)
	}
}
