package camera

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeCamera implements the webcam state machine of the Open GoPro spec.
type fakeCamera struct {
	mu        sync.Mutex
	status    WebcamStatus
	wiredUSB  bool
	requests  []string
	failFirst int  // answer this many requests with HTTP 503 first
	startErr  int  // error code in the start reply
	lazy      bool // start does not change the status
}

func (f *fakeCamera) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, r.URL.RequestURI())
	if f.failFirst > 0 {
		f.failFirst--
		http.Error(w, "busy", http.StatusServiceUnavailable)
		return
	}
	switch r.URL.Path {
	case pathWiredUSB:
		f.wiredUSB = r.URL.Query().Get("p") == "1"
		fmt.Fprint(w, `{}`)
	case pathStatus:
		fmt.Fprintf(w, `{"status":%d,"error":0}`, f.status)
	case pathStart:
		if f.wiredUSB {
			fmt.Fprint(w, `{"status":4,"error":7}`)
			return
		}
		if f.startErr != 0 {
			fmt.Fprintf(w, `{"status":%d,"error":%d}`, f.status, f.startErr)
			return
		}
		if !f.lazy {
			f.status = StatusHighPowerPreview
		}
		fmt.Fprint(w, `{}`)
	case pathStop, pathExit:
		f.status = StatusIdle
		fmt.Fprint(w, `{}`)
	case pathKeepAlive:
		fmt.Fprint(w, `{}`)
	default:
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{}`)
	}
}

func (f *fakeCamera) log() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.requests...)
}

func newTestClient(t *testing.T, cam *fakeCamera) *Client {
	t.Helper()
	srv := httptest.NewServer(cam)
	t.Cleanup(srv.Close)
	addr := netip.MustParseAddrPort(srv.Listener.Addr().(*net.TCPAddr).String())
	return NewClient(addr, netip.MustParseAddr("127.0.0.1"), time.Second)
}

func testOptions() StartOptions {
	return StartOptions{
		Res: Res1080, FOV: FOVLinear, Port: 8554,
		Poll: 10 * time.Millisecond, Connect: time.Second, Streaming: 200 * time.Millisecond,
	}
}

func TestStartWebcam(t *testing.T) {
	cam := &fakeCamera{status: StatusOff, wiredUSB: true, failFirst: 2}
	c := newTestClient(t, cam)
	if err := c.StartWebcam(context.Background(), testOptions()); err != nil {
		t.Fatal(err)
	}
	got := strings.Join(cam.log(), " ")
	want := "/gopro/camera/control/wired_usb?p=0 " + // 503
		"/gopro/camera/control/wired_usb?p=0 " + // 503
		"/gopro/camera/control/wired_usb?p=0 " +
		"/gopro/webcam/status " +
		"/gopro/webcam/start?res=12&fov=4&port=8554&protocol=TS " +
		"/gopro/webcam/status"
	if got != want {
		t.Errorf("requests:\n got %s\nwant %s", got, want)
	}
}

func TestStartWebcamStopsLeftover(t *testing.T) {
	cam := &fakeCamera{status: StatusHighPowerPreview}
	c := newTestClient(t, cam)
	o := testOptions()
	o.Res, o.FOV, o.Port = Res720, FOVWide, 9000
	if err := c.StartWebcam(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	log := cam.log()
	if len(log) < 4 || log[2] != pathStop || log[3] != "/gopro/webcam/start?res=7&fov=0&port=9000&protocol=TS" {
		t.Errorf("requests = %q, want stop before start", log)
	}
}

func TestStartWebcamErrors(t *testing.T) {
	var re *ReplyError
	c := newTestClient(t, &fakeCamera{startErr: 1})
	if err := c.StartWebcam(context.Background(), testOptions()); !errors.As(err, &re) || re.Code != 1 {
		t.Errorf("start error reply: %v", err)
	}

	c = newTestClient(t, &fakeCamera{lazy: true})
	if err := c.StartWebcam(context.Background(), testOptions()); err == nil || !strings.Contains(err.Error(), "did not start streaming") {
		t.Errorf("camera that never streams: %v", err)
	}

	c = newTestClient(t, &fakeCamera{failFirst: 1 << 30})
	o := testOptions()
	o.Connect = 100 * time.Millisecond
	if err := c.StartWebcam(context.Background(), o); !IsStatus(err, http.StatusServiceUnavailable) {
		t.Errorf("camera that never answers: %v", err)
	}
}

func TestStopWebcam(t *testing.T) {
	cam := &fakeCamera{status: StatusHighPowerPreview}
	c := newTestClient(t, cam)
	if err := c.StopWebcam(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(cam.log(), " "); got != pathStop+" "+pathExit {
		t.Errorf("requests = %s", got)
	}
}

func TestNoProxy(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	t.Setenv("http_proxy", "http://127.0.0.1:1")
	cam := &fakeCamera{}
	c := newTestClient(t, cam)
	if err := c.KeepAlive(context.Background()); err != nil {
		t.Fatalf("keep-alive went through the proxy: %v", err)
	}
}

func TestRedirectNotFollowed(t *testing.T) {
	srv := httptest.NewServer(http.RedirectHandler("http://example.com/", http.StatusFound))
	t.Cleanup(srv.Close)
	addr := netip.MustParseAddrPort(srv.Listener.Addr().(*net.TCPAddr).String())
	c := NewClient(addr, netip.MustParseAddr("127.0.0.1"), time.Second)
	if err := c.KeepAlive(context.Background()); !IsStatus(err, http.StatusFound) {
		t.Errorf("redirect: %v, want HTTP 302 error", err)
	}
}
