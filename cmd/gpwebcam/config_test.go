package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkodemic/gpwebcam/internal/settings"
)

func TestConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CONFIGURATION_DIRECTORY", "")
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "gpwebcam", settings.FileName)

	var out bytes.Buffer
	if err := cmdConfig(nil, &out); err != nil {
		t.Fatal(err)
	}
	want := "# " + path + "\nres=1080\nfov=linear\nhwdec=auto\nnotify=on\ntray=on\n"
	if out.String() != want {
		t.Errorf("defaults:\n%s\nwant:\n%s", out.String(), want)
	}
	if _, err := os.Stat(path); err == nil {
		t.Error("showing the settings wrote the file")
	}

	if err := cmdConfig([]string{"tray", "off"}, &out); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := cmdConfig([]string{"tray"}, &out); err != nil {
		t.Fatal(err)
	}
	if out.String() != "off\n" {
		t.Errorf("tray after setting it off: %q", out.String())
	}

	for _, args := range [][]string{{"fov", "fisheye"}, {"fps"}, {"fps", "60"}, {"a", "b", "c"}} {
		if err := cmdConfig(args, &out); err == nil {
			t.Errorf("config %v accepted", args)
		}
	}

	if err := os.WriteFile(path, []byte("{broken"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := cmdConfig([]string{"tray", "on"}, &out)
	if err == nil || !strings.Contains(err.Error(), "fix or delete") {
		t.Errorf("broken file: err = %v", err)
	}
	if data, _ := os.ReadFile(path); string(data) != "{broken" {
		t.Error("a broken file was overwritten")
	}
}
