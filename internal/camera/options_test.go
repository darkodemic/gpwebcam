package camera

import "testing"

func TestResolutionFor(t *testing.T) {
	for _, tc := range []struct {
		w, h int
		want Resolution
	}{
		{1920, 1080, Res1080},
		{1280, 720, Res720},
		{640, 480, Res720},
		{3840, 2160, Res1080},
	} {
		if got := ResolutionFor(tc.w, tc.h); got != tc.want {
			t.Errorf("ResolutionFor(%d, %d) = %s, want %s", tc.w, tc.h, got, tc.want)
		}
	}
}
