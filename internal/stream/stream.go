// Package stream runs ffmpeg, which receives the camera's MPEG-TS stream over
// UDP and writes raw frames into a v4l2loopback device.
package stream

import (
	"context"
	"fmt"
	"io"
	"net/netip"
	"os/exec"
	"runtime"
	"strconv"
	"syscall"
	"time"
)

// Config describes one ffmpeg run. Every field is validated by the caller's
// types or by Validate, so nothing unchecked reaches the argument list.
type Config struct {
	FFmpeg string         // ffmpeg executable, looked up in PATH if it has no slash
	Listen netip.AddrPort // host address on the GoPro link; never unspecified
	Device string         // e.g. /dev/video42
	// ReadTimeout makes ffmpeg exit when no packet arrives for this long,
	// for example after the camera is unplugged. While opening the input,
	// ffmpeg reads several times, so with no stream at all it gives up
	// after about four timeouts (measured with ffmpeg 9.0.2).
	ReadTimeout time.Duration
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
	if c.Device == "" || c.FFmpeg == "" {
		return fmt.Errorf("device and ffmpeg path must be set")
	}
	return nil
}

// Args returns ffmpeg's argument list, without the program name.
func (c Config) Args() []string {
	// For input, ffmpeg binds the UDP socket to the host in the URL
	// (checked with ffmpeg 9.0.2), so only the GoPro link can feed it.
	// timeout is in microseconds and applies to reads only.
	url := fmt.Sprintf("udp://%s?timeout=%d&overrun_nonfatal=1",
		c.Listen, c.ReadTimeout.Microseconds())
	return []string{
		"-hide_banner",
		"-nostdin",
		"-loglevel", "warning",
		// Input options go before -i, or ffmpeg applies them to the output.
		// nobuffer drops the packets read while probing instead of queueing
		// them, so the first frame shown is current. With the default 5 s
		// analyzeduration that loses the first 6 s; 1 s brings it to about
		// 2 s (measured with ffmpeg 9.0.2 on a 1080p30 H.264 + AAC TS).
		"-fflags", "nobuffer",
		"-flags", "low_delay",
		"-analyzeduration", "1000000",
		"-f", "mpegts",
		"-i", url,
		// The TS also carries AAC, an empty AC3 track and a private data
		// stream; only the video goes to the loopback device.
		"-map", "0:v:0",
		"-vf", "format=yuv420p",
		"-f", "v4l2",
		c.Device,
	}
}

// Run starts ffmpeg and waits for it to exit. Cancelling ctx sends SIGTERM,
// then SIGKILL after grace. ffmpeg's stderr is copied to logs.
func Run(ctx context.Context, c Config, logs io.Writer, grace time.Duration) error {
	if err := c.Validate(); err != nil {
		return err
	}
	return runArgs(ctx, c.FFmpeg, c.Args(), logs, grace, c.ReadTimeout)
}

func runArgs(ctx context.Context, ffmpeg string, args []string, logs io.Writer, grace, readTimeout time.Duration) error {
	cmd := exec.CommandContext(ctx, ffmpeg, args...)
	cmd.Stdout = logs
	cmd.Stderr = logs
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
	cmd.WaitDelay = grace
	// Die with gw even if gw is killed without a chance to clean up.
	// Pdeathsig fires when the forking OS thread exits, so keep this
	// goroutine on that thread until ffmpeg is gone.
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start ffmpeg: %w", err)
	}
	err := cmd.Wait()
	if ctx.Err() != nil {
		// Stopped on request; ffmpeg's exit status after SIGTERM is noise.
		return nil
	}
	if err != nil {
		return fmt.Errorf("ffmpeg: %w", err)
	}
	// With UDP input ffmpeg ends cleanly only when the read timeout fires.
	return fmt.Errorf("no video from the camera for %v; was it unplugged or switched off?", readTimeout)
}

// Port parses and range-checks a UDP port given as text.
func Port(s string) (uint16, error) {
	n, err := strconv.ParseUint(s, 10, 16)
	if err != nil || n < 1024 {
		return 0, fmt.Errorf("port %q: must be a number from 1024 to 65535", s)
	}
	return uint16(n), nil
}
