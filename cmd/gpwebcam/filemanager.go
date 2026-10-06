package main

import (
	"context"
	"net/url"
	"os"
	"time"

	"github.com/godbus/dbus/v5"
)

// showFolder asks the desktop's file manager, over the session D-Bus
// (org.freedesktop.FileManager1), to show dir. D-Bus starts the file
// manager outside gpwebcam's sandbox, which a child process would share.
func showFolder(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return err
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	u := (&url.URL{Scheme: "file", Path: dir}).String()
	return conn.Object("org.freedesktop.FileManager1", "/org/freedesktop/FileManager1").
		CallWithContext(ctx, "org.freedesktop.FileManager1.ShowFolders", 0, []string{u}, "").Err
}
