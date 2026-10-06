// Package settings holds the choices a user can change while gpwebcam runs,
// from the tray menu or with "gpwebcam config". They are saved as JSON in
// the user's configuration directory.
package settings

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/darkodemic/gpwebcam/internal/camera"
)

// Settings are the user's choices. Flags of the same name override them.
type Settings struct {
	Res    camera.Resolution `json:"res"`
	FOV    camera.FOV        `json:"fov"`
	HWDec  string            `json:"hwdec"`
	Notify bool              `json:"notify"`
	Tray   bool              `json:"tray"`
}

// Keys are the setting names, in the order "gpwebcam config" prints them.
// Each is also the name of the flag that overrides it.
var Keys = []string{"res", "fov", "hwdec", "notify", "tray"}

// FileName is the name of the settings file in the configuration directory.
const FileName = "settings.json"

// maxFile bounds what Load reads; the real file is about 100 bytes.
const maxFile = 64 << 10

// Defaults are the settings without a file.
func Defaults() Settings {
	return Settings{Res: camera.Res1080, FOV: camera.FOVLinear, HWDec: "auto", Notify: true, Tray: true}
}

// Validate reports the first value that is not allowed. JSON decoding does
// not go through camera's flag.Value checks, so Load calls this.
func (s Settings) Validate() error {
	for _, k := range []string{"res", "fov", "hwdec"} {
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

// Load reads the settings file. A missing file gives the defaults, and so
// does a missing key, so a file can hold only what the user changed.
func Load(path string) (Settings, error) {
	s := Defaults()
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxFile+1))
	if err != nil {
		return Defaults(), err
	}
	if len(data) > maxFile {
		return Defaults(), fmt.Errorf("%s: larger than %d bytes", path, maxFile)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&s); err != nil {
		return Defaults(), fmt.Errorf("%s: %w", path, err)
	}
	if err := s.Validate(); err != nil {
		return Defaults(), fmt.Errorf("%s: %w", path, err)
	}
	return s, nil
}

// Save writes the settings file through a temporary file and a rename, so
// a reader never sees half a file.
func Save(path string, s Settings) error {
	if err := s.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
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
