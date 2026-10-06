package main

import (
	"context"
	"os"
	"time"

	"github.com/godbus/dbus/v5"
)

// underSystemd reports whether gpwebcam runs as a systemd service, which
// sets INVOCATION_ID for every unit it starts.
func underSystemd() bool { return os.Getenv("INVOCATION_ID") != "" }

// restartService asks the user's systemd, over the session D-Bus, to
// restart the unit this process runs in. systemd then stops this process.
func restartService() error {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return err
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	const dest = "org.freedesktop.systemd1"
	var unit dbus.ObjectPath
	if err := conn.Object(dest, "/org/freedesktop/systemd1").CallWithContext(ctx,
		"org.freedesktop.systemd1.Manager.GetUnitByPID", 0, uint32(os.Getpid())).Store(&unit); err != nil {
		return err
	}
	var job dbus.ObjectPath
	return conn.Object(dest, unit).CallWithContext(ctx,
		"org.freedesktop.systemd1.Unit.Restart", 0, "replace").Store(&job)
}
