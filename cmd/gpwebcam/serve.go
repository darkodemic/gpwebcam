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
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/darkodemic/gpwebcam/internal/camera"
	"github.com/darkodemic/gpwebcam/internal/feed"
	"github.com/darkodemic/gpwebcam/internal/notify"
	"github.com/darkodemic/gpwebcam/internal/placeholder"
	"github.com/darkodemic/gpwebcam/internal/stream"
	"github.com/darkodemic/gpwebcam/internal/usbnet"
	"github.com/darkodemic/gpwebcam/internal/v4l2"
)

const (
	// idleInterval is how often the placeholder frame is repeated.
	idleInterval = 100 * time.Millisecond
	// pollInterval is how often gpwebcam looks for a camera, and checks that the
	// interface of a running session still exists.
	pollInterval = 500 * time.Millisecond
	// devicePoll is how often gpwebcam looks for its loopback device while
	// the module is not loaded yet.
	devicePoll = 2 * time.Second
	// retryDelay is the pause before a new session when the last one failed
	// with the camera still connected.
	retryDelay = 2 * time.Second
	// firstFrame is how long a session waits for video after the camera
	// reports streaming. Frames normally arrive about 1.5 s after start;
	// after a replug the camera sometimes reports streaming and sends
	// nothing (seen 2026-10-05, also upstream PR #76), and a new start fixes it.
	firstFrame = 6 * time.Second
)

// Causes that end a session when its interface disappears.
var (
	errUnplugged = errors.New("camera was unplugged")
	// errRenamed: the interface vanished but its USB device is still there.
	// udev renames the kernel's "eth0" to a predictable name about 0.5 s
	// after the camera appears, and a session may have started in between.
	errRenamed = errors.New("network interface was renamed")
)

// noVideoHint is how many sessions in a row must end without video before
// the placeholder suggests a firewall.
const noVideoHint = 3

// server owns the loopback device for its whole life, so the device stays
// listed as a camera between sessions.
type server struct {
	f      startFlags
	log    *slog.Logger
	notify *notify.Notifier // nil when disabled or notify-send is missing
	feed   *feed.Feed
	width  int
	height int

	render *placeholder.Renderer

	mu       sync.Mutex
	pictures map[string]*placeholder.Picture // by status line
	reported map[string]bool                 // untested models already reported

	// gpu is "vaapi" when -hwdec auto found a working VAAPI device at
	// start; it becomes "none" after GPU decoding gave no video twice.
	gpu string
}

