package main

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"time"

	"github.com/darkodemic/gpwebcam/internal/camera"
	"github.com/darkodemic/gpwebcam/internal/placeholder"
	"github.com/darkodemic/gpwebcam/internal/settings"
	"github.com/darkodemic/gpwebcam/internal/usbnet"
	"github.com/darkodemic/gpwebcam/internal/v4l2"
)

// demandGrace is how long the camera keeps streaming after the last
// application stopped using the device. Applications sometimes stop and
// start again at once, and each camera start costs about 5 s of placeholder.
const demandGrace = 15 * time.Second

// errIdle ends a session when no application has used the device for
// demandGrace, and errOff when the camera mode turns off.
var (
	errIdle = errors.New("no application uses the camera")
	errOff  = errors.New("the camera mode is off")
)

// watchUsage follows v4l2loopback's reports of applications streaming from
// device. Without them, camera mode demand works like always.
func (s *server) watchUsage(ctx context.Context, device string) {
	u, err := v4l2.WatchUsage(device)
	if err != nil {
		s.log.Warn("cannot tell when applications use the camera, so it streams whenever it is connected", "err", err)
		return
	}
	s.usageOK.Store(true)
	go func() {
		defer u.Close()
		err := u.Run(ctx, func(used bool) {
			if s.used.Swap(used) == used {
				return
			}
			// Whoever kept the device's size may be gone now.
			s.mu.Lock()
			s.resizeTried = ""
			s.mu.Unlock()
			if used {
				s.log.Info("an application started using the camera")
			} else {
				s.log.Info("no application uses the camera")
			}
			s.kick()
		})
		if ctx.Err() == nil {
			s.log.Warn("lost track of applications using the camera, so it streams whenever it is connected", "err", err)
			s.usageOK.Store(false)
			s.kick()
		}
	}()
}

// kick wakes idle to look at wantCamera again.
func (s *server) kick() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// wantCamera reports whether the camera should stream now.
func (s *server) wantCamera() bool {
	switch s.live.Get().Camera {
	case settings.CameraOff:
		return false
	case settings.CameraAlways:
		return true
	}
	return !s.usageOK.Load() || s.used.Load()
}

// idle waits, with the camera's webcam mode off, until the camera should
// stream, the interface disappears or ctx ends. It keeps the camera awake
// meanwhile: the camera powers itself off after its auto power down time
// without keep-alive (5 minutes on Darko's HERO13, setting 59), and over USB
// it cannot be woken again.
func (s *server) idle(parent context.Context, iface usbnet.Interface) error {
	ctx, cancel := context.WithCancelCause(parent)
	defer cancel(nil)
	go watchIface(ctx, iface, cancel)
	go s.idleKeepAlive(ctx, iface)

	t := time.NewTicker(pollInterval)
	defer t.Stop()
	shown := ""
	for !s.wantCamera() {
		if err := s.feed.Err(); err != nil {
			return fmt.Errorf("write placeholder: %w", err)
		}
		// The mode can change between off and demand while idle.
		if mode := s.live.Get().Camera; mode != shown {
			shown = mode
			if mode == settings.CameraOff {
				s.showCalm(ctx, placeholder.Paused, "Camera off")
			} else {
				model := modelName(iface)
				s.showCalm(ctx, placeholder.Ready(model), model+": ready, starts when an application uses it")
			}
		}
		s.maybeResize(ctx)
		select {
		case <-ctx.Done():
			return ended(ctx, nil)
		case <-s.wake:
		case <-t.C:
		}
	}
	return nil
}

// idleKeepAlive sends keep-alive to an idle camera until ctx ends.
func (s *server) idleKeepAlive(ctx context.Context, iface usbnet.Interface) {
	host, err := usbnet.WaitIPv4(ctx, iface.Name, 250*time.Millisecond)
	if err != nil {
		return // ctx ended; a session reports a missing address
	}
	camAddr, err := camera.AddressFor(host)
	if err != nil {
		return
	}
	cam := camera.NewClient(netip.AddrPortFrom(camAddr, camera.HTTPPort), host.Addr(), s.f.httpTimeout)
	keepAlive(ctx, cam, s.f.httpTimeout, s.log)
}

// watchDemand ends a session with errIdle once no application has used the
// device for demandGrace.
func (s *server) watchDemand(ctx context.Context, cancel context.CancelCauseFunc) {
	t := time.NewTicker(pollInterval)
	defer t.Stop()
	var since time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		switch {
		case s.live.Get().Camera == settings.CameraAlways && s.resizeWanted():
			// Always on never ends a session by itself; end it so that
			// the device can take the new size.
			s.log.Info("no application uses the camera; stopping it to change the resolution")
			cancel(errReconfigured)
			return
		case s.wantCamera():
			since = time.Time{}
		case since.IsZero():
			since = time.Now()
			if s.live.Get().Camera == settings.CameraDemand {
				s.log.Info("stopping the camera unless an application uses it again", "in", demandGrace)
			}
		case time.Since(since) >= demandGrace:
			cancel(errIdle)
			return
		}
	}
}

// endSession ends the running session, if any, with cause.
func (s *server) endSession(cause error) {
	s.mu.Lock()
	cancel := s.cancelSession
	s.mu.Unlock()
	if cancel != nil {
		cancel(cause)
	}
}
