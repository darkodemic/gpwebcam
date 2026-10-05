package stream

import (
	"bytes"
	"context"
	"io"
	"net/netip"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"
)

func testConfig() Config {
	return Config{
		FFmpeg:      "ffmpeg",
		Listen:      netip.MustParseAddrPort("172.25.187.52:8554"),
		Device:      "/dev/video42",
		ReadTimeout: 10 * time.Second,
	}
}

func TestArgs(t *testing.T) {
	args := testConfig().Args()
	i := slices.Index(args, "-i")
	if i < 0 || i+1 >= len(args) {
		t.Fatalf("no -i in %q", args)
	}
	if want := "udp://172.25.187.52:8554?timeout=10000000&overrun_nonfatal=1"; args[i+1] != want {
		t.Errorf("input URL = %q, want %q", args[i+1], want)
	}
	// Low-latency flags must be input options.
	for _, flag := range []string{"-fflags", "-flags", "-analyzeduration", "-f"} {
		if j := slices.Index(args, flag); j < 0 || j > i {
			t.Errorf("%s is missing or placed after -i: %q", flag, args)
		}
	}
	if args[len(args)-1] != "/dev/video42" {
		t.Errorf("last argument = %q, want the device", args[len(args)-1])
	}
}

func TestValidate(t *testing.T) {
	if err := testConfig().Validate(); err != nil {
		t.Fatalf("valid config: %v", err)
	}
	for _, listen := range []string{"0.0.0.0:8554", "239.1.1.1:8554", "[::1]:8554", "172.25.187.52:0"} {
		c := testConfig()
		c.Listen = netip.MustParseAddrPort(listen)
		if err := c.Validate(); err == nil {
			t.Errorf("listen %s accepted", listen)
		}
	}
}

func TestPort(t *testing.T) {
	if p, err := Port("8554"); err != nil || p != 8554 {
		t.Errorf("Port(8554) = %d, %v", p, err)
	}
	for _, bad := range []string{"", "0", "80", "65536", "-1", "8554 ", "0x2000", "1e4"} {
		if _, err := Port(bad); err == nil {
			t.Errorf("Port(%q) accepted", bad)
		}
	}
}

// TestRunTimesOut starts the real ffmpeg on loopback with nothing sending to
// it: the read timeout must end the run with an error.
func TestRunTimesOut(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	c := testConfig()
	c.Listen = netip.MustParseAddrPort("127.0.0.1:47554")
	c.Device = t.TempDir() + "/out"
	c.ReadTimeout = 500 * time.Millisecond
	var logs bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := Run(ctx, c, &logs, time.Second)
	if err == nil || ctx.Err() != nil {
		t.Fatalf("Run = %v (ctx %v), want an ffmpeg error before the deadline; logs:\n%s", err, ctx.Err(), logs.String())
	}
	if !strings.Contains(err.Error(), "ffmpeg") {
		t.Errorf("error %q does not name ffmpeg", err)
	}
}

// TestRunStreamStops feeds a short MPEG-TS stream over UDP and then stops
// sending, as an unplugged camera does: ffmpeg must end on its read timeout
// and Run must say that the video stopped.
func TestRunStreamStops(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	c := testConfig()
	c.Listen = netip.MustParseAddrPort("127.0.0.1:47556")
	c.Device = "-" // not a device: write raw frames to a discarded stdout
	c.ReadTimeout = 500 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	sender := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-nostdin", "-loglevel", "error",
		"-re", "-f", "lavfi", "-i", "testsrc2=size=320x240:rate=30", "-t", "3",
		"-c:v", "mpeg2video", "-g", "15", "-f", "mpegts", "udp://"+c.Listen.String()+"?pkt_size=1316")
	if err := sender.Start(); err != nil {
		t.Fatal(err)
	}
	defer sender.Wait()

	args := c.Args()
	args[len(args)-2] = "rawvideo" // replace "-f v4l2"
	err := runArgs(ctx, c.FFmpeg, args, io.Discard, time.Second, c.ReadTimeout)
	if err == nil || ctx.Err() != nil || !strings.Contains(err.Error(), "no video from the camera") {
		t.Fatalf("Run = %v (ctx %v), want the stream-stopped error", err, ctx.Err())
	}
}

// TestRunCancel checks that cancelling the context stops ffmpeg cleanly.
func TestRunCancel(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	c := testConfig()
	c.Listen = netip.MustParseAddrPort("127.0.0.1:47555")
	c.Device = t.TempDir() + "/out"
	c.ReadTimeout = time.Minute
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(300*time.Millisecond, cancel)
	start := time.Now()
	if err := Run(ctx, c, &bytes.Buffer{}, 2*time.Second); err != nil {
		t.Fatalf("Run after cancel = %v, want nil", err)
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Errorf("stopping took %v", d)
	}
}
