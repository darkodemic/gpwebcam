package stream

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"net/netip"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"
)

// Latency test setup: a local ffmpeg sends H.264 in MPEG-TS over UDP, as
// the camera does, with the frame number drawn into each frame as a row of
// latencyBits white or black squares. The sender logs each frame's wall
// clock time through showinfo; the sink reads the number back from the
// pixels and takes the difference.
const (
	latencyW, latencyH = 640, 360
	latencyBits        = 12
	latencyBlock       = 32 // edge of one bit's square, in pixels
	latencyFPS         = 30
	latencySeconds     = 15
	latencyWarmup      = 2 * latencyFPS // frames left out while things settle
)

var showinfoLine = regexp.MustCompile(`^(\d{4}-\d\d-\d\d \d\d:\d\d:\d\d\.\d{3}) .*Parsed_showinfo.* n:\s*(\d+) `)

// TestLatency measures the delay from the sender to Run's sink. It needs
// ffmpeg with libx264 and takes about 20 s, so it runs only by hand:
//
//	GPWEBCAM_LATENCY=1 go test -run TestLatency -v ./internal/stream
//
// GPWEBCAM_LATENCY_HW=vaapi decodes on the GPU. The numbers include the
// sender's encoding, the same for every version of Run, so they compare
// versions of the receiving side rather than state an absolute latency.
func TestLatency(t *testing.T) {
	if os.Getenv("GPWEBCAM_LATENCY") == "" {
		t.Skip("set GPWEBCAM_LATENCY=1 to measure")
	}
	c := Config{
		FFmpeg:      "ffmpeg",
		Listen:      netip.MustParseAddrPort("127.0.0.1:47560"),
		Width:       latencyW,
		Height:      latencyH,
		ReadTimeout: 2 * time.Second,
		HWAccel:     os.Getenv("GPWEBCAM_LATENCY_HW"),
	}
	ctx, cancel := context.WithTimeout(context.Background(), (latencySeconds+15)*time.Second)
	defer cancel()

	graph := fmt.Sprintf("color=c=black:s=%dx%d:r=%d", latencyW, latencyH, latencyFPS)
	for k := 0; k < latencyBits; k++ {
		graph += fmt.Sprintf(",drawbox=x=%d:y=0:w=%d:h=%d:color=white:t=fill:enable='mod(floor(n/%d),2)'",
			k*latencyBlock, latencyBlock, latencyBlock, 1<<k)
	}
	graph += ",showinfo"
	sender := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-nostdin", "-loglevel", "+datetime+info",
		"-re", "-f", "lavfi", "-i", graph, "-t", strconv.Itoa(latencySeconds),
		"-c:v", "libx264", "-preset", "ultrafast", "-tune", "zerolatency", "-g", "30", "-pix_fmt", "yuv420p",
		"-f", "mpegts", "udp://"+c.Listen.String()+"?pkt_size=1316")
	var senderLog bytes.Buffer
	sender.Stderr = &senderLog

	var mu sync.Mutex
	arrived := map[int]time.Time{}
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, c, &bytes.Buffer{}, time.Second, func(f []byte) error {
			now := time.Now()
			n := 0
			for k := 0; k < latencyBits; k++ {
				// The middle of bit k's square in the luma plane.
				if f[(latencyBlock/2)*latencyW+k*latencyBlock+latencyBlock/2] > 128 {
					n |= 1 << k
				}
			}
			mu.Lock()
			if _, seen := arrived[n]; !seen {
				arrived[n] = now
			}
			mu.Unlock()
			return nil
		})
	}()
	time.Sleep(300 * time.Millisecond) // let Run listen before the first datagram
	if err := sender.Run(); err != nil {
		t.Fatalf("sender: %v\n%s", err, senderLog.String())
	}
	if err := <-done; err == nil {
		t.Log("Run ended without an error")
	}

	sent := map[int]time.Time{}
	sc := bufio.NewScanner(&senderLog)
	for sc.Scan() {
		m := showinfoLine.FindStringSubmatch(sc.Text())
		if m == nil {
			continue
		}
		at, err := time.ParseInLocation("2006-01-02 15:04:05.000", m[1], time.Local)
		if err != nil {
			continue
		}
		n, _ := strconv.Atoi(m[2])
		sent[n] = at
	}
	var lat []time.Duration
	for n, at := range arrived {
		if s, ok := sent[n]; ok && n >= latencyWarmup {
			lat = append(lat, at.Sub(s))
		}
	}
	if len(lat) < latencySeconds*latencyFPS/2 {
		t.Fatalf("only %d frames matched (%d sent, %d arrived)", len(lat), len(sent), len(arrived))
	}
	slices.Sort(lat)
	pct := func(p int) time.Duration { return lat[len(lat)*p/100] }
	t.Logf("hwaccel %q, %d frames: median %v, p10 %v, p90 %v, max %v",
		c.HWAccel, len(lat), pct(50).Round(time.Millisecond), pct(10).Round(time.Millisecond),
		pct(90).Round(time.Millisecond), lat[len(lat)-1].Round(time.Millisecond))
}
