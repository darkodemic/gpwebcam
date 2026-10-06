// Package stream receives the camera's MPEG-TS stream over UDP and runs
// ffmpeg, which decodes it from a pipe and hands gpwebcam raw frames through
// another pipe.
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
	"strings"
	"sync/atomic"
	"syscall"
	"time"
)

// ErrNoVideo means the camera reported a running stream, but no frame
// arrived within Config.FirstFrame. ErrNoPackets, which matches it, says
// that not even a datagram arrived.
var (
	ErrNoVideo   = errors.New("the camera reports streaming, but no video arrived")
	ErrNoPackets = fmt.Errorf("%w: no packet reached gpwebcam; a firewall or VPN may be dropping UDP, or the camera did not really start", ErrNoVideo)
)

// Config describes one ffmpeg run. Every field is validated by the caller's
// types or by Validate, so nothing unchecked reaches the argument list.
type Config struct {
	FFmpeg string         // ffmpeg executable, looked up in PATH if it has no slash
	Listen netip.AddrPort // host address on the GoPro link; never unspecified
	// Camera, when valid, is the only sender whose datagrams count.
	Camera netip.Addr
	// Width and Height are the size of the frames gpwebcam gets; ffmpeg scales
	// to them, so they always match the loopback device's format.
	Width, Height int
	// ReadTimeout ends the run when no datagram arrives for this long, for
	// example after the camera is unplugged. Before the first datagram
	// FirstFrame applies instead, when it is set.
	ReadTimeout time.Duration
	// HWAccel is "vaapi" to decode on the GPU through VAAPI, or "" or
	// "none" to decode in software. ffmpeg does not fall back by itself
	// when VAAPI fails to start, so check it first with ProbeVAAPI.
	HWAccel string
	// FirstFrame, when positive, ends the run with ErrNoVideo if no frame
	// arrives this long after ffmpeg starts, which catches a camera that
	// reports streaming but sends nothing.
	FirstFrame time.Duration
	// OnStats, if set, gets the receiver's counts when the run ends.
	OnStats func(Stats)
	// OnPacket, if set, gets every datagram from the camera as it arrives,
	// for example for a recording. It must not block. Each datagram has a
	// slice of its own that nobody changes, so it may be kept, but not
	// changed: ffmpeg reads the same one.
	OnPacket func([]byte)
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
	if c.Camera.IsValid() && (!c.Camera.Is4() || c.Camera.IsUnspecified() || c.Camera.IsMulticast()) {
		return fmt.Errorf("camera address %s: must be a unicast IPv4 address", c.Camera)
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
	case "", "none", "vaapi":
	default:
		return fmt.Errorf("hardware decoding %q: must be vaapi or none", c.HWAccel)
	}
	return nil
}

// FrameSize is the length in bytes of one yuv420p frame.
func (c Config) FrameSize() int { return c.Width * c.Height * 3 / 2 }

