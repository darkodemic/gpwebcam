package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/darkodemic/gpwebcam/internal/settings"
)

const configUsage = `usage: gpwebcam config [<setting> [<value>]]

Shows or changes the settings that the tray menu changes too. Without
arguments it prints them all. A running "gpwebcam run" applies a change
within a few seconds; a new resolution applies once no application uses
the camera. A flag given to "gpwebcam run" overrides the file.

settings:
  camera  demand (while an application uses it), always or off
  res     1080 or 720
  fov     wide, narrow, superview or linear
  hwdec   auto or none
  notify  on or off
  tray    on or off
`

func cmdConfig(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("config", flag.ContinueOnError)
	fs.Usage = func() { fmt.Fprint(fs.Output(), configUsage) }
	if err := fs.Parse(args); err != nil {
		return err
	}
	path, err := settings.Path()
	if err != nil {
		return err
	}
	s, err := settings.Load(path)
	var uk *settings.UnknownKeysError
	switch {
	case errors.As(err, &uk):
		fmt.Fprintln(os.Stderr, "gpwebcam:", err)
	case err != nil:
		// Do not replace a file the user may want to fix by hand.
		return fmt.Errorf("%w; fix or delete the file", err)
	}
	switch fs.NArg() {
	case 0:
		fmt.Fprintf(stdout, "# %s\n", path)
		for _, k := range settings.Keys {
			fmt.Fprintf(stdout, "%s=%s\n", k, s.Get(k))
		}
		return nil
	case 1:
		k := fs.Arg(0)
		probe := s
		if err := probe.Set(k, s.Get(k)); err != nil {
			return err
		}
		fmt.Fprintln(stdout, s.Get(k))
		return nil
	case 2:
		if err := s.Set(fs.Arg(0), fs.Arg(1)); err != nil {
			return err
		}
		return settings.Save(path, s)
	default:
		fs.Usage()
		return fmt.Errorf("config takes at most a setting and a value")
	}
}
