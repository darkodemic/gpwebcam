package tray

import (
	"bytes"
	"image/color"
	"image/png"
	"testing"
)

func TestIcon(t *testing.T) {
	seen := map[color.NRGBA]State{}
	for _, st := range []State{Off, Live, Trouble} {
		img, err := png.Decode(bytes.NewReader(Icon(st)))
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
		if c := at(36, 32); c != lensInner {
			t.Errorf("state %d: lens centre %v, want %v", st, c, lensInner)
		}
		body := at(8, 40)
		if body != bodyColors[st] {
			t.Errorf("state %d: body %v, want %v", st, body, bodyColors[st])
		}
		if other, dup := seen[body]; dup {
			t.Errorf("states %d and %d look the same", other, st)
		}
		seen[body] = st
	}
}
