package main

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/darkodemic/gpwebcam/internal/camera"
	"github.com/darkodemic/gpwebcam/internal/settings"
)

func quietLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestLiveFlagsOverrideFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), settings.FileName)
	file := settings.Defaults()
	file.FOV, file.Notify = camera.FOVWide, false
	if err := settings.Save(path, file); err != nil {
		t.Fatal(err)
	}
	f, err := parseStart("run", []string{"-fov", "narrow", "-ffmpeg", "/bin/true"})
	if err != nil {
		t.Fatal(err)
	}
	l := newLive(path, f, quietLog())
	got := l.Get()
	if got.FOV != camera.FOVNarrow {
		t.Errorf("fov %s, want the flag's narrow", got.FOV)
	}
	if got.Notify {
		t.Error("notify on, want the file's off: -notify was not given")
	}
	if !l.Locked("fov") || l.Locked("notify") {
		t.Errorf("locked fov=%t notify=%t, want true false", l.Locked("fov"), l.Locked("notify"))
	}
	if err := l.Set("fov", "wide"); err == nil {
		t.Error("changed a setting a flag fixes")
	}
}

func TestLiveSetSavesAndReports(t *testing.T) {
	path := filepath.Join(t.TempDir(), settings.FileName)
	f, err := parseStart("run", []string{"-ffmpeg", "/bin/true"})
	if err != nil {
		t.Fatal(err)
	}
	l := newLive(path, f, quietLog())
	var changes [][2]settings.Settings
	l.changed = func(old, cur settings.Settings) { changes = append(changes, [2]settings.Settings{old, cur}) }

	if err := l.Set("fov", "superview"); err != nil {
		t.Fatal(err)
	}
	if err := l.Set("fov", "superview"); err != nil { // no change
		t.Fatal(err)
	}
	if err := l.Set("fov", "fisheye"); err == nil {
		t.Error("accepted an unknown FOV")
	}
	if len(changes) != 1 || changes[0][0].FOV != camera.FOVLinear || changes[0][1].FOV != camera.FOVSuperView {
		t.Errorf("changes %+v, want one from linear to superview", changes)
	}
	saved, err := settings.Load(path)
	if err != nil || saved.FOV != camera.FOVSuperView {
		t.Errorf("file has %+v (%v), want fov superview", saved, err)
	}
	// Our own write is not reported again by the watcher.
	l.reload()
	if len(changes) != 1 {
		t.Errorf("reload after our own write reported %d changes", len(changes)-1)
	}
}

func TestLiveReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), settings.FileName)
	f, err := parseStart("run", []string{"-ffmpeg", "/bin/true"})
	if err != nil {
		t.Fatal(err)
	}
	l := newLive(path, f, quietLog())
	var last settings.Settings
	n := 0
	l.changed = func(_, cur settings.Settings) { last, n = cur, n+1 }

	// What "gpwebcam config" does.
	next := settings.Defaults()
	next.Tray = false
	if err := settings.Save(path, next); err != nil {
		t.Fatal(err)
	}
	l.reload()
	if n != 1 || last.Tray {
		t.Fatalf("after the file turned the tray off: %d changes, tray %t", n, last.Tray)
	}

	// A broken file keeps the settings in use.
	if err := os.WriteFile(path, []byte(`{"fov": "fisheye"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	l.reload()
	if n != 1 || l.Get().Tray {
		t.Errorf("a broken file changed the settings: %d changes, %+v", n, l.Get())
	}

	// Deleting the file goes back to the defaults.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	l.reload()
	if n != 2 || !last.Tray {
		t.Errorf("after deleting the file: %d changes, tray %t", n, last.Tray)
	}
}

func TestLiveBrokenFileAtStart(t *testing.T) {
	path := filepath.Join(t.TempDir(), settings.FileName)
	if err := os.WriteFile(path, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := parseStart("run", []string{"-ffmpeg", "/bin/true"})
	if err != nil {
		t.Fatal(err)
	}
	if got := newLive(path, f, quietLog()).Get(); got != settings.Defaults() {
		t.Errorf("broken file gave %+v, want defaults", got)
	}
}
