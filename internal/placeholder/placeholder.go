// Package placeholder renders the pictures gpwebcam shows while no camera
// video is available, so the loopback device keeps offering a picture.
//
// A picture is one shared base frame (background and title) plus a band of
// rows with the status line. Statuses that wait for something cycle through
// ".", ".." and "..." after the text, so a viewer can tell gpwebcam is alive.
// Only the bands are kept per status, which keeps memory small.
package placeholder

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
	"unicode"
)

// Title is the first line of every placeholder.
const Title = "gpwebcam - GoPro webcam for Linux"

// Status lines that do not depend on the camera model.
const (
	NotConnected = "Camera not connected"
	NoVideo      = "No video from camera. Is a firewall blocking UDP?"
	NotAnswering = "Camera not answering. Unplug and replug the cable."
	Problem      = "Camera problem, retrying. See the gpwebcam log."
)

// WaitingNetwork, Starting and Retrying name the camera; they are shown
// with moving dots.
func WaitingNetwork(model string) string { return model + " found, waiting for its network" }
func Starting(model string) string       { return "Starting " + model }
func Retrying(model string) string       { return "No video from " + model + ", retrying" }

const (
	background = "0x1d2026"
	titleColor = "white"
	textColor  = "0xb0b4bc"
	// dotStep is how long each of "", ".", ".." and "..." stays.
	dotStep = 500 * time.Millisecond
	// textLuma separates text from background in the luma plane: the
	// background is about 0x24, the status text about 0xb2.
	textLuma = 0x60
)

// validText reports whether s is safe to put into a drawtext filter
// without escaping: letters, digits, spaces and - , . ? ( ). The text is
// quoted, so commas and parentheses are literal; quotes, colons, percent
// signs and backslashes, which drawtext or the filter parser interpret,
// are rejected.
func validText(s string) bool {
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == ' ', r == '-', r == ',', r == '.', r == '?', r == '(', r == ')':
		default:
			return false
		}
	}
	return s != ""
}

// Clean makes text from outside, such as a USB product string, safe for a
// status line: it keeps the characters validText allows, collapses spaces
// and cuts it to 40 bytes.
func Clean(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case unicode.IsSpace(r):
			b.WriteRune(' ')
		case validText(string(r)):
			b.WriteRune(r)
		}
	}
	s = strings.Join(strings.Fields(b.String()), " ")
	if len(s) > 40 {
		s = strings.TrimSpace(s[:40])
	}
	return s
}

// Renderer draws placeholder pictures of one size. It is safe for
// concurrent use.
type Renderer struct {
	ffmpeg     string
	w, h       int
	big, small int
	gap        int
	y0, y1     int // rows of the status band, both even
	base       []byte

	mu      sync.Mutex
	buf     []byte // the frame handed out last; reused
	lastPic *Picture
	lastIdx int
}

// NewRenderer renders the base frame with ffmpeg's drawtext filter. If that
// fails, for example because ffmpeg lacks drawtext, it returns a renderer
// that draws a plain dark frame, together with the error.
func NewRenderer(ctx context.Context, ffmpeg string, width, height int) (*Renderer, error) {
	r := &Renderer{
		ffmpeg: ffmpeg, w: width, h: height,
		big: height / 14, small: height / 22, gap: height / 30,
	}
	r.y0 = (height/2 + r.gap/2) &^ 1
	r.y1 = min(height, (height/2+r.gap+2*r.small+1)&^1)
	r.buf = make([]byte, width*height*3/2)
	base, err := r.render(ctx, fmt.Sprintf(
		"drawtext=text='%s':fontcolor=%s:fontsize=%d:x=(w-text_w)/2:y=h/2-text_h-%d",
		Title, titleColor, r.big, r.gap))
	if err != nil {
		r.base = Blank(width, height)
		return r, err
	}
	r.base = base
	return r, nil
}

// render runs ffmpeg on the background with the given drawtext filters and
// returns one YU12 frame.
func (r *Renderer) render(ctx context.Context, filters string) ([]byte, error) {
	graph := fmt.Sprintf("color=c=%s:s=%dx%d:r=1", background, r.w, r.h)
	if filters != "" {
		graph += "," + filters
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, r.ffmpeg, "-hide_banner", "-nostdin", "-loglevel", "error",
		"-f", "lavfi", "-i", graph, "-frames:v", "1", "-pix_fmt", "yuv420p", "-f", "rawvideo", "pipe:1")
	var out, errs bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errs
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("render placeholder: %w: %s", err, bytes.TrimSpace(errs.Bytes()))
	}
	if want := r.w * r.h * 3 / 2; out.Len() != want {
		return nil, fmt.Errorf("render placeholder: got %d bytes, want %d", out.Len(), want)
	}
	return out.Bytes(), nil
}

