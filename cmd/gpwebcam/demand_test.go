package main

import (
	"path/filepath"
	"testing"

	"github.com/darkodemic/gpwebcam/internal/settings"
)

func TestWantCamera(t *testing.T) {
	f, err := parseStart("run", []string{"-ffmpeg", "/bin/true"})
	if err != nil {
		t.Fatal(err)
	}
	s := &server{live: newLive(filepath.Join(t.TempDir(), settings.FileName), f, quietLog()), wake: make(chan struct{}, 1)}
	for _, tc := range []struct {
		mode          string
		usageOK, used bool
		want          bool
	}{
		{settings.CameraDemand, true, false, false},
		{settings.CameraDemand, true, true, true},
		{settings.CameraDemand, false, false, true}, // no usage events: like always
		{settings.CameraAlways, true, false, true},
		{settings.CameraOff, true, true, false},
		{settings.CameraOff, false, false, false},
	} {
		if err := s.live.Set("camera", tc.mode); err != nil {
			t.Fatal(err)
		}
		s.usageOK.Store(tc.usageOK)
		s.used.Store(tc.used)
		if got := s.wantCamera(); got != tc.want {
			t.Errorf("mode %s, usage events %t, used %t: wantCamera = %t, want %t", tc.mode, tc.usageOK, tc.used, got, tc.want)
		}
	}
}
