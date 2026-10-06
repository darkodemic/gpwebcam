package record

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// cameraLikeTS makes a few seconds of MPEG-TS with a video and an audio
// stream, as the camera sends, cut into 1316-byte datagrams.
func cameraLikeTS(t *testing.T) [][]byte {
	t.Helper()
	out, err := exec.Command("ffmpeg", "-hide_banner", "-nostdin", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc2=size=320x240:rate=30", "-f", "lavfi", "-i", "anullsrc",
		"-t", "3", "-c:v", "mpeg2video", "-g", "15", "-c:a", "aac", "-f", "mpegts", "pipe:1").Output()
	if err != nil {
		t.Fatal(err)
	}
	var ps [][]byte
	for len(out) > 0 {
		n := min(1316, len(out))
		ps = append(ps, out[:n])
		out = out[n:]
	}
	return ps
}

func TestRecord(t *testing.T) {
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	dir := filepath.Join(t.TempDir(), "recordings")
	var logs bytes.Buffer
	r, err := Start("ffmpeg", dir, &logs)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(r.Path()) != dir || !strings.HasPrefix(filepath.Base(r.Path()), "GoPro-") {
		t.Errorf("recording at %s", r.Path())
	}
	for _, p := range cameraLikeTS(t) {
		r.Packet(p)
	}
	if err := r.Stop(10 * time.Second); err != nil {
		t.Fatalf("Stop: %v\n%s", err, logs.String())
	}
	r.Packet([]byte("after stop")) // must not panic

	out, err := exec.Command("ffprobe", "-v", "error", "-show_entries", "format=format_name,duration:stream=codec_type",
		"-of", "default=noprint_wrappers=1", r.Path()).Output()
	if err != nil {
		t.Fatalf("ffprobe: %v", err)
	}
	info := string(out)
	if !strings.Contains(info, "format_name=matroska") || strings.Count(info, "codec_type=") != 1 || !strings.Contains(info, "codec_type=video") {
		t.Errorf("recording is not Matroska with only video:\n%s", info)
	}
	if !strings.Contains(info, "duration=2.") && !strings.Contains(info, "duration=3.") {
		t.Errorf("recording does not last about 3 s:\n%s", info)
	}
}

func TestStartRefuses(t *testing.T) {
	if _, err := Start("ffmpeg", "relative/dir", &bytes.Buffer{}); err == nil {
		t.Error("relative folder accepted")
	}
	// A folder that cannot be created.
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Start("ffmpeg", filepath.Join(file, "sub"), &bytes.Buffer{}); err == nil {
		t.Error("folder under a file accepted")
	}
	// ffmpeg missing: the empty file must not stay behind.
	dir := t.TempDir()
	if _, err := Start(filepath.Join(dir, "no-ffmpeg"), dir, &bytes.Buffer{}); err == nil {
		t.Error("missing ffmpeg accepted")
	}
	if left, _ := filepath.Glob(filepath.Join(dir, "GoPro-*")); len(left) > 0 {
		t.Errorf("files left after a failed start: %v", left)
	}
}

func TestFileNames(t *testing.T) {
	dir := t.TempDir()
	at := time.Date(2026, 10, 6, 23, 50, 12, 0, time.Local)
	a, err := create(dir, at)
	if err != nil {
		t.Fatal(err)
	}
	b, err := create(dir, at)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(a) != "GoPro-2026-10-06-235012.mkv" || filepath.Base(b) != "GoPro-2026-10-06-235012-2.mkv" {
		t.Errorf("names %s and %s", filepath.Base(a), filepath.Base(b))
	}
}

func TestSizeText(t *testing.T) {
	for n, want := range map[uint64]string{
		512:           "512 bytes",
		52153:         "52.2 kB",
		9165466:       "9.2 MB", // a recording that Nautilus shows as 9.2 MB
		13363560:      "13.4 MB",
		1_000_000_000: "1.0 GB",
		2_700_000_000: "2.7 GB",
	} {
		if got := SizeText(n); got != want {
			t.Errorf("SizeText(%d) = %q, want %q", n, got, want)
		}
	}
}
