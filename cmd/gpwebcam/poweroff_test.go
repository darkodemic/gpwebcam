package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/darkodemic/gpwebcam/internal/camera"
	"github.com/darkodemic/gpwebcam/internal/settings"
	"github.com/darkodemic/gpwebcam/internal/usbnet"
)

func testServer(t *testing.T) *server {
	t.Helper()
	f, err := parseStart("run", []string{"-ffmpeg", "/bin/true"})
	if err != nil {
		t.Fatal(err)
	}
	return &server{f: f, live: newLive(filepath.Join(t.TempDir(), settings.FileName), f, quietLog()), log: quietLog(), wake: make(chan struct{}, 1)}
}

func TestPowerOff(t *testing.T) {
	s := testServer(t)
	if err := s.powerOff(); err == nil {
		t.Error("power off without a camera did not fail")
	}
	if s.offWanted.Load() {
		t.Error("power off wanted without a camera")
	}

	// With a camera and a running session: the session ends so that the
	// webcam stops first, the recording does not come back, and the idle
	// wait is woken in case there is no session.
	s.present.Store(true)
	ctx, cancel := context.WithCancelCause(context.Background())
	s.cancelSession = cancel
	s.recWant = true
	if err := s.powerOff(); err != nil {
		t.Fatal(err)
	}
	if cause := context.Cause(ctx); !errors.Is(cause, errPowerOff) {
		t.Errorf("session ended with %v, want errPowerOff", cause)
	}
	if err := ended(ctx, nil); !errors.Is(err, errPowerOff) {
		t.Errorf("session reports %v, want errPowerOff", err)
	}
	if s.recording() {
		t.Error("recording still wanted")
	}
	if !s.offWanted.Load() {
		t.Error("power off not wanted")
	}
	select {
	case <-s.wake:
	default:
		t.Error("the idle wait was not woken")
	}
	// A second click while the first is on its way changes nothing, and
	// neither item is offered meanwhile.
	if err := s.powerOff(); err != nil {
		t.Fatal(err)
	}
	if v := s.view(); v.CanRecord || v.CanPowerOff {
		t.Errorf("CanRecord %t CanPowerOff %t while turning off, want false false", v.CanRecord, v.CanPowerOff)
	}
	if err := s.setRecording(true); err == nil {
		t.Error("recording started while turning off")
	}

	// The camera left the bus before the power off: nothing is pending.
	s.forget()
	if s.offWanted.Load() || s.present.Load() || s.recording() {
		t.Error("forget left something behind")
	}
}

func TestPowerOffPending(t *testing.T) {
	// A power off asked just before: the idle wait and a session end at
	// once with errPowerOff, so that the main loop turns the camera off.
	s := testServer(t)
	s.present.Store(true)
	s.offWanted.Store(true)
	if err := s.live.Set("camera", settings.CameraOff); err != nil {
		t.Fatal(err)
	}
	iface := usbnet.Interface{Name: "gpwtest0", USBPath: filepath.Join(t.TempDir(), "none")}
	if err := s.idle(context.Background(), iface); !errors.Is(err, errPowerOff) {
		t.Errorf("idle returned %v, want errPowerOff", err)
	}
	if _, err := s.session(context.Background(), iface); !errors.Is(err, errPowerOff) {
		t.Errorf("session returned %v, want errPowerOff", err)
	}
	if !s.offWanted.Load() {
		t.Error("the flag is for the main loop to clear")
	}
}

// sleepyCamera answers the webcam status, stop, exit and sleep requests.
type sleepyCamera struct {
	mu        sync.Mutex
	streaming bool
	noSleep   bool // a firmware without the command
	requests  []string
	asleep    bool
}

func (c *sleepyCamera) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.requests = append(c.requests, r.URL.Path)
	switch r.URL.Path {
	case "/gopro/webcam/status":
		st := 0
		if c.streaming {
			st = 2
		}
		fmt.Fprintf(w, `{"status":%d,"error":0}`, st)
	case "/gopro/webcam/stop", "/gopro/webcam/exit":
		c.streaming = false
		fmt.Fprint(w, `{}`)
	case "/gp/gpControl/command/system/sleep":
		if c.noSleep {
			w.WriteHeader(http.StatusNotFound)
		} else {
			c.asleep = true
		}
		fmt.Fprint(w, `{}`)
	default:
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{}`)
	}
}

func (c *sleepyCamera) log() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return strings.Join(c.requests, " ")
}

func TestSleepCamera(t *testing.T) {
	client := func(c *sleepyCamera) *camera.Client {
		srv := httptest.NewServer(c)
		t.Cleanup(srv.Close)
		addr := netip.MustParseAddrPort(srv.Listener.Addr().(*net.TCPAddr).String())
		return camera.NewClient(addr, netip.MustParseAddr("127.0.0.1"), time.Second)
	}
	// gone reports the camera off the bus from the n-th look on.
	gone := func(n int) func() bool {
		looks := 0
		return func() bool { looks++; return looks >= n }
	}

	// A camera that still streams is stopped first; it leaves the bus on
	// the second look after sleep.
	cam := &sleepyCamera{streaming: true}
	if err := sleepCamera(context.Background(), client(cam), time.Second, time.Second, gone(2), quietLog()); err != nil {
		t.Fatal(err)
	}
	if want := "/gopro/webcam/status /gopro/webcam/stop /gopro/webcam/exit /gp/gpControl/command/system/sleep"; cam.log() != want {
		t.Errorf("requests %q, want %q", cam.log(), want)
	}

	// An idle camera gets sleep right away.
	cam = &sleepyCamera{}
	if err := sleepCamera(context.Background(), client(cam), time.Second, time.Second, gone(1), quietLog()); err != nil {
		t.Fatal(err)
	}
	if want := "/gopro/webcam/status /gp/gpControl/command/system/sleep"; cam.log() != want {
		t.Errorf("requests %q, want %q", cam.log(), want)
	}

	// A firmware without the command: the error says so, the camera stays.
	cam = &sleepyCamera{noSleep: true}
	err := sleepCamera(context.Background(), client(cam), time.Second, time.Second, func() bool { return false }, quietLog())
	if !errors.Is(err, camera.ErrNoSleep) {
		t.Errorf("no sleep command: %v", err)
	}

	// A camera that accepts the command but stays on the bus.
	cam = &sleepyCamera{}
	err = sleepCamera(context.Background(), client(cam), time.Second, 300*time.Millisecond, func() bool { return false }, quietLog())
	if err == nil || !strings.Contains(err.Error(), "still connected") {
		t.Errorf("camera stays: %v", err)
	}

	// A camera that leaves the bus before it answers counts as off.
	cam = &sleepyCamera{noSleep: true}
	if err := sleepCamera(context.Background(), client(cam), time.Second, time.Second, func() bool { return true }, quietLog()); err != nil {
		t.Errorf("gone before answering: %v", err)
	}
}
