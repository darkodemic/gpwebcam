package settings

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkodemic/gpwebcam/internal/camera"
)

func TestLoadMissingFileGivesDefaults(t *testing.T) {
	s, err := Load(filepath.Join(t.TempDir(), FileName))
	if err != nil {
		t.Fatal(err)
	}
	if s != Defaults() {
		t.Errorf("got %+v, want defaults %+v", s, Defaults())
	}
}

func TestSaveLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", FileName)
	want := Settings{Camera: CameraOff, Res: camera.Res720, FOV: camera.FOVWide, HWDec: "none", Notify: false, Tray: false}
	if err := Save(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Errorf("mode %v, want 0644", info.Mode().Perm())
	}
	left, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".settings-*"))
	if len(left) > 0 {
		t.Errorf("temporary files left: %v", left)
	}
}

func TestLoadPartialFileKeepsDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	if err := os.WriteFile(path, []byte(`{"fov": "wide"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	want := Defaults()
	want.FOV = camera.FOVWide
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestLoadRejects(t *testing.T) {
	for _, tc := range []struct{ name, data, want string }{
		{"bad fov", `{"fov": "fisheye"}`, "fov"},
		{"bad res", `{"res": "4k"}`, "resolution"},
		{"bad hwdec", `{"hwdec": "cuda"}`, "hwdec"},
		{"bad camera", `{"camera": "sometimes"}`, "camera"},
		{"wrong type", `{"tray": "yes"}`, "tray"},
		{"not json", `res=720`, "invalid character"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), FileName)
			if err := os.WriteFile(path, []byte(tc.data), 0o644); err != nil {
				t.Fatal(err)
			}
			s, err := Load(path)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want it to mention %q", err, tc.want)
			}
			if s != Defaults() {
				t.Errorf("a bad file gave %+v, want defaults", s)
			}
		})
	}
}

func TestUnknownKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	if err := os.WriteFile(path, []byte(`{"fov": "wide", "fps": 60, "zoom": "x"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := Load(path)
	var uk *UnknownKeysError
	if !errors.As(err, &uk) || strings.Join(uk.Keys, ",") != "fps,zoom" {
		t.Fatalf("err = %v, want unknown fps and zoom", err)
	}
	if s.FOV != camera.FOVWide {
		t.Errorf("known setting not loaded next to unknown ones: %+v", s)
	}

	// Saving keeps them, for the version that knows them.
	s.Tray = false
	if err := Save(path, s); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	for _, want := range []string{`"fps": 60`, `"zoom": "x"`, `"tray": false`, `"fov": "wide"`} {
		if !strings.Contains(string(data), want) {
			t.Errorf("saved file lacks %s:\n%s", want, data)
		}
	}
}

func TestLoadTooLarge(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	if err := os.WriteFile(path, []byte(strings.Repeat(" ", maxFile+1)), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Error("no error for an oversized file")
	}
}

func TestSaveRejectsInvalid(t *testing.T) {
	s := Defaults()
	s.FOV = "fisheye"
	path := filepath.Join(t.TempDir(), FileName)
	if err := Save(path, s); err == nil {
		t.Fatal("saved an invalid FOV")
	}
	if _, err := os.Stat(path); err == nil {
		t.Error("file written despite the error")
	}
}

func TestSetGet(t *testing.T) {
	s := Defaults()
	for _, tc := range []struct{ key, in, out string }{
		{"camera", "always", "always"},
		{"camera", "off", "off"},
		{"res", "720", "720"},
		{"fov", "superview", "superview"},
		{"hwdec", "none", "none"},
		{"notify", "off", "off"},
		{"notify", "true", "on"},
		{"tray", "0", "off"},
		{"tray", "on", "on"},
	} {
		if err := s.Set(tc.key, tc.in); err != nil {
			t.Errorf("Set(%s, %s): %v", tc.key, tc.in, err)
			continue
		}
		if got := s.Get(tc.key); got != tc.out {
			t.Errorf("after Set(%s, %s), Get = %q, want %q", tc.key, tc.in, got, tc.out)
		}
	}
	for _, tc := range []struct{ key, in string }{
		{"camera", "on"}, {"res", "4k"}, {"fov", ""}, {"hwdec", "cuda"}, {"notify", "maybe"}, {"fps", "60"},
	} {
		before := s
		if err := s.Set(tc.key, tc.in); err == nil {
			t.Errorf("Set(%s, %q) accepted", tc.key, tc.in)
		}
		if s != before {
			t.Errorf("Set(%s, %q) changed the settings despite the error", tc.key, tc.in)
		}
	}
}

func TestDir(t *testing.T) {
	t.Setenv("CONFIGURATION_DIRECTORY", "/run/cfg/gpwebcam")
	t.Setenv("XDG_CONFIG_HOME", "/xdg")
	if d, _ := Dir(); d != "/run/cfg/gpwebcam" {
		t.Errorf("with CONFIGURATION_DIRECTORY: %s", d)
	}
	// Several directories, or a relative one, are not ours.
	t.Setenv("CONFIGURATION_DIRECTORY", "/a:/b")
	if d, _ := Dir(); d != "/xdg/gpwebcam" {
		t.Errorf("with a list in CONFIGURATION_DIRECTORY: %s", d)
	}
	t.Setenv("CONFIGURATION_DIRECTORY", "")
	t.Setenv("XDG_CONFIG_HOME", "relative")
	t.Setenv("HOME", "/home/u")
	if d, _ := Dir(); d != "/home/u/.config/gpwebcam" {
		t.Errorf("with a relative XDG_CONFIG_HOME: %s", d)
	}
}
