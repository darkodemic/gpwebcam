package tray

import (
	"bytes"
	_ "embed"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"sync"
)

// icon.png is the application icon at iconSize, rendered from the SVG
// that the packages install; regenerate it after changing the SVG. The
// render is 72x72 cropped to the middle 64x64, so the camera body fills
// the width of a tray slot instead of leaving the SVG's margins empty.
//
//go:generate rsvg-convert -w 72 -h 72 --page-width 64 --page-height 64 --left=-4 --top=-4 -o icon.png ../../packaging/icons/gpwebcam.svg
//go:embed icon.png
var iconPNG []byte

// State is what the icon shows.
type State int

const (
	NoCamera State = iota // no camera is connected
	Starting              // a camera is connected and is being started
	Ready                 // camera mode demand: the camera waits for an application
	Paused                // camera mode off
	Live                  // video is flowing
	Trouble               // the camera is connected but does not work
)

// iconSize is the edge of the icon in pixels; panels scale it down.
const iconSize = 64

// subsamples per pixel edge; 4x4 samples smooth the edges of the dots.
const subsamples = 4

var (
	green  = color.NRGBA{0x3e, 0xc4, 0x6d, 0xff}
	blue   = color.NRGBA{0x3d, 0x8b, 0xfd, 0xff}
	gray   = color.NRGBA{0x9e, 0x9e, 0x9e, 0xff}
	orange = color.NRGBA{0xf2, 0xa3, 0x3a, 0xff}
	red    = color.NRGBA{0xe5, 0x39, 0x35, 0xff}
	// rim keeps a dot visible on a light panel; on a dark one it hardly
	// shows.
	rim = color.NRGBA{0x00, 0x00, 0x00, 0x60}
)

// stateDots is the color of the state dot; a state without one, such as
// NoCamera, shows the bare icon.
var stateDots = map[State]color.NRGBA{
	Starting: blue,
	Ready:    blue,
	Paused:   gray,
	Live:     green,
	Trouble:  orange,
}

// Where the dots sit, in the 64x64 icon: the state dot over the cooling
// ribs at the bottom right, the recording dot on the lens cover's top
// right corner.
const (
	stateX, stateY     = 53, 51
	recordX, recordY   = 53, 11
	dotRadius, rimSize = 9, 2
)

// logo decodes the embedded application icon once.
var logo = sync.OnceValue(func() *image.NRGBA {
	src, err := png.Decode(bytes.NewReader(iconPNG))
	if err != nil {
		panic("tray: embedded icon.png: " + err.Error())
	}
	img := image.NewNRGBA(src.Bounds())
	draw.Draw(img, img.Bounds(), src, src.Bounds().Min, draw.Src)
	return img
})

// Icon draws the icon for a state as PNG: the application icon with a
// dot for the state at the bottom right (green live, blue starting or
// waiting for an application, gray paused, orange a problem, none without
// a camera) and a red dot at the top right while recording.
func Icon(st State, recording bool) []byte {
	img := image.NewNRGBA(logo().Bounds())
	copy(img.Pix, logo().Pix)
	if c, ok := stateDots[st]; ok {
		dot(img, stateX, stateY, c)
	}
	if recording {
		dot(img, recordX, recordY, red)
	}
	var buf bytes.Buffer
	// Encoding an in-memory NRGBA image cannot fail.
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

// dot paints a filled circle with a dark rim around it.
func dot(img *image.NRGBA, cx, cy float64, c color.NRGBA) {
	paint(img, circle(cx, cy, dotRadius+rimSize), rim)
	paint(img, circle(cx, cy, dotRadius), c)
}

func circle(cx, cy, r float64) func(x, y float64) bool {
	return func(x, y float64) bool { return (x-cx)*(x-cx)+(y-cy)*(y-cy) <= r*r }
}

// paint lays c over img where inside is true, weighting each pixel by how
// many of its subsamples are inside.
func paint(img *image.NRGBA, inside func(x, y float64) bool, c color.NRGBA) {
	const n = subsamples * subsamples
	b := img.Bounds()
	for py := b.Min.Y; py < b.Max.Y; py++ {
		for px := b.Min.X; px < b.Max.X; px++ {
			hits := 0
			for sy := 0; sy < subsamples; sy++ {
				for sx := 0; sx < subsamples; sx++ {
					x := float64(px) + (float64(sx)+0.5)/subsamples
					y := float64(py) + (float64(sy)+0.5)/subsamples
					if inside(x, y) {
						hits++
					}
				}
			}
			if hits > 0 {
				img.SetNRGBA(px, py, over(img.NRGBAAt(px, py), c, float64(hits)/n))
			}
		}
	}
}

// over paints c, with its opacity scaled by coverage, over dst; both are
// straight (not premultiplied) alpha.
func over(dst, c color.NRGBA, coverage float64) color.NRGBA {
	sa := float64(c.A) / 255 * coverage
	da := float64(dst.A) / 255 * (1 - sa)
	a := sa + da
	if a == 0 {
		return color.NRGBA{}
	}
	mix := func(s, d uint8) uint8 { return uint8((float64(s)*sa+float64(d)*da)/a + 0.5) }
	return color.NRGBA{mix(c.R, dst.R), mix(c.G, dst.G), mix(c.B, dst.B), uint8(a*255 + 0.5)}
}
