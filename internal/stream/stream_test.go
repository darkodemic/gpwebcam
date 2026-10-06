package stream

import (
	"bytes"
	"context"
	"errors"
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
		Width:       1920,
		Height:      1080,
		ReadTimeout: 10 * time.Second,
	}
}

func TestArgs(t *testing.T) {
	args := testConfig().Args()
	i := slices.Index(args, "-i")
	if i < 0 || i+1 >= len(args) {
		t.Fatalf("no -i in %q", args)
	}
	if args[i+1] != "pipe:0" {
		t.Errorf("input = %q, want pipe:0: gpwebcam receives the datagrams", args[i+1])
	}
	if slices.ContainsFunc(args, func(a string) bool { return strings.Contains(a, "udp:") }) {
		t.Errorf("ffmpeg must not open a UDP socket: %q", args)
	}
	// Low-latency flags must be input options.
	for _, flag := range []string{"-fflags", "-flags", "-analyzeduration", "-f"} {
		if j := slices.Index(args, flag); j < 0 || j > i {
			t.Errorf("%s is missing or placed after -i: %q", flag, args)
		}
	}
	if j := slices.Index(args, "-vf"); j < 0 || args[j+1] != "scale=1920:1080,format=yuv420p" {
		t.Errorf("video filter missing or wrong: %q", args)
	}
	if j := slices.Index(args, "-fps_mode"); j < 0 || j < i || args[j+1] != "passthrough" {
		t.Errorf("output must pass frames through without frame rate conversion: %q", args)
	}
	if !slices.Contains(args, "-flush_packets") || args[len(args)-1] != "pipe:1" {
		t.Errorf("output must be flushed raw video on stdout: %q", args)
	}
}

