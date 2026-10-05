package camera

import (
	"fmt"
	"strings"
)

// Resolution is the webcam stream resolution. Only the listed values exist;
// Set rejects anything else. 480p is left out: the spec lists it for HERO9
// and HERO10 only.
type Resolution string

const (
	Res1080 Resolution = "1080"
	Res720  Resolution = "720"
)

var resolutions = []Resolution{Res1080, Res720}

// code is the value of the res parameter of /gopro/webcam/start.
func (r Resolution) code() int {
	switch r {
	case Res720:
		return 7
	default:
		return 12
	}
}

func (r *Resolution) String() string { return string(*r) }

// Set implements flag.Value.
func (r *Resolution) Set(s string) error {
	for _, v := range resolutions {
		if s == string(v) {
			*r = v
			return nil
		}
	}
	return fmt.Errorf("resolution %q: must be one of %s", s, join(resolutions))
}

// FOV is the webcam field of view (digital lens).
type FOV string

const (
	FOVWide      FOV = "wide"
	FOVNarrow    FOV = "narrow"
	FOVSuperView FOV = "superview"
	FOVLinear    FOV = "linear"
)

var fovs = []FOV{FOVWide, FOVNarrow, FOVSuperView, FOVLinear}

// code is the value of the fov parameter of /gopro/webcam/start
// (setting 43, Webcam Digital Lenses).
func (f FOV) code() int {
	switch f {
	case FOVNarrow:
		return 2
	case FOVSuperView:
		return 3
	case FOVLinear:
		return 4
	default:
		return 0
	}
}

func (f *FOV) String() string { return string(*f) }

// Set implements flag.Value.
func (f *FOV) Set(s string) error {
	for _, v := range fovs {
		if s == string(v) {
			*f = v
			return nil
		}
	}
	return fmt.Errorf("fov %q: must be one of %s", s, join(fovs))
}

func join[T ~string](vs []T) string {
	s := make([]string, len(vs))
	for i, v := range vs {
		s[i] = string(v)
	}
	return strings.Join(s, ", ")
}
