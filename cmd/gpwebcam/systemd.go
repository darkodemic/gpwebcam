package main

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/godbus/dbus/v5"
)

// serviceUnit is the packaged systemd user unit.
const serviceUnit = "gpwebcam.service"

const (
	systemdDest    = "org.freedesktop.systemd1"
	systemdPath    = "/org/freedesktop/systemd1"
	systemdManager = "org.freedesktop.systemd1.Manager"
	systemdUnit    = "org.freedesktop.systemd1.Unit"
)

// underSystemd reports whether gpwebcam runs as a systemd service, which
// sets INVOCATION_ID for every unit it starts.
func underSystemd() bool { return os.Getenv("INVOCATION_ID") != "" }

// withSystemd calls f with a connection to the user's systemd over the
// session D-Bus and a deadline for the calls.
func withSystemd(f func(ctx context.Context, conn *dbus.Conn) error) error {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return err
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return f(ctx, conn)
}

// ownUnit returns the object path of the unit this process runs in.
func ownUnit(ctx context.Context, conn *dbus.Conn) (dbus.ObjectPath, error) {
	var unit dbus.ObjectPath
	err := conn.Object(systemdDest, systemdPath).CallWithContext(ctx,
		systemdManager+".GetUnitByPID", 0, uint32(os.Getpid())).Store(&unit)
	return unit, err
}

// ownUnitDo asks systemd to restart or stop the unit this process runs in;
// systemd then stops this process.
func ownUnitDo(method string) error {
	return withSystemd(func(ctx context.Context, conn *dbus.Conn) error {
		unit, err := ownUnit(ctx, conn)
		if err != nil {
			return err
		}
		var job dbus.ObjectPath
		return conn.Object(systemdDest, unit).CallWithContext(ctx, systemdUnit+"."+method, 0, "replace").Store(&job)
	})
}

func restartService() error { return ownUnitDo("Restart") }
func stopService() error    { return ownUnitDo("Stop") }

// serviceState returns the unit's ActiveState: active, inactive, failed,
// activating and so on.
func serviceState(ctx context.Context, conn *dbus.Conn) (string, error) {
	var unit dbus.ObjectPath
	if err := conn.Object(systemdDest, systemdPath).CallWithContext(ctx,
		systemdManager+".LoadUnit", 0, serviceUnit).Store(&unit); err != nil {
		return "", err
	}
	v, err := conn.Object(systemdDest, unit).GetProperty(systemdUnit + ".ActiveState")
	if err != nil {
		return "", err
	}
	state, ok := v.Value().(string)
	if !ok {
		return "", errors.New("unexpected ActiveState")
	}
	return state, nil
}

// startService starts the user service unless it runs, and waits until it
// is active. It reports whether it was running already.
func startService() (wasRunning bool, err error) {
	err = withSystemd(func(ctx context.Context, conn *dbus.Conn) error {
		state, err := serviceState(ctx, conn)
		if err != nil {
			return err
		}
		if state == "active" {
			wasRunning = true
			return nil
		}
		var job dbus.ObjectPath
		if err := conn.Object(systemdDest, systemdPath).CallWithContext(ctx,
			systemdManager+".StartUnit", 0, serviceUnit, "replace").Store(&job); err != nil {
			return err
		}
		for {
			state, err := serviceState(ctx, conn)
			if err != nil {
				return err
			}
			switch state {
			case "active":
				return nil
			case "failed":
				return errors.New("gpwebcam.service failed to start")
			}
			select {
			case <-ctx.Done():
				return errors.New("gpwebcam.service did not start in time")
			case <-time.After(200 * time.Millisecond):
			}
		}
	})
	return wasRunning, err
}