func TestArgsHWAccel(t *testing.T) {
	c := testConfig()
	if slices.Contains(c.Args(), "-hwaccel") {
		t.Error("-hwaccel without HWAccel")
	}
	c.HWAccel = "vaapi"
	args := c.Args()
	j, i := slices.Index(args, "-hwaccel"), slices.Index(args, "-i")
	if j < 0 || j > i || args[j+1] != "vaapi" {
		t.Errorf("-hwaccel vaapi missing or after -i: %q", args)
	}
	c.HWAccel = "cuda"
	if err := c.Validate(); err == nil {
		t.Error("HWAccel cuda accepted")
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
	for _, cam := range []string{"0.0.0.0", "239.1.1.1", "::1"} {
		c := testConfig()
		c.Camera = netip.MustParseAddr(cam)
		if err := c.Validate(); err == nil {
			t.Errorf("camera %s accepted", cam)
		}
	}
	for _, size := range [][2]int{{0, 1080}, {1920, -2}, {1921, 1080}} {
		c := testConfig()
		c.Width, c.Height = size[0], size[1]
		if err := c.Validate(); err == nil {
			t.Errorf("frame size %v accepted", size)
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

func noFrames(t *testing.T) func([]byte) error {
	return func([]byte) error {
		t.Error("unexpected frame")
		return nil
	}
}

// TestRunTimesOut starts the real ffmpeg with nothing sending: the read
// timeout must end the run with ErrNoPackets.
func TestRunTimesOut(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	c := testConfig()
	c.Listen = netip.MustParseAddrPort("127.0.0.1:47554")
	c.ReadTimeout = 500 * time.Millisecond
	var logs bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := Run(ctx, c, &logs, time.Second, noFrames(t))
	if !errors.Is(err, ErrNoPackets) || ctx.Err() != nil {
		t.Fatalf("Run = %v (ctx %v), want ErrNoPackets before the deadline; logs:\n%s", err, ctx.Err(), logs.String())
	}
}

// TestRunNoFirstFrame: with nothing sending, FirstFrame must end the run
// with ErrNoPackets, which matches ErrNoVideo, before the read timeout.
func TestRunNoFirstFrame(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	c := testConfig()
	c.Listen = netip.MustParseAddrPort("127.0.0.1:47558")
	c.ReadTimeout = 30 * time.Second
	c.FirstFrame = time.Second
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	start := time.Now()
	err := Run(ctx, c, &bytes.Buffer{}, time.Second, noFrames(t))
	if !errors.Is(err, ErrNoVideo) || !errors.Is(err, ErrNoPackets) {
		t.Fatalf("Run = %v, want ErrNoPackets", err)
	}
	if d := time.Since(start); d > 4*time.Second {
		t.Errorf("gave up after %v", d)
	}
}

// TestRunIgnoresOtherSenders: datagrams from an address other than the
// camera's must not reach ffmpeg.
func TestRunIgnoresOtherSenders(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	c := testConfig()
	c.Listen = netip.MustParseAddrPort("127.0.0.1:47559")
	c.Camera = netip.MustParseAddr("127.0.0.9") // the sender is 127.0.0.1
	c.Width, c.Height = 320, 240
	c.ReadTimeout = 30 * time.Second
	c.FirstFrame = 2 * time.Second
	var stats Stats
	c.OnStats = func(s Stats) { stats = s }
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	sendTS(t, ctx, c.Listen, "5")
	err := Run(ctx, c, &bytes.Buffer{}, time.Second, noFrames(t))
	if !errors.Is(err, ErrNoPackets) {
		t.Fatalf("Run = %v, want ErrNoPackets", err)
	}
	if stats.Packets != 0 || stats.Foreign == 0 {
		t.Errorf("stats %+v, want only foreign datagrams", stats)
	}
}

// sendTS sends a few seconds of MPEG-TS to addr, as the camera does.
func sendTS(t *testing.T, ctx context.Context, addr netip.AddrPort, seconds string) {
	t.Helper()
	sender := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-nostdin", "-loglevel", "error",
		"-re", "-f", "lavfi", "-i", "testsrc2=size=320x240:rate=30", "-t", seconds,
		"-c:v", "mpeg2video", "-g", "15", "-f", "mpegts", "udp://"+addr.String()+"?pkt_size=1316")
	if err := sender.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sender.Wait() })
}

// TestRunStreamStops feeds a short stream and then stops sending, as an
// unplugged camera does: frames of the configured size must reach the
// sink, then ffmpeg must end on its read timeout and Run must say that the
// video stopped.
func TestRunStreamStops(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	c := testConfig()
	c.Listen = netip.MustParseAddrPort("127.0.0.1:47556")
	c.Width, c.Height = 640, 360 // ffmpeg scales the 320x240 input
	// GPU decoding where VAAPI works, software elsewhere: frames must
	// arrive either way.
	if ProbeVAAPI(context.Background(), "ffmpeg") == nil {
		c.HWAccel = "vaapi"
	}
	c.ReadTimeout = 500 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	sendTS(t, ctx, c.Listen, "3")

	frames := 0
	var stats Stats
	c.OnStats = func(s Stats) { stats = s }
	err := Run(ctx, c, &bytes.Buffer{}, time.Second, func(f []byte) error {
		if len(f) != 640*360*3/2 {
			t.Errorf("frame is %d bytes", len(f))
		}
		frames++
		return nil
	})
	if err == nil || ctx.Err() != nil || !strings.Contains(err.Error(), "no video from the camera") {
		t.Fatalf("Run = %v (ctx %v), want the stream-stopped error", err, ctx.Err())
	}
	if frames < 30 {
		t.Errorf("got %d frames from a 3 s stream", frames)
	}
	if stats.Packets == 0 || stats.Bytes == 0 || stats.Dropped != 0 {
		t.Errorf("stats %+v, want datagrams and no drops", stats)
	}
}

// TestRunSinkError checks that a failing sink stops ffmpeg and is reported.
func TestRunSinkError(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	c := testConfig()
	c.Listen = netip.MustParseAddrPort("127.0.0.1:47557")
	c.Width, c.Height = 320, 240
	c.ReadTimeout = 5 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	sendTS(t, ctx, c.Listen, "10")

	errGone := errors.New("device gone")
	start := time.Now()
	err := Run(ctx, c, &bytes.Buffer{}, time.Second, func([]byte) error { return errGone })
	if !errors.Is(err, errGone) {
		t.Fatalf("Run = %v, want the sink error", err)
	}
	if d := time.Since(start); d > 8*time.Second {
		t.Errorf("stopping after a sink error took %v", d)
	}
}

// TestRunCancel checks that cancelling the context stops ffmpeg cleanly.
func TestRunCancel(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	c := testConfig()
	c.Listen = netip.MustParseAddrPort("127.0.0.1:47555")
	c.ReadTimeout = time.Minute
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(300*time.Millisecond, cancel)
	start := time.Now()
	if err := Run(ctx, c, &bytes.Buffer{}, 2*time.Second, noFrames(t)); err != nil {
		t.Fatalf("Run after cancel = %v, want nil", err)
	}
	// ffmpeg ignores SIGTERM while it waits on its input pipe; closing
	// the pipe must stop it well before the 2 s grace period.
	if d := time.Since(start); d > 1500*time.Millisecond {
		t.Errorf("stopping took %v", d)
	}
}
