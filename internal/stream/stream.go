// Package stream runs ffmpeg, which receives the camera's MPEG-TS stream over
// UDP, decodes it and hands gpwebcam raw frames through a pipe.
package stream

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"sync/atomic"
	"syscall"
	"time"
)

// ErrNoVideo means the camera reported a running stream, but no frame
// arrived within Config.FirstFrame.
var ErrNoVideo = errors.New("the camera reports streaming, but no video arrived; a firewall or VPN may be dropping UDP, or the camera did not really start")

// Config describes one ffmpeg run. Every field is validated by the caller's
// types or by Validate, so nothing unchecked reaches the argument list.
type Config struct {
	FFmpeg string         // ffmpeg executable, looked up in PATH if it has no slash
	Listen netip.AddrPort // host address on the GoPro link; never unspecified
	// Width and Height are the size of the frames gpwebcam gets; ffmpeg scales
	// to them, so they always match the loopback device's format.
	Width, Height int
	// ReadTimeout makes ffmpeg exit when no packet arrives for this long,
	// for example after the camera is unplugged. While opening the input,
	// ffmpeg reads several times, so with no stream at all it gives up
	// after about four timeouts (measured with ffmpeg 9.0.2).
	ReadTimeout time.Duration
	// HWAccel is ffmpeg's -hwaccel value: "auto" decodes on the GPU when
	// one is usable and falls back to software otherwise; "" or "none"
	// always decodes in software.
	HWAccel string
	// FirstFrame, when positive, ends the run with ErrNoVideo if no frame
	// arrives this long after ffmpeg starts. It catches a camera that
	// reports streaming but sends nothing much sooner than ReadTimeout,
	// which ffmpeg applies several times while it opens the input.
	FirstFrame time.Duration
}

// Validate rejects configurations that would make ffmpeg listen beyond the
// GoPro link or write somewhere unexpected.
func (c Config) Validate() error {
	a := c.Listen.Addr()
	if !a.IsValid() || !a.Is4() || a.IsUnspecified() || a.IsMulticast() {
		return fmt.Errorf("listen address %s: must be the host's unicast IPv4 address on the GoPro interface", c.Listen)
	}
	if c.Listen.Port() == 0 {
		return fmt.Errorf("listen port must not be 0")
	}
	if c.ReadTimeout <= 0 {
		return fmt.Errorf("read timeout must be positive")
	}
	if c.Width <= 0 || c.Height <= 0 || c.Width%2 != 0 || c.Height%2 != 0 {
		return fmt.Errorf("frame size %dx%d: must be positive and even", c.Width, c.Height)
	}
	if c.FFmpeg == "" {
		return fmt.Errorf("ffmpeg path must be set")
	}
	switch c.HWAccel {
	case "", "none", "auto":
	default:
		return fmt.Errorf("hardware decoding %q: must be auto or none", c.HWAccel)
	}
	return nil
}

// FrameSize is the length in bytes of one yuv420p frame.
func (c Config) FrameSize() int { return c.Width * c.Height * 3 / 2 }

