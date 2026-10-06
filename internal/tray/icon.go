package tray

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
)

// State is what the icon shows.
type State int

const (
	Off     State = iota // no camera, or the camera is starting
	Live                 // video is flowing
	Trouble              // the camera is connected but does not work
)

// iconSize is the edge of the icon in pixels; panels scale it down.
const iconSize = 64

// subsamples per pixel edge; 4x4 samples smooth the edges.
const subsamples = 4

var (
	bodyColors = map[State]color.NRGBA{
		Off:     {0x80, 0x86, 0x8b, 0xff},
		Live:    {0x4c, 0x8b, 0xf5, 0xff},
		Trouble: {0xf2, 0xa3, 0x3a, 0xff},
	}
	dark      = color.NRGBA{0x1d, 0x20, 0x26, 0xff}
	lensInner = color.NRGBA{0x5f, 0x66, 0x70, 0xff}
	shine     = color.NRGBA{0xc8, 0xcc, 0xd2, 0xff}
)

// shape is one filled area of the icon, in a 64x64 coordinate space.
type shape struct {
	inside func(x, y float64) bool
	color  color.NRGBA
}

func roundRect(x0, y0, x1, y1, r float64) func(x, y float64) bool {
	return func(x, y float64) bool {
		if x < x0 || x > x1 || y < y0 || y > y1 {
			return false
		}
		dx := max(x0+r-x, 0, x-(x1-r))
		dy := max(y0+r-y, 0, y-(y1-r))
		return dx*dx+dy*dy <= r*r
	}
}

func circle(cx, cy, r float64) func(x, y float64) bool {
	return func(x, y float64) bool { return (x-cx)*(x-cx)+(y-cy)*(y-cy) <= r*r }
}

// Icon draws the icon for a state as PNG: a camera body in the state's
// color with a dark lens, so it reads on light and dark panels alike.
func Icon(st State) []byte {
	body, ok := bodyColors[st]
	if !ok {
		body = bodyColors[Off]
	}
	// Later shapes paint over earlier ones.
	shapes := []shape{
		{roundRect(4, 12, 60, 52, 9), body},
		{roundRect(10, 17, 20, 24, 2), dark},
		{circle(36, 32, 14), dark},
		{circle(36, 32, 7), lensInner},
		{circle(39, 29, 2.5), shine},
	}
	img := image.NewNRGBA(image.Rect(0, 0, iconSize, iconSize))
	const n = subsamples * subsamples
	for py := 0; py < iconSize; py++ {
		for px := 0; px < iconSize; px++ {
			var r, g, b, hits int
			for sy := 0; sy < subsamples; sy++ {
				for sx := 0; sx < subsamples; sx++ {
					x := float64(px) + (float64(sx)+0.5)/subsamples
					y := float64(py) + (float64(sy)+0.5)/subsamples
					for i := len(shapes) - 1; i >= 0; i-- {
						if shapes[i].inside(x, y) {
							c := shapes[i].color
							r, g, b = r+int(c.R), g+int(c.G), b+int(c.B)
							hits++
							break
						}
					}
				}
			}
			if hits == 0 {
				continue
			}
			img.SetNRGBA(px, py, color.NRGBA{
				R: uint8(r / hits), G: uint8(g / hits), B: uint8(b / hits),
				A: uint8(255 * hits / n),
			})
		}
	}
	var buf bytes.Buffer
	// Encoding an in-memory NRGBA image cannot fail.
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}
