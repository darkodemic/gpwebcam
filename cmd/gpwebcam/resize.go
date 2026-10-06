package main

import (
	"context"
	"fmt"

	"github.com/darkodemic/gpwebcam/internal/camera"
	"github.com/darkodemic/gpwebcam/internal/feed"
	"github.com/darkodemic/gpwebcam/internal/placeholder"
	"github.com/darkodemic/gpwebcam/internal/v4l2"
)

// size returns the device's resolution and frame size.
func (s *server) size() (camera.Resolution, int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.res, s.width, s.height
}

// resizeWanted reports whether the settings ask for another resolution and
// the device could take it now: applications keep the frame size they
// started with, so it changes only while none streams. A size the device
// kept at the last try waits until applications come or go.
func (s *server) resizeWanted() bool {
	want := s.live.Get().Res
	s.mu.Lock()
	cur, tried := s.res, s.resizeTried
	s.mu.Unlock()
	return want != cur && want != tried && s.usageOK.Load() && !s.used.Load()
}

// maybeResize reopens the device at the resolution of the settings when
// resizeWanted says so. It must not run during a session.
func (s *server) maybeResize(ctx context.Context) {
	if !s.resizeWanted() {
		return
	}
	want := s.live.Get().Res
	w, h := want.Size()
	_, cw, ch := s.size()
	// Render before the swap: the feed writes nothing while it swaps.
	render, err := placeholder.NewRenderer(ctx, s.f.ffmpeg, w, h)
	if err != nil {
		s.log.Warn("render placeholder, using a plain frame", "err", err)
	}
	s.mu.Lock()
	status, animate := s.shownStatus, s.shownAnimate
	s.mu.Unlock()
	var pic feed.Source = feed.Still(placeholder.Blank(w, h))
	if p, err := render.Picture(ctx, status, animate); err == nil && p != nil {
		pic = p
	}

	var out *v4l2.Output
	err = s.feed.Swap(func(feed.Sink) (feed.Sink, feed.Source, error) {
		// The old output must close first: v4l2loopback has one output
		// format token, which the open output holds.
		s.out.Close()
		o, err := v4l2.OpenOutput(s.device, w, h)
		if err != nil {
			s.log.Warn("reopen the device at the new size", "size", fmt.Sprintf("%dx%d", w, h), "err", err)
			if o, err = v4l2.OpenOutput(s.device, cw, ch); err != nil {
				return nil, nil, err
			}
		}
		out = o
		gw, gh := o.Size()
		switch {
		case gw == w && gh == h:
			return o, pic, nil
		case gw == cw && gh == ch:
			// An application opened the device and set its format without
			// streaming; it keeps that size.
			s.mu.Lock()
			old := s.pictures[pictureKey(status, animate)]
			s.mu.Unlock()
			if old == nil {
				return o, feed.Still(placeholder.Blank(gw, gh)), nil
			}
			return o, old, nil
		default:
			return o, feed.Still(placeholder.Blank(gw, gh)), nil
		}
	})
	if err != nil {
		// Neither size opened: the device is gone or taken. Writing to the
		// closed output fails, and the main loop ends on that error.
		s.log.Error("lost the loopback device while changing its size", "err", err)
		return
	}

	gw, gh := out.Size()
	s.mu.Lock()
	s.out = out
	switch {
	case gw == w && gh == h:
		s.res, s.width, s.height, s.render = want, w, h, render
		s.pictures = map[string]*placeholder.Picture{}
	case gw == cw && gh == ch:
		s.resizeTried = want
	default:
		s.res, s.width, s.height = camera.ResolutionFor(gw, gh), gw, gh
		s.render, _ = placeholder.NewRenderer(ctx, s.f.ffmpeg, gw, gh)
		s.pictures = map[string]*placeholder.Picture{}
	}
	s.mu.Unlock()

	if gw == w && gh == h {
		s.log.Info("device size changed", "size", fmt.Sprintf("%dx%d", w, h))
	} else {
		s.log.Warn("an application keeps the device at its size; the new resolution applies when it closes the camera",
			"size", fmt.Sprintf("%dx%d", gw, gh), "wanted", fmt.Sprintf("%dx%d", w, h))
	}
	s.updateTray()
}
