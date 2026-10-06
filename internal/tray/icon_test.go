package tray

import (
	"bytes"
	"image/color"
	"image/png"
	"testing"
	"time"
)

func TestIcon(t *testing.T) {
	want := map[State]color.NRGBA{
		Live:    {0xff, 0xff, 0xff, 0xff},
		Trouble: {0xf2, 0xa3, 0x3a, 0xff},
		Off:     {0xff, 0xff, 0xff, 0x73}, // white at 45 %
	}
	for st, body := range want {
		img, err := png.Decode(bytes.NewReader(Icon(st, false)))
		if err != nil {
			t.Fatalf("state %d: %v", st, err)
		}
		if b := img.Bounds(); b.Dx() != iconSize || b.Dy() != iconSize {
			t.Fatalf("state %d: size %v", st, b)
		}
		at := func(x, y int) color.NRGBA {
			return color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
		}
		if c := at(0, 0); c.A != 0 {
			t.Errorf("state %d: corner is not transparent: %v", st, c)
		}
		if c := at(8, 40); c != body {
			t.Errorf("state %d: body %v, want %v", st, c, body)
		}
		if c := at(36, 32); c.R != lensInner.R || c.A != body.A {
			t.Errorf("state %d: lens center %v, want %v at the body's opacity", st, c, lensInner)
		}
		if c := at(2, 32); c.A == 0 || c.R > 0x10 {
			t.Errorf("state %d: no dark outline at the left edge: %v", st, c)
		}
	}
}

func TestIconRecording(t *testing.T) {
	for _, st := range []State{Off, Live, Trouble} {
		img, err := png.Decode(bytes.NewReader(Icon(st, true)))
		if err != nil {
			t.Fatal(err)
		}
		if c := color.NRGBAModel.Convert(img.At(52, 12)).(color.NRGBA); c != red {
			t.Errorf("state %d: dot %v, want opaque red %v", st, c, red)
		}
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
