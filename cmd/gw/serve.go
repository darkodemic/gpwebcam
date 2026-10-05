package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/darkodemic/gw/internal/camera"
	"github.com/darkodemic/gw/internal/feed"
	"github.com/darkodemic/gw/internal/placeholder"
	"github.com/darkodemic/gw/internal/stream"
	"github.com/darkodemic/gw/internal/usbnet"
	"github.com/darkodemic/gw/internal/v4l2"
)

const (
	// idleInterval is how often the placeholder frame is repeated.
	idleInterval = 100 * time.Millisecond
	// pollInterval is how often gw looks for a camera, and checks that the
	// interface of a running session still exists.
	pollInterval = 500 * time.Millisecond
	// retryDelay is the pause before a new session when the last one failed
	// with the camera still connected.
	retryDelay = 5 * time.Second
)

// errUnplugged ends a session whose interface disappeared.
var errUnplugged = errors.New("camera was unplugged")

// server owns the loopback device for its whole life, so the device stays
// listed as a camera between sessions.
type server struct {
	f      startFlags
	log    *slog.Logger
	feed   *feed.Feed
	width  int
	height int
	frames map[string][]byte // rendered placeholders by status line
}

// cmdServe implements both "run" (once false: wait for cameras forever) and
// "start" (once true: one session, then exit).
func cmdServe(args []string, log *slog.Logger, once bool) error {
	name := "run"
	if once {
		name = "start"
	}
	f, err := parseStart(name, args)
	if err != nil {
		return err
	}
	device, err := v4l2.DevicePath(f.videoNr)
	if err != nil {
		return err
	}
	var iface usbnet.Interface
	if once {
		// Fail before touching the device when there is nothing to start.
		if iface, err = findIface(f.iface); err != nil {
			return err
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	// After the first signal, a second one kills gw at once, in case
	// stopping the camera hangs; ffmpeg dies with it through Pdeathsig.
	go func() {
		<-ctx.Done()
		stop()
	}()

	w, h := f.res.Size()
	out, err := v4l2.OpenOutput(device, w, h)
	if err != nil {
		return err
	}
	defer out.Close()
	s := &server{f: f, log: log, feed: feed.New(out, idleInterval), width: w, height: h, frames: map[string][]byte{}}
	go s.feed.Run(ctx)
	log.Info("feeding loopback device", "device", device, "size", fmt.Sprintf("%dx%d", w, h))

	if once {
		s.show(ctx, placeholder.Connecting)
		return interrupted(ctx, s.session(ctx, iface), log)
	}
	s.show(ctx, placeholder.NotConnected)
	for {
		iface, err := s.waitForCamera(ctx)
		if err != nil {
			return interrupted(ctx, err, log)
		}
		s.show(ctx, placeholder.Connecting)
		err = s.session(ctx, iface)
		if ctx.Err() != nil {
			log.Info("stopped on signal")
			return nil
		}
		log.Warn("session ended", "iface", iface.Name, "err", err)
		if _, lerr := net.InterfaceByName(iface.Name); lerr == nil {
			// Still connected: the camera refused or stalled. Retry later
			// rather than hammer it.
			s.show(ctx, placeholder.Retrying)
			if !sleep(ctx, retryDelay) {
				log.Info("stopped on signal")
				return nil
			}
		}
		s.show(ctx, placeholder.NotConnected)
	}
}

// waitForCamera polls for a GoPro interface until one appears. It returns
// an error only when ctx ends or the device can no longer be written.
func (s *server) waitForCamera(ctx context.Context) (usbnet.Interface, error) {
	var last string
	for {
		if err := s.feed.Err(); err != nil {
			return usbnet.Interface{}, fmt.Errorf("write placeholder: %w", err)
		}
		iface, err := findIface(s.f.iface)
		if err == nil {
			s.log.Info("found camera", "iface", iface.Name, "product", iface.Product)
			return iface, nil
		}
		// Log each new reason once, not on every poll.
		if msg := err.Error(); msg != last {
			last = msg
			if errors.Is(err, errNoCamera) {
				s.log.Info("waiting for a camera")
			} else {
				s.log.Warn("waiting for a camera", "err", err)
			}
		}
		if !sleep(ctx, pollInterval) {
			return usbnet.Interface{}, ctx.Err()
		}
	}
}

// show switches the device to the placeholder with the given status line.
func (s *server) show(ctx context.Context, status string) {
	frame, ok := s.frames[status]
	if !ok {
		var err error
		frame, err = placeholder.Render(ctx, s.f.ffmpeg, s.width, s.height, status)
		if err != nil {
			s.log.Warn("render placeholder, using a blank frame", "err", err)
			frame = placeholder.Blank(s.width, s.height)
		}
		s.frames[status] = frame
	}
	if err := s.feed.Idle(frame); err != nil {
		s.log.Warn("write placeholder", "err", err)
	}
}

// session streams one camera into the device until the stream ends, the
// interface disappears or ctx ends; in the last case it returns nil.
func (s *server) session(parent context.Context, iface usbnet.Interface) error {
	f, log := s.f, s.log
	ctx, cancel := context.WithCancelCause(parent)
	defer cancel(nil)
	go watchIface(ctx, iface.Name, cancel)

	waitCtx, cancelWait := context.WithTimeout(ctx, f.dhcpWait)
	host, err := usbnet.WaitIPv4(waitCtx, iface.Name, 250*time.Millisecond)
	cancelWait()
	if err != nil {
		return ended(ctx, err)
	}
	camAddr, err := camera.AddressFor(host)
	if err != nil {
		return err
	}
	log.Info("link is up", "iface", iface.Name, "host", host, "camera", camAddr)

	cam := camera.NewClient(netip.AddrPortFrom(camAddr, camera.HTTPPort), host.Addr(), f.httpTimeout)
	err = cam.StartWebcam(ctx, camera.StartOptions{
		Res:       f.res,
		FOV:       f.fov,
		Port:      f.port,
		Poll:      500 * time.Millisecond,
		Connect:   f.connectWait,
		Streaming: 10 * time.Second,
	})
	// Stop even after a failed start: the camera may be half way into
	// webcam mode.
	defer func() {
		// An unplugged camera has nothing left to stop.
		if _, err := net.InterfaceByName(iface.Name); err != nil {
			log.Info("camera is gone, nothing to stop", "iface", iface.Name)
			return
		}
		// ctx may already be cancelled; STOP gets its own short deadline.
		sctx, cancel := context.WithTimeout(context.Background(), 2*f.httpTimeout)
		defer cancel()
		if err := cam.StopWebcam(sctx); err != nil {
			log.Warn("stop webcam", "err", err)
		} else {
			log.Info("webcam stopped")
		}
	}()
	if err != nil {
		return ended(ctx, err)
	}
	log.Info("webcam started", "res", f.res, "fov", f.fov, "port", f.port)

	kctx, cancelKeepAlive := context.WithCancel(ctx)
	defer cancelKeepAlive()
	go keepAlive(kctx, cam, f.httpTimeout, log)

	err = stream.Run(ctx, stream.Config{
		FFmpeg:      f.ffmpeg,
		Listen:      netip.AddrPortFrom(host.Addr(), f.port),
		Width:       s.width,
		Height:      s.height,
		ReadTimeout: 5 * time.Second,
	}, os.Stderr, 3*time.Second, s.feed.Live)
	return ended(ctx, err)
}

// ended picks the error a session reports: nil after a signal, errUnplugged
// after the interface disappeared, otherwise err.
func ended(ctx context.Context, err error) error {
	if ctx.Err() == nil {
		return err
	}
	if cause := context.Cause(ctx); errors.Is(cause, errUnplugged) {
		return errUnplugged
	}
	return nil
}

// watchIface cancels the session with errUnplugged once the interface is
// gone, so the placeholder appears without waiting for ffmpeg's timeout.
func watchIface(ctx context.Context, name string, cancel context.CancelCauseFunc) {
	for sleep(ctx, pollInterval) {
		if _, err := net.InterfaceByName(name); err != nil {
			cancel(errUnplugged)
			return
		}
	}
}

// sleep waits for d and reports false if ctx ended first.
func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// interrupted turns an error caused by a signal into a clean exit.
func interrupted(ctx context.Context, err error, log *slog.Logger) error {
	if ctx.Err() != nil {
		log.Info("stopped on signal")
		return nil
	}
	return err
}

// keepAlive pings the camera every KeepAliveInterval, as the spec asks, and
// logs only when the result changes.
func keepAlive(ctx context.Context, cam *camera.Client, timeout time.Duration, log *slog.Logger) {
	t := time.NewTicker(camera.KeepAliveInterval)
	defer t.Stop()
	failing := false
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		rctx, cancel := context.WithTimeout(ctx, timeout)
		err := cam.KeepAlive(rctx)
		cancel()
		switch {
		case err != nil && ctx.Err() == nil && !failing:
			log.Warn("keep-alive failed", "err", err)
			failing = true
		case err == nil && failing:
			log.Info("keep-alive works again")
			failing = false
		}
	}
}
