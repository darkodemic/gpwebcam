// Package placeholder renders the still frame gpwebcam shows while no camera
// video is available, so the loopback device keeps offering a picture.
package placeholder

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"
)

// Title is the first line of every placeholder.
const Title = "gpwebcam - GoPro webcam for Linux"

// Status lines shown under the title.
const (
	NotConnected   = "Camera not connected"
	WaitingNetwork = "Camera found, waiting for its network"
	Starting       = "Starting the camera"
	Retrying       = "No video from camera, retrying"
	NoVideo        = "No video from camera. Is a firewall blocking UDP?"
	NotAnswering   = "Camera not answering. Unplug and replug the cable."
	Problem        = "Camera problem, retrying. See the gpwebcam log."
)

// All lists every status line, for rendering them up front.
var All = []string{NotConnected, WaitingNetwork, Starting, Retrying, NoVideo, NotAnswering, Problem}

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

// Render draws Title and status on a dark background as one width x height
// YU12 (yuv420p) frame, using ffmpeg's drawtext filter.
func Render(ctx context.Context, ffmpeg string, width, height int, status string) ([]byte, error) {
	if !validText(Title) || !validText(status) {
		return nil, fmt.Errorf("placeholder text %q: unsupported characters", status)
	}
	big, small := height/14, height/22
	gap := height / 30
	graph := fmt.Sprintf("color=c=0x1d2026:s=%dx%d:r=1,"+
		"drawtext=text='%s':fontcolor=white:fontsize=%d:x=(w-text_w)/2:y=h/2-text_h-%d,"+
		"drawtext=text='%s':fontcolor=0xb0b4bc:fontsize=%d:x=(w-text_w)/2:y=h/2+%d",
		width, height, Title, big, gap, status, small, gap)
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, ffmpeg, "-hide_banner", "-nostdin", "-loglevel", "error",
		"-f", "lavfi", "-i", graph, "-frames:v", "1", "-pix_fmt", "yuv420p", "-f", "rawvideo", "pipe:1")
	var out, errs bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errs
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("render placeholder: %w: %s", err, bytes.TrimSpace(errs.Bytes()))
	}
	if want := width * height * 3 / 2; out.Len() != want {
		return nil, fmt.Errorf("render placeholder: got %d bytes, want %d", out.Len(), want)
	}
	return out.Bytes(), nil
}

// Blank returns a plain dark width x height YU12 frame, for when Render
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
