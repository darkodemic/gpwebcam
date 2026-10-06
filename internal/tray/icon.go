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
	Off     State = iota // no video: no camera, or the camera is starting
	Live                 // video is flowing
	Trouble              // the camera is connected but does not work
)

// iconSize is the edge of the icon in pixels; panels scale it down.
const iconSize = 64

// subsamples per pixel edge; 4x4 samples smooth the edges.
const subsamples = 4

var (
	white  = color.NRGBA{0xff, 0xff, 0xff, 0xff}
	orange = color.NRGBA{0xf2, 0xa3, 0x3a, 0xff}
	// outline keeps the white body visible on a light panel; on a dark
	// one it hardly shows.
	outline   = color.NRGBA{0x00, 0x00, 0x00, 0x60}
	dark      = color.NRGBA{0x1d, 0x20, 0x26, 0xff}
	lensInner = color.NRGBA{0x5f, 0x66, 0x70, 0xff}
	shine     = color.NRGBA{0xc8, 0xcc, 0xd2, 0xff}
)

// bodyColors and opacity per state: white while video flows, orange on a
// problem, faded white otherwise.
var (
	bodyColors = map[State]color.NRGBA{Off: white, Live: white, Trouble: orange}
	opacity    = map[State]float64{Off: 0.45, Live: 1, Trouble: 1}
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

// rgba is a premultiplied color with float channels, for compositing.
type rgba struct{ r, g, b, a float64 }

// over paints c over dst.
func over(dst rgba, c color.NRGBA) rgba {
	a := float64(c.A) / 255
	return rgba{
		r: float64(c.R)/255*a + dst.r*(1-a),
		g: float64(c.G)/255*a + dst.g*(1-a),
		b: float64(c.B)/255*a + dst.b*(1-a),
		a: a + dst.a*(1-a),
	}
}

// Icon draws the icon for a state as PNG: a camera body with a dark lens.
func Icon(st State) []byte {
	body, ok := bodyColors[st]
	if !ok {
		st, body = Off, bodyColors[Off]
	}
	// Painted in order, later shapes over earlier ones.
	shapes := []shape{
		{roundRect(2, 10, 62, 54, 11), outline},
		{roundRect(4, 12, 60, 52, 9), body},
		{roundRect(10, 17, 20, 24, 2), dark},
		{circle(36, 32, 14), dark},
		{circle(36, 32, 7), lensInner},
		{circle(39, 29, 2.5), shine},
	}
	fade := opacity[st]
	img := image.NewNRGBA(image.Rect(0, 0, iconSize, iconSize))
	const n = subsamples * subsamples
	for py := 0; py < iconSize; py++ {
		for px := 0; px < iconSize; px++ {
			var sum rgba
			for sy := 0; sy < subsamples; sy++ {
				for sx := 0; sx < subsamples; sx++ {
					x := float64(px) + (float64(sx)+0.5)/subsamples
					y := float64(py) + (float64(sy)+0.5)/subsamples
					var c rgba
					for _, s := range shapes {
						if s.inside(x, y) {
							c = over(c, s.color)
						}
					}
					sum.r, sum.g, sum.b, sum.a = sum.r+c.r, sum.g+c.g, sum.b+c.b, sum.a+c.a
				}
			}
			if sum.a == 0 {
				continue
			}
			// Back from premultiplied to straight alpha.
			img.SetNRGBA(px, py, color.NRGBA{
				R: uint8(sum.r/sum.a*255 + 0.5),
				G: uint8(sum.g/sum.a*255 + 0.5),
				B: uint8(sum.b/sum.a*255 + 0.5),
				A: uint8(sum.a/n*fade*255 + 0.5),
			})
		}
	}
	var buf bytes.Buffer
	// Encoding an in-memory NRGBA image cannot fail.
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}
