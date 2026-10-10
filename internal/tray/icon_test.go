package tray

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"testing"
	"time"
)

// decode renders the icon for a state and decodes it.
func decode(t *testing.T, st State, recording bool) *image.NRGBA {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(Icon(st, recording)))
	if err != nil {
		t.Fatalf("state %d: %v", st, err)
	}
	if b := img.Bounds(); b.Dx() != iconSize || b.Dy() != iconSize {
		t.Fatalf("state %d: size %v, want %dx%d", st, b, iconSize, iconSize)
	}
	out := image.NewNRGBA(img.Bounds())
	draw.Draw(out, out.Bounds(), img, img.Bounds().Min, draw.Src)
	return out
}

// screen is a point on the logo's teal display.
const screenX, screenY = 17, 32

var allStates = []State{NoCamera, Starting, Ready, Paused, Live, Trouble}

func TestIcon(t *testing.T) {
	if b := logo().Bounds(); b.Dx() != iconSize || b.Dy() != iconSize {
		t.Fatalf("icon.png is %v, want %dx%d; run go generate", b, iconSize, iconSize)
	}
	screen := logo().NRGBAAt(screenX, screenY)
	if screen.A != 0xff || screen.B <= screen.R {
		t.Fatalf("logo display %v, want an opaque blue-green pixel", screen)
	}
	want := map[State]color.NRGBA{Starting: blue, Ready: blue, Paused: gray, Live: green, Trouble: orange}
	// Unknown states show no dot, like NoCamera.
	for _, st := range append(allStates, State(-1), State(99)) {
		img := decode(t, st, false)
		if c := img.NRGBAAt(0, 0); c.A != 0 {
			t.Errorf("state %d: corner is not transparent: %v", st, c)
		}
		// The icon itself is always in color.
		if c := img.NRGBAAt(screenX, screenY); c != screen {
			t.Errorf("state %d: display %v, want the logo's %v", st, c, screen)
		}
		c := img.NRGBAAt(stateX, stateY)
		if dot, ok := want[st]; ok {
			if c != dot {
				t.Errorf("state %d: dot %v, want %v", st, c, dot)
			}
		} else if c != logo().NRGBAAt(stateX, stateY) {
			t.Errorf("state %d: a dot %v without a state", st, c)
		}
		if c := img.NRGBAAt(recordX, recordY); c == red {
			t.Errorf("state %d: red dot without a recording", st)
		}
	}
}

func TestIconRecording(t *testing.T) {
	for _, st := range allStates {
		img := decode(t, st, true)
		if c := img.NRGBAAt(recordX, recordY); c != red {
			t.Errorf("state %d: dot %v, want opaque red %v", st, c, red)
		}
		// The state dot stays.
		if c, plain := img.NRGBAAt(stateX, stateY), decode(t, st, false).NRGBAAt(stateX, stateY); c != plain {
			t.Errorf("state %d: state dot %v while recording, want %v", st, c, plain)
		}
	}
}

func TestOver(t *testing.T) {
	half := over(color.NRGBA{}, red, 0.5)
	if half.R != red.R || half.A != 0x80 {
		t.Errorf("red at half coverage over nothing: %v", half)
	}
	if c := over(color.NRGBA{0, 0, 0xff, 0xff}, red, 1); c != red {
		t.Errorf("opaque red over blue: %v", c)
	}
	if c := over(color.NRGBA{}, color.NRGBA{}, 1); c != (color.NRGBA{}) {
		t.Errorf("nothing over nothing: %v", c)
	}
}

func TestClock(t *testing.T) {
	for d, want := range map[time.Duration]string{
		0:                                     "0:00",
		59*time.Second + 600*time.Millisecond: "1:00",
		12*time.Minute + 34*time.Second:       "12:34",
		time.Hour + 2*time.Minute + 3*time.Second: "1:02:03",
	} {
		if got := Clock(d); got != want {
			t.Errorf("Clock(%v) = %q, want %q", d, got, want)
		}
	}
}
