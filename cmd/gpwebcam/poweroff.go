package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"time"

	"github.com/darkodemic/gpwebcam/internal/camera"
	"github.com/darkodemic/gpwebcam/internal/placeholder"
	"github.com/darkodemic/gpwebcam/internal/usbnet"
)

// errPowerOff ends a session or the idle wait because the user asked to
// turn the camera off.
var errPowerOff = errors.New("the camera is being turned off")

// powerOffWait bounds the wait for a camera that accepted the sleep
// command to leave the USB bus; a HERO13 Black took 1.7 s (2026-10-27).
const powerOffWait = 10 * time.Second

// powerOff asks the main loop to turn the camera off, as its button does.
// It ends the session, if one runs, so that the webcam is stopped first,
// and the main loop then sends the command (turnOff). Only the camera's
// button turns it on again; the service then carries on by itself.
func (s *server) powerOff() error {
	if !s.present.Load() {
		return errors.New("no camera is connected")
	}
	if s.offWanted.Swap(true) {
		return nil // already on its way
	}
	s.log.Info("power off asked from the tray")
	// The session saves the recording; a camera turned on again does not
	// record by itself.
	s.recMu.Lock()
	s.recWant = false
	s.recMu.Unlock()
	s.endSession(errPowerOff)
	s.kick()
	s.updateTray()
	return nil
}

// forget resets what the service keeps about a camera that is gone: it is
// not present, a recording does not resume by itself when a camera comes
// back, and a power off that was asked is moot.
func (s *server) forget() {
	s.present.Store(false)
	s.recMu.Lock()
	s.recWant = false
	s.recMu.Unlock()
	s.offWanted.Store(false)
}

// turnOff turns the camera on iface off and waits for it to leave the USB
// bus.
func (s *server) turnOff(ctx context.Context, iface usbnet.Interface) error {
	defer s.offWanted.Store(false)
	s.log.Info("turning the camera off", "iface", iface.Name)
	s.showMoving(ctx, placeholder.PoweringOff(modelName(iface)))
	cctx, cancel := context.WithTimeout(ctx, s.f.httpTimeout)
	cam, err := s.cameraClient(cctx, iface)
	cancel()
	if err != nil {
		return err
	}
	gone := func() bool {
		_, err := net.InterfaceByName(iface.Name)
		return err != nil
	}
	return sleepCamera(ctx, cam, s.f.httpTimeout, powerOffWait, gone, s.log)
}

// sleepCamera stops the webcam if it still streams, sends the sleep command
// and waits up to wait for gone to report that the camera left the bus. A
// camera that leaves before it answers counts as off.
func sleepCamera(ctx context.Context, cam *camera.Client, timeout, wait time.Duration, gone func() bool, log *slog.Logger) error {
	// Up to four requests, each with the client's own timeout.
	rctx, cancel := context.WithTimeout(ctx, 4*timeout)
	defer cancel()
	// The session's end stops the webcam; this is for when that failed.
	// Sleep was only ever tried with the webcam stopped.
	if r, err := cam.Status(rctx); err != nil {
		log.Warn("webcam status before the power off", "err", err)
	} else if r.Status.Streaming() {
		log.Warn("the webcam still streams, stopping it before the power off")
		if err := cam.StopWebcam(rctx); err != nil {
			return err
		}
	}
	if err := cam.Sleep(rctx); err != nil && !gone() {
		return err
	}
	deadline := time.Now().Add(wait)
	for !gone() {
		if time.Now().After(deadline) {
			return fmt.Errorf("the camera accepted the command but is still connected after %v", wait)
		}
		if !sleep(ctx, 250*time.Millisecond) {
			return ctx.Err()
		}
	}
	return nil
}

// cameraClient returns a client for the camera on iface, once the
// interface has its address.
func (s *server) cameraClient(ctx context.Context, iface usbnet.Interface) (*camera.Client, error) {
	host, err := usbnet.WaitIPv4(ctx, iface.Name, 250*time.Millisecond)
	if err != nil {
		return nil, err
	}
	camAddr, err := camera.AddressFor(host)
	if err != nil {
		return nil, err
	}
	return camera.NewClient(netip.AddrPortFrom(camAddr, camera.HTTPPort), host.Addr(), s.f.httpTimeout), nil
}
