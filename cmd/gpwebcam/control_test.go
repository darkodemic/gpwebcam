package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeController records like the server, without a camera: a file opens
// as soon as recording is on.
type fakeController struct {
	mu        sync.Mutex
	recording bool
	refuse    error
}

func (f *fakeController) status() controlStatus {
	f.mu.Lock()
	defer f.mu.Unlock()
	st := controlStatus{Camera: "GoPro HERO13 Black: 720p, linear", State: "live", Recording: f.recording}
	if f.recording {
		st.File, st.Since = "/home/u/Videos/gpwebcam/GoPro-2026-10-07-101500.mkv", time.Now().Add(-75*time.Second)
	}
	return st
}

func (f *fakeController) setRecording(on bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if on && f.refuse != nil {
		return f.refuse
	}
	f.recording = on
	return nil
}

func (f *fakeController) stopRecording() (savedRecording, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.recording {
		return savedRecording{}, errors.New("not recording")
	}
	f.recording = false
	return savedRecording{File: "/home/u/Videos/gpwebcam/GoPro-2026-10-07-101500.mkv", Seconds: 75, Bytes: 56 << 20}, nil
}

func TestControlAndRecordCommand(t *testing.T) {
	// A short path: Unix socket paths are limited to about 100 bytes.
	dir, err := os.MkdirTemp("", "gpw")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	t.Setenv("RUNTIME_DIRECTORY", "")
	t.Setenv("XDG_RUNTIME_DIR", dir)

	run := func(args ...string) (string, error) {
		var out bytes.Buffer
		err := cmdRecord(args, &out)
		return out.String(), err
	}
	if _, err := run(); err == nil || !strings.Contains(err.Error(), "not running") {
		t.Errorf("without a service: %v", err)
	}

	fake := &fakeController{}
	ctx, cancel := context.WithCancel(context.Background())
	served := make(chan struct{})
	go func() {
		serveControl(ctx, fake, slog.New(slog.NewTextHandler(io.Discard, nil)))
		close(served)
	}()
	path := filepath.Join(dir, "gpwebcam", controlSocket)
	for i := 0; i < 100; i++ {
		if _, err := os.Stat(path); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("socket %v, mode %v", err, info)
	}

	if out, err := run(); err != nil || out != "Not recording.\n" {
		t.Errorf("status: %q, %v", out, err)
	}
	if out, err := run("start"); err != nil || !strings.HasPrefix(out, "Recording to /home/u/Videos/gpwebcam/") {
		t.Errorf("start: %q, %v", out, err)
	}
	if out, err := run(); err != nil || !strings.Contains(out, "for 1:15") {
		t.Errorf("status while recording: %q, %v", out, err)
	}
	if out, err := run("stop"); err != nil || out != "Saved /home/u/Videos/gpwebcam/GoPro-2026-10-07-101500.mkv (1:15, 58.7 MB)\n" {
		t.Errorf("stop: %q, %v", out, err)
	}
	if _, err := run("stop"); err == nil || !strings.Contains(err.Error(), "not recording") {
		t.Errorf("second stop: %v", err)
	}
	fake.refuse = errors.New("no camera is connected")
	if _, err := run("start"); err == nil || err.Error() != "no camera is connected" {
		t.Errorf("refused start: %v", err)
	}
	if _, err := run("pause"); err == nil {
		t.Error("unknown command accepted")
	}

	cancel()
	<-served
	if _, err := os.Stat(path); err == nil {
		t.Error("socket left after the service stopped")
	}
}

func TestControlSocketPath(t *testing.T) {
	t.Setenv("RUNTIME_DIRECTORY", "/run/user/1000/gpwebcam")
	t.Setenv("XDG_RUNTIME_DIR", "/run/user/1000")
	if p, _ := controlSocketPath(); p != "/run/user/1000/gpwebcam/control.sock" {
		t.Errorf("under systemd: %s", p)
	}
	t.Setenv("RUNTIME_DIRECTORY", "")
	if p, _ := controlSocketPath(); p != "/run/user/1000/gpwebcam/control.sock" {
		t.Errorf("by hand: %s", p)
	}
	t.Setenv("XDG_RUNTIME_DIR", "")
	if _, err := controlSocketPath(); err == nil {
		t.Error("no error without XDG_RUNTIME_DIR")
	}
}
