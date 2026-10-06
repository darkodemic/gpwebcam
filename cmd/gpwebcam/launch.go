package main

import (
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/darkodemic/gpwebcam/internal/notify"
	"github.com/darkodemic/gpwebcam/internal/settings"
)

const launchUsage = `usage: gpwebcam launch

Starts the user service gpwebcam.service, as "systemctl --user start
gpwebcam" does, and says so in a notification. The GoPro Webcam entry in
the application menu runs it, so the service can be started without a
terminal, for example after Quit in the tray menu.
`

func cmdLaunch(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("launch", flag.ContinueOnError)
	fs.Usage = func() { fmt.Fprint(fs.Output(), launchUsage) }
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		fs.Usage()
		return errors.New("launch takes no arguments")
	}
	// The menu entry has no terminal, so the outcome goes into a
	// notification too.
	n := notify.New()
	wasRunning, err := startService()
	if err != nil {
		n.SendNow(notify.Normal, "gpwebcam did not start", err.Error()+". See: journalctl --user -u gpwebcam")
		return err
	}
	where := "Its icon is in the system tray."
	if path, perr := settings.Path(); perr == nil {
		if s, _ := settings.Load(path); !s.Tray {
			where = "The tray icon is hidden; show it with: gpwebcam config tray on"
		}
	}
	if wasRunning {
		fmt.Fprintln(stdout, "gpwebcam is already running.")
		n.SendNow(notify.Low, "gpwebcam is already running", where)
		return nil
	}
	fmt.Fprintln(stdout, "gpwebcam started.")
	n.SendNow(notify.Low, "gpwebcam started", "The GoPro camera is ready. "+where)
	return nil
}
