package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestH264Decoders(t *testing.T) {
	out := `Decoders:
 V..... = Video
 ------
 VFS..D h264                 H.264 / AVC / MPEG-4 AVC / MPEG-4 part 10
 V..... h264_v4l2m2m         V4L2 mem2mem H.264 decoder wrapper (codec h264)
 V....D h264_qsv             H264 video (Intel Quick Sync Video acceleration) (codec h264)
 V....D libopenh264          OpenH264 H.264 / AVC / MPEG-4 AVC / MPEG-4 part 10 (codec h264)
 A....D aac                  AAC (Advanced Audio Coding)
`
	if got := h264Decoders(out); !reflect.DeepEqual(got, []string{"h264", "libopenh264"}) {
		t.Errorf("h264Decoders = %q", got)
	}
	if got := h264Decoders(" V....D hevc  HEVC\n"); len(got) != 0 {
		t.Errorf("h264Decoders(hevc only) = %q", got)
	}
}

func TestOptionLines(t *testing.T) {
	out := "alias char_major_10_255 v4l2loopback\noptions v4l2loopback devices=2 video_nr=42,-1\noptions snd_hda_intel power_save=1\n"
	if got := optionLines(out); !reflect.DeepEqual(got, []string{"options v4l2loopback devices=2 video_nr=42,-1"}) {
		t.Errorf("optionLines = %q", got)
	}
}

func TestLoopbackDevices(t *testing.T) {
	root := t.TempDir()
	add := func(class, real, name string) {
		t.Helper()
		dir := filepath.Join(root, real)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		os.WriteFile(filepath.Join(dir, "name"), []byte(name+"\n"), 0o644)
		link := filepath.Join(root, "class", "video4linux", class)
		os.MkdirAll(filepath.Dir(link), 0o755)
		if err := os.Symlink(dir, link); err != nil {
			t.Fatal(err)
		}
	}
	add("video0", "devices/pci0000:00/usb5/5-3/video4linux/video0", "UVC Camera")
	add("video42", "devices/virtual/video4linux/video42", "GoPro")
	add("video43", "devices/virtual/video4linux/video43", "OBS Virtual Camera")
	got := loopbackDevices(root)
	want := []loopDev{{"/dev/video42", "GoPro"}, {"/dev/video43", "OBS Virtual Camera"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("loopbackDevices = %+v", got)
	}
}
