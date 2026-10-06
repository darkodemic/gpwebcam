// Package settings holds the choices a user can change while gpwebcam runs,
// from the tray menu or with "gpwebcam config". They are saved as JSON in
// the user's configuration directory.
package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/darkodemic/gpwebcam/internal/camera"
)

// Settings are the user's choices. Flags of the same name override them.
type Settings struct {
	// Camera says when the camera streams: "demand" while an application
	// uses the device, "always" while it is connected, "off" never.
	Camera string            `json:"camera"`
	Res    camera.Resolution `json:"res"`
	FOV    camera.FOV        `json:"fov"`
	HWDec  string            `json:"hwdec"`
	Notify bool              `json:"notify"`
	Tray   bool              `json:"tray"`
}

// Keys are the setting names, in the order "gpwebcam config" prints them.
// Each is also the name of the flag that overrides it.
var Keys = []string{"camera", "res", "fov", "hwdec", "notify", "tray"}

// Camera modes.
const (
	CameraDemand = "demand"
	CameraAlways = "always"
	CameraOff    = "off"
)

// FileName is the name of the settings file in the configuration directory.
const FileName = "settings.json"

// maxFile bounds what Load reads; the real file is about 100 bytes.
const maxFile = 64 << 10

// Defaults are the settings without a file.
func Defaults() Settings {
	return Settings{Camera: CameraDemand, Res: camera.Res1080, FOV: camera.FOVLinear, HWDec: "auto", Notify: true, Tray: true}
}

// Validate reports the first value that is not allowed. JSON decoding does
// not go through camera's flag.Value checks, so Load calls this.
func (s Settings) Validate() error {
	for _, k := range []string{"camera", "res", "fov", "hwdec"} {
		var probe Settings
		if err := probe.Set(k, s.Get(k)); err != nil {
			return err
		}
	}
	return nil
}

// Get returns one setting as text.
func (s Settings) Get(key string) string {
	switch key {
	case "camera":
		return s.Camera
	case "res":
		return string(s.Res)
	case "fov":
		return string(s.FOV)
	case "hwdec":
		return s.HWDec
	case "notify":
		return onOff(s.Notify)
	case "tray":
		return onOff(s.Tray)
	}
	return ""
}

// Set parses and checks one setting given as text.
func (s *Settings) Set(key, value string) error {
	switch key {
	case "camera":
		if value != CameraDemand && value != CameraAlways && value != CameraOff {
			return fmt.Errorf("camera %q: must be demand, always or off", value)
		}
		s.Camera = value
		return nil
	case "res":
		return s.Res.Set(value)
	case "fov":
		return s.FOV.Set(value)
	case "hwdec":
		if value != "auto" && value != "none" {
			return fmt.Errorf("hwdec %q: must be auto or none", value)
		}
		s.HWDec = value
		return nil
	case "notify":
		return parseBool(key, value, &s.Notify)
	case "tray":
		return parseBool(key, value, &s.Tray)
	}
	return fmt.Errorf("unknown setting %q; settings are %s", key, strings.Join(Keys, ", "))
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

func parseBool(key, value string, dst *bool) error {
	switch value {
	case "on":
		*dst = true
		return nil
	case "off":
		*dst = false
		return nil
	}
	b, err := strconv.ParseBool(value)
	if err != nil {
		return fmt.Errorf("%s %q: must be on or off", key, value)
	}
	*dst = b
	return nil
}

// Dir returns the configuration directory: $CONFIGURATION_DIRECTORY, which
// systemd sets for the user service (ConfigurationDirectory=gpwebcam), else
// $XDG_CONFIG_HOME/gpwebcam, else ~/.config/gpwebcam. For a user service
// systemd derives the first from the second, so the service and the
// command line use the same file.
func Dir() (string, error) {
	if d := os.Getenv("CONFIGURATION_DIRECTORY"); d != "" && filepath.IsAbs(d) && !strings.Contains(d, ":") {
		return d, nil
	}
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" && filepath.IsAbs(d) {
		return filepath.Join(d, "gpwebcam"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("settings directory: %w", err)
	}
	return filepath.Join(home, ".config", "gpwebcam"), nil
}

// Path returns the settings file path.
func Path() (string, error) {
	d, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, FileName), nil
}

// UnknownKeysError names keys of the settings file that this gpwebcam does
// not know: a typo, or a setting of another version. Load returns it
// together with the settings it knows, and Save keeps such keys, so an
// older and a newer gpwebcam can share the file.
type UnknownKeysError struct {
	Path string
	Keys []string
}

func (e *UnknownKeysError) Error() string {
	return fmt.Sprintf("%s: unknown settings ignored: %s", e.Path, strings.Join(e.Keys, ", "))
}

func known(key string) bool {
	for _, k := range Keys {
		if k == key {
			return true
		}
	}
	return false
}

// readFile returns the file's content, or nil when there is no file.
func readFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxFile+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxFile {
		return nil, fmt.Errorf("%s: larger than %d bytes", path, maxFile)
	}
	return data, nil
}

// Load reads the settings file. A missing file gives the defaults, and so
// does a missing key, so a file can hold only what the user changed. A file
// that is not JSON or has a bad value gives the defaults and an error;
// unknown keys give the known settings and an *UnknownKeysError.
func Load(path string) (Settings, error) {
	data, err := readFile(path)
	if err != nil || data == nil {
		return Defaults(), err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return Defaults(), fmt.Errorf("%s: %w", path, err)
	}
	s := Defaults()
	if err := json.Unmarshal(data, &s); err != nil {
		return Defaults(), fmt.Errorf("%s: %w", path, err)
	}
	if err := s.Validate(); err != nil {
		return Defaults(), fmt.Errorf("%s: %w", path, err)
	}
	var unknown []string
	for k := range raw {
		if !known(k) {
			unknown = append(unknown, k)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return s, &UnknownKeysError{Path: path, Keys: unknown}
	}
	return s, nil
}

// Save writes the settings file through a temporary file and a rename, so
// a reader never sees half a file. Keys of the old file that this gpwebcam
// does not know stay.
func Save(path string, s Settings) error {
	if err := s.Validate(); err != nil {
		return err
	}
	merged := map[string]json.RawMessage{}
	if old, err := readFile(path); err == nil && old != nil {
		var raw map[string]json.RawMessage
		if json.Unmarshal(old, &raw) == nil {
			for k, v := range raw {
				if !known(k) {
					merged[k] = v
				}
			}
		}
	}
	ours, err := json.Marshal(s)
	if err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(ours, &fields); err != nil {
		return err
	}
	for k, v := range fields {
		merged[k] = v
	}
	data, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".settings-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // fails harmlessly after the rename
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