// hwdec returns the stream.Config HWAccel value for the next session.
func (s *server) hwdec() string { return s.gpu }

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
	var iface usbnet.Interface
	if once {
		// Fail before touching the device when there is nothing to start.
		if iface, err = findIface(f.iface); err != nil {
			return err
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	// After the first signal, a second one kills gpwebcam at once, in case
	// stopping the camera hangs; ffmpeg dies with it through Pdeathsig.
	go func() {
		<-ctx.Done()
		stop()
	}()

	var notifier *notify.Notifier
	if f.notify {
		notifier = notify.New()
	}
	device, err := findDevice(f)
	if err != nil && !once {
		notifier.Send(notify.Normal, "gpwebcam is waiting for its video device",
			"Load the v4l2loopback module or reboot. See: journalctl --user -u gpwebcam")
		// At boot the user service may start before the module is loaded,
		// or the module may be installed later: wait instead of exiting
		// into a restart loop.
		log.Warn("waiting for the loopback device", "err", err)
		device, err = waitForDevice(ctx, f)
	}
	if err != nil {
		return interrupted(ctx, err, log)
	}

	w, h := f.res.Size()
	out, err := v4l2.OpenOutput(device, w, h)
	if err != nil {
		return err
	}
	defer out.Close()
	gpu := "none"
	if f.hwdec == "auto" {
		if err := stream.ProbeVAAPI(ctx, f.ffmpeg); err == nil {
			gpu = "vaapi"
		} else {
			log.Info("hardware decoding is not available, decoding in software", "err", err)
		}
	}
	render, err := placeholder.NewRenderer(ctx, f.ffmpeg, w, h)
	if err != nil {
		log.Warn("render placeholder, using a plain frame", "err", err)
	}
	s := &server{
		f: f, log: log, notify: notifier, feed: feed.New(out, idleInterval), width: w, height: h,
		render: render, pictures: map[string]*placeholder.Picture{}, reported: map[string]bool{},
		gpu: gpu,
	}
	go s.feed.Run(ctx)
	log.Info("feeding loopback device", "device", device, "size", fmt.Sprintf("%dx%d", w, h))
	for _, st := range []string{placeholder.NotConnected, placeholder.NoVideo, placeholder.NotAnswering, placeholder.Problem} {
		s.picture(ctx, st, false)
	}

	if once {
		s.showMoving(ctx, placeholder.WaitingNetwork(modelName(iface)))
		return interrupted(ctx, s.session(ctx, iface), log)
	}
	s.show(ctx, placeholder.NotConnected)
	noVideo := 0
	for {
		iface, err := s.waitForCamera(ctx)
		if err != nil {
			return interrupted(ctx, err, log)
		}
		s.reportModel(iface)
		s.showMoving(ctx, placeholder.WaitingNetwork(modelName(iface)))
		err = s.session(ctx, iface)
		switch {
		case ctx.Err() != nil:
			log.Info("stopped on signal")
			return nil
		case errors.Is(err, errRenamed):
			log.Info("network interface renamed, starting again", "old", iface.Name)
			continue
		case errors.Is(err, errUnplugged):
			log.Info("camera unplugged", "iface", iface.Name)
			noVideo = 0
			s.show(ctx, placeholder.NotConnected)
			s.notify.Send(notify.Low, modelName(iface)+" disconnected", "The webcam shows a placeholder until the camera is back.")
			continue
		}
		log.Warn("session ended", "iface", iface.Name, "err", err)
		if errors.Is(err, stream.ErrNoVideo) {
			noVideo++
		} else {
			noVideo = 0
		}
		if noVideo >= 2 && s.gpu != "none" {
			// A GPU decoder that initialises but yields nothing would
			// otherwise fail every session; software decoding always works.
			log.Warn("no video twice with hardware decoding, using software decoding from now on")
			s.gpu = "none"
		}
		if _, lerr := net.InterfaceByName(iface.Name); lerr != nil {
			s.show(ctx, placeholder.NotConnected)
			continue
		}
		// Still connected: the camera refused or stalled. Say why on the
		// picture and retry later rather than hammer it.
		model := modelName(iface)
		status := retryStatus(err, noVideo, model)
		if status == placeholder.Retrying(model) {
			s.showMoving(ctx, status)
		} else {
			s.show(ctx, status)
		}
		switch status {
		case placeholder.NoVideo:
			s.notify.Send(notify.Normal, model+" sends no video",
				fmt.Sprintf("Is a firewall blocking UDP port %d? See: journalctl --user -u gpwebcam", f.port))
		case placeholder.NotAnswering:
			s.notify.Send(notify.Normal, model+" does not answer", "Unplug and replug the USB cable.")
		case placeholder.Problem:
			s.notify.Send(notify.Normal, model+" problem", "gpwebcam keeps retrying. See: journalctl --user -u gpwebcam")
		}
		if !sleep(ctx, retryDelay) {
			log.Info("stopped on signal")
			return nil
		}
	}
}

// retryStatus picks the placeholder line for a session that failed while
// the camera stayed connected.
func retryStatus(err error, noVideo int, model string) string {
	switch {
	case errors.Is(err, stream.ErrNoVideo) && noVideo >= noVideoHint:
		return placeholder.NoVideo
	case errors.Is(err, stream.ErrNoVideo):
		return placeholder.Retrying(model)
	case errors.Is(err, camera.ErrNoAnswer):
		return placeholder.NotAnswering
	default:
		return placeholder.Problem
	}
}

// waitForDevice polls until findDevice succeeds or ctx ends.
func waitForDevice(ctx context.Context, f startFlags) (string, error) {
	for {
		if !sleep(ctx, devicePoll) {
			return "", ctx.Err()
		}
		if device, err := findDevice(f); err == nil {
			return device, nil
		}
	}
}

// findDevice returns /dev/videoN from -video-nr, or else the device whose
// card label is -device-label.
func findDevice(f startFlags) (string, error) {
	if f.videoNr >= 0 {
		p, err := v4l2.DevicePath(f.videoNr)
		if err != nil {
			return "", err
		}
		if _, err := os.Stat(p); err != nil {
			return "", fmt.Errorf("%w (is v4l2loopback loaded with video_nr=%d?)", err, f.videoNr)
		}
		return p, nil
	}
	return v4l2.FindByLabel("/sys", f.label)
}

// waitForCamera polls for a GoPro interface until one appears. It returns
// an error only when ctx ends or the device can no longer be written.
func (s *server) waitForCamera(ctx context.Context) (usbnet.Interface, error) {
	var last string
	shown := false
	for {
		if err := s.feed.Err(); err != nil {
			return usbnet.Interface{}, fmt.Errorf("write placeholder: %w", err)
		}
		iface, err := findIface(s.f.iface)
		if err == nil {
			s.log.Info("found camera", "iface", iface.Name, "product", iface.Product)
			return iface, nil
		}
		if !shown {
			s.show(ctx, placeholder.NotConnected)
			shown = true
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

// picture returns the placeholder for a status line, rendering it once.
func (s *server) picture(ctx context.Context, status string, animate bool) *placeholder.Picture {
	key := fmt.Sprintf("%t %s", animate, status)
	s.mu.Lock()
	defer s.mu.Unlock()
	if p, ok := s.pictures[key]; ok {
		return p
	}
	p, err := s.render.Picture(ctx, status, animate)
	if err != nil {
		s.log.Warn("render placeholder", "status", status, "err", err)
	}
	s.pictures[key] = p
	return p
}

// show switches the device to a still placeholder with the given status.
func (s *server) show(ctx context.Context, status string) { s.showPicture(ctx, status, false) }

// showMoving shows a status that waits for something, with moving dots.
func (s *server) showMoving(ctx context.Context, status string) { s.showPicture(ctx, status, true) }

func (s *server) showPicture(ctx context.Context, status string, animate bool) {
	if err := s.feed.Idle(s.picture(ctx, status, animate)); err != nil {
		s.log.Warn("write placeholder", "err", err)
	}
}

// testedModels are the cameras gpwebcam has been tested with, by USB
// product string.
var testedModels = map[string]bool{"HERO13 Black": true}

// modelName is the camera's name for people: "GoPro " and the USB product
// string, cleaned because it comes from the device.
func modelName(iface usbnet.Interface) string {
	p := placeholder.Clean(iface.Product)
	switch {
	case p == "":
		return "GoPro"
	case strings.HasPrefix(strings.ToLower(p), "gopro"):
		return p
	default:
		return "GoPro " + p
	}
}

// reportModel logs and notifies, once per model, that a camera has not
// been tested, so its users know to report how it works.
func (s *server) reportModel(iface usbnet.Interface) {
	if testedModels[iface.Product] {
		return
	}
	s.mu.Lock()
	seen := s.reported[iface.Product]
	s.reported[iface.Product] = true
	s.mu.Unlock()
	if seen {
		return
	}
	s.log.Warn("this camera model has not been tested with gpwebcam; please report whether it works",
		"model", modelName(iface), "tested", "HERO13 Black")
	s.notify.Send(notify.Normal, modelName(iface)+" has not been tested",
		"gpwebcam will try it. Please report whether it works: https://github.com/darkodemic/gpwebcam/issues")
}

// session streams one camera into the device until the stream ends, the
// interface disappears or ctx ends; in the last case it returns nil.
func (s *server) session(parent context.Context, iface usbnet.Interface) error {
	f, log := s.f, s.log
	ctx, cancel := context.WithCancelCause(parent)
	defer cancel(nil)
	go watchIface(ctx, iface, cancel)
	go func() {
		// Show the placeholder as soon as the cable is out, not when
		// ffmpeg has finished exiting.
		<-ctx.Done()
		if errors.Is(context.Cause(ctx), errUnplugged) {
			s.show(parent, placeholder.NotConnected)
		}
	}()

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
	s.showMoving(ctx, placeholder.Starting(modelName(iface)))

	cam := camera.NewClient(netip.AddrPortFrom(camAddr, camera.HTTPPort), host.Addr(), f.httpTimeout)
	err = cam.StartWebcam(ctx, camera.StartOptions{
		Res:       f.res,
		FOV:       f.fov,
		Port:      f.port,
		Poll:      500 * time.Millisecond,
		Connect:   f.connectWait,
		Streaming: 10 * time.Second,
		OnStatus: func(st camera.WebcamStatus) {
			log.Info("camera webcam status before start", "status", st)
		},
	})
	// Stop even after a failed start: the camera may be half way into
	// webcam mode.
	defer func() {
		// Without the interface the camera cannot be reached anyway.
		if _, err := net.InterfaceByName(iface.Name); err != nil {
			log.Info("interface is gone, not stopping the camera", "iface", iface.Name)
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
	log.Info("webcam started", "res", f.res, "fov", f.fov, "port", f.port, "hwdec", s.hwdec())

	kctx, cancelKeepAlive := context.WithCancel(ctx)
	defer cancelKeepAlive()
	go keepAlive(kctx, cam, f.httpTimeout, log)

	var videoFlowing sync.Once
	err = stream.Run(ctx, stream.Config{
		FFmpeg:      f.ffmpeg,
		Listen:      netip.AddrPortFrom(host.Addr(), f.port),
		Width:       s.width,
		Height:      s.height,
		ReadTimeout: 5 * time.Second,
		FirstFrame:  firstFrame,
		HWAccel:     s.hwdec(),
	}, newLineLogger(log, "ffmpeg"), 3*time.Second, func(frame []byte) error {
		if ctx.Err() != nil {
			return nil // ending: leave the device to the placeholder
		}
		videoFlowing.Do(func() {
			log.Info("video is flowing")
			s.notify.Send(notify.Low, modelName(iface)+" connected",
				fmt.Sprintf("Streaming %sp with the %s field of view.", f.res, f.fov))
		})
		return s.feed.Live(frame)
	})
	return ended(ctx, err)
}

// ended picks the error a session reports: nil after a signal, errUnplugged
// or errRenamed after the interface disappeared, otherwise err.
func ended(ctx context.Context, err error) error {
	if ctx.Err() == nil {
		return err
	}
	switch cause := context.Cause(ctx); {
	case errors.Is(cause, errUnplugged):
		return errUnplugged
	case errors.Is(cause, errRenamed):
		return errRenamed
	}
	return nil
}

// watchIface ends the session once the interface is gone, so the
// placeholder appears without waiting for ffmpeg's timeout. If the USB
// device is still there, the interface was only renamed.
func watchIface(ctx context.Context, iface usbnet.Interface, cancel context.CancelCauseFunc) {
	for sleep(ctx, pollInterval) {
		if _, err := net.InterfaceByName(iface.Name); err != nil {
			// On unplug the interface can go a moment before the USB
			// device, so look at the device again a little later.
			sleep(ctx, 300*time.Millisecond)
			if _, err := os.Stat(iface.USBPath); err == nil {
				cancel(errRenamed)
			} else {
				cancel(errUnplugged)
			}
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