// Args returns ffmpeg's argument list, without the program name.
func (c Config) Args() []string {
	var hw []string
	if c.HWAccel == "vaapi" {
		// Measured 2026-10-05 on an AMD GPU, 1080p30 H.264: 9.5 % of one
		// core instead of 13.7 %, latency 74 ms instead of 71 ms. Not
		// "-hwaccel auto": it tries CUDA first and logs errors on every
		// start where there is no NVIDIA driver.
		hw = []string{"-hwaccel", "vaapi"}
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
		// gpwebcam receives the UDP datagrams and writes them to stdin.
		"-f", "mpegts",
		"-i", "pipe:0",
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

// Run listens for the camera's datagrams, starts ffmpeg on them and passes
// every decoded frame to sink until the run ends. The slice is reused for
// the next frame. Cancelling ctx sends SIGTERM, then SIGKILL after grace; a
// sink error or missing datagrams stop ffmpeg the same way. ffmpeg's stderr
// is copied to logs.
func Run(ctx context.Context, c Config, logs io.Writer, grace time.Duration, sink func([]byte) error) error {
	if err := c.Validate(); err != nil {
		return err
	}
	rcv, err := listen(c.Listen, c.Camera, c.OnPacket)
	if err != nil {
		return err
	}
	defer rcv.close()
	if c.OnStats != nil {
		defer func() { c.OnStats(rcv.stats()) }()
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	r, w, err := os.Pipe()
	if err != nil {
		return err
	}
	inR, inW, err := os.Pipe()
	if err != nil {
		r.Close()
		w.Close()
		return err
	}
	cmd := exec.CommandContext(runCtx, c.FFmpeg, c.Args()...)
	cmd.Stdin = inR
	cmd.Stdout = w
	cmd.Stderr = logs
	cmd.Cancel = func() error {
		// ffmpeg waiting for input on the pipe does not act on SIGTERM;
		// the end of its input makes it exit at once.
		inW.Close()
		return cmd.Process.Signal(syscall.SIGTERM)
	}
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
		inR.Close()
		inW.Close()
		return fmt.Errorf("start ffmpeg: %w", err)
	}
	// ffmpeg holds its ends now: reads of frames see EOF once it exits, and
	// writes of datagrams fail.
	w.Close()
	inR.Close()
	go rcv.read()
	go func() {
		_ = rcv.pump(inW)
		inW.Close()
	}()

	// The camera stops sending when it is unplugged or switched off.
	var lost atomic.Bool
	start := time.Now()
	go func() {
		t := time.NewTicker(c.ReadTimeout / 10)
		defer t.Stop()
		for {
			select {
			case <-runCtx.Done():
				return
			case <-t.C:
			}
			d, ok := rcv.sinceLast()
			if !ok {
				if c.FirstFrame > 0 {
					continue
				}
				d = time.Since(start)
			}
			if d > c.ReadTimeout {
				lost.Store(true)
				cancel()
				return
			}
		}
	}()

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
	rcv.close() // ends read, and with it pump
	switch {
	case ctx.Err() != nil:
		// Stopped on request; ffmpeg's exit status after SIGTERM is noise.
		return nil
	case sinkErr != nil:
		return fmt.Errorf("write frame: %w", sinkErr)
	case noVideo.Load() && rcv.packets.Load() == 0:
		return fmt.Errorf("%w (waited %v)", ErrNoPackets, c.FirstFrame)
	case noVideo.Load():
		return fmt.Errorf("%w: %d datagrams arrived, but ffmpeg decoded no frame (waited %v)", ErrNoVideo, rcv.packets.Load(), c.FirstFrame)
	case lost.Load() && rcv.packets.Load() == 0:
		return fmt.Errorf("%w (waited %v)", ErrNoPackets, c.ReadTimeout)
	case lost.Load():
		return fmt.Errorf("no video from the camera for %v; was it unplugged or switched off?", c.ReadTimeout)
	case err != nil:
		return fmt.Errorf("ffmpeg: %w", err)
	}
	return errors.New("ffmpeg ended by itself")
}

// ProbeVAAPI reports whether ffmpeg can open a VAAPI device, by creating
// one for an empty input. It takes about 50 ms.
func ProbeVAAPI(ctx context.Context, ffmpeg string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, ffmpeg, "-hide_banner", "-nostdin", "-loglevel", "error",
		"-init_hw_device", "vaapi=va", "-f", "lavfi", "-i", "nullsrc=s=64x64",
		"-frames:v", "1", "-f", "null", "-").CombinedOutput()
	if err != nil {
		return fmt.Errorf("VAAPI: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Port parses and range-checks a UDP port given as text.
func Port(s string) (uint16, error) {
	n, err := strconv.ParseUint(s, 10, 16)
	if err != nil || n < 1024 {
		return 0, fmt.Errorf("port %q: must be a number from 1024 to 65535", s)
	}
	return uint16(n), nil
}