// Args returns ffmpeg's argument list, without the program name.
func (c Config) Args() []string {
	// For input, ffmpeg binds the UDP socket to the host in the URL
	// (checked with ffmpeg 9.0.2), so only the GoPro link can feed it.
	// timeout is in microseconds and applies to reads only.
	url := fmt.Sprintf("udp://%s?timeout=%d&overrun_nonfatal=1",
		c.Listen, c.ReadTimeout.Microseconds())
	var hw []string
	if c.HWAccel == "auto" {
		// Measured 2026-10-05 on an AMD GPU (VAAPI), 1080p30 H.264: 8.7 %
		// of one core instead of 13.7 %, latency 74 ms instead of 71 ms.
		hw = []string{"-hwaccel", "auto"}
	}
	return append([]string{
		"-hide_banner",
		"-nostdin",
		// Warnings repeat on every start (the TS's audio and data streams,
		// the yuvj420p range); errors such as decoding failures still show.
		"-loglevel", "error",
		// Input options go before -i, or ffmpeg applies them to the output.
		// nobuffer drops the packets read while probing instead of queueing
		// them, so the first frame shown is current. With the default 5 s
		// analyzeduration that loses the first 6 s; 1 s brings it to about
		// 2 s (measured with ffmpeg 9.0.2 on a 1080p30 H.264 + AAC TS).
		"-fflags", "nobuffer",
		"-flags", "low_delay",
		"-analyzeduration", "1000000",
	}, append(hw,
		"-f", "mpegts",
		"-i", url,
		// The TS also carries AAC, an empty AC3 track and a private data
		// stream; only the video goes to the loopback device.
		"-map", "0:v:0",
		// Hand on every frame as soon as it is decoded. The default for
		// raw video output is constant frame rate, which held the camera's
		// frames back by about 0.85 s (measured 2026-10-05: 1.02 s from
		// scene to screen with the default, 0.18 s with passthrough).
		"-fps_mode", "passthrough",
		"-vf", fmt.Sprintf("scale=%d:%d,format=yuv420p", c.Width, c.Height),
		"-f", "rawvideo",
		// Without this the tail of each frame waits in ffmpeg's output
		// buffer until the next frame arrives.
		"-flush_packets", "1",
		"pipe:1",
	)...)
}

// Run starts ffmpeg and passes every decoded frame to sink until ffmpeg
// exits. The slice is reused for the next frame. Cancelling ctx sends
// SIGTERM, then SIGKILL after grace; a sink error stops ffmpeg the same way.
// ffmpeg's stderr is copied to logs.
func Run(ctx context.Context, c Config, logs io.Writer, grace time.Duration, sink func([]byte) error) error {
	if err := c.Validate(); err != nil {
		return err
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	r, w, err := os.Pipe()
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(runCtx, c.FFmpeg, c.Args()...)
	cmd.Stdout = w
	cmd.Stderr = logs
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
	cmd.WaitDelay = grace
	// Die with gpwebcam even if gpwebcam is killed without a chance to clean up.
	// Pdeathsig fires when the forking OS thread exits, so keep this
	// goroutine on that thread until ffmpeg is gone.
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := cmd.Start(); err != nil {
		r.Close()
		w.Close()
		return fmt.Errorf("start ffmpeg: %w", err)
	}
	// ffmpeg holds the write end now; reads see EOF once it exits.
	w.Close()

	var sinkErr error
	var gotFrame, noVideo atomic.Bool
	if c.FirstFrame > 0 {
		t := time.AfterFunc(c.FirstFrame, func() {
			if !gotFrame.Load() {
				noVideo.Store(true)
				cancel()
			}
		})
		defer t.Stop()
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer r.Close() // an early return makes ffmpeg's next write fail
		frame := make([]byte, c.FrameSize())
		for {
			if _, err := io.ReadFull(r, frame); err != nil {
				return
			}
			gotFrame.Store(true)
			if err := sink(frame); err != nil {
				sinkErr = err
				cancel()
				return
			}
		}
	}()
	err = cmd.Wait()
	<-done
	switch {
	case ctx.Err() != nil:
		// Stopped on request; ffmpeg's exit status after SIGTERM is noise.
		return nil
	case sinkErr != nil:
		return fmt.Errorf("write frame: %w", sinkErr)
	case noVideo.Load():
		return fmt.Errorf("%w (waited %v)", ErrNoVideo, c.FirstFrame)
	case err != nil:
		return fmt.Errorf("ffmpeg: %w", err)
	}
	// With UDP input ffmpeg ends cleanly only when the read timeout fires.
	return fmt.Errorf("no video from the camera for %v; was it unplugged or switched off?", c.ReadTimeout)
}

// Port parses and range-checks a UDP port given as text.
func Port(s string) (uint16, error) {
	n, err := strconv.ParseUint(s, 10, 16)
	if err != nil || n < 1024 {
		return 0, fmt.Errorf("port %q: must be a number from 1024 to 65535", s)
	}
	return uint16(n), nil
}