// Picture is a placeholder with one status line.
type Picture struct {
	r     *Renderer
	bands [][]byte // one per animation step; none for a title-only picture
}

// Picture renders status. With animate, it cycles dots after the text. On
// an error the returned picture still works and shows the base frame.
func (r *Renderer) Picture(ctx context.Context, status string, animate bool) (*Picture, error) {
	if !validText(status) {
		return &Picture{r: r}, fmt.Errorf("placeholder text %q: unsupported characters", status)
	}
	// The status is centred without dots, so it does not move while the
	// dots change; the dots go right after its last column, on the same
	// baseline. Trailing spaces would not help: drawtext leaves them out
	// of text_w, so padded text still moves (measured 2026-10-05).
	baseline := r.h/2 + r.gap + r.small
	text := fmt.Sprintf("drawtext=text='%s':fontcolor=%s:fontsize=%d:x=(w-text_w)/2:y=%d:y_align=baseline",
		status, textColor, r.small, baseline)
	plain, err := r.render(ctx, text)
	if err != nil {
		return &Picture{r: r}, err
	}
	p := &Picture{r: r, bands: [][]byte{r.band(plain)}}
	if !animate {
		return p, nil
	}
	right := r.textRight(plain)
	if right < 0 {
		return p, nil
	}
	for n := 1; n <= 3; n++ {
		dots := fmt.Sprintf("%s,drawtext=text='%s':fontcolor=%s:fontsize=%d:x=%d:y=%d:y_align=baseline",
			text, strings.Repeat(".", n), textColor, r.small, right+r.small/8, baseline)
		f, err := r.render(ctx, dots)
		if err != nil {
			return p, err
		}
		p.bands = append(p.bands, r.band(f))
	}
	return p, nil
}

// band copies the status rows y0..y1 of a frame: luma rows, then the
// matching rows of both chroma planes.
func (r *Renderer) band(frame []byte) []byte {
	w, h := r.w, r.h
	b := make([]byte, 0, (r.y1-r.y0)*w*3/2)
	b = append(b, frame[r.y0*w:r.y1*w]...)
	cw, c0, c1 := w/2, r.y0/2, r.y1/2
	u, v := w*h, w*h+(w/2)*(h/2)
	b = append(b, frame[u+c0*cw:u+c1*cw]...)
	b = append(b, frame[v+c0*cw:v+c1*cw]...)
	return b
}

// textRight returns the last column of the status band that holds text,
// or -1.
func (r *Renderer) textRight(frame []byte) int {
	for x := r.w - 1; x >= 0; x-- {
		for y := r.y0; y < r.y1; y++ {
			if frame[y*r.w+x] > textLuma {
				return x
			}
		}
	}
	return -1
}

// Frame returns the picture as it should look elapsed after it was first
// shown. The returned slice is reused by the next call on the same
// Renderer, so write it out before calling again.
func (p *Picture) Frame(elapsed time.Duration) []byte {
	r := p.r
	r.mu.Lock()
	defer r.mu.Unlock()
	idx := 0
	if len(p.bands) > 1 {
		idx = int(elapsed/dotStep) % len(p.bands)
	}
	if p == r.lastPic && idx == r.lastIdx {
		return r.buf
	}
	copy(r.buf, r.base)
	if len(p.bands) > 0 {
		w, h := r.w, r.h
		b := p.bands[idx]
		ny := (r.y1 - r.y0) * w
		nc := (r.y1 - r.y0) / 2 * (w / 2)
		copy(r.buf[r.y0*w:], b[:ny])
		u, v := w*h, w*h+(w/2)*(h/2)
		copy(r.buf[u+r.y0/2*(w/2):], b[ny:ny+nc])
		copy(r.buf[v+r.y0/2*(w/2):], b[ny+nc:])
	}
	r.lastPic, r.lastIdx = p, idx
	return r.buf
}

// Blank returns a plain dark width x height YU12 frame, for when rendering
// fails, for example because ffmpeg lacks the drawtext filter.
func Blank(width, height int) []byte {
	y := width * height
	f := make([]byte, y*3/2)
	for i := range f[:y] {
		f[i] = 0x24 // dark grey in limited-range luma
	}
	for i := range f[y:] {
		f[y+i] = 0x80 // neutral chroma
	}
	return f
}
