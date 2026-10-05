// Command gpwebcam makes a GoPro connected over USB usable as a Linux webcam
// through a v4l2loopback device.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/darkodemic/gpwebcam/internal/camera"
	"github.com/darkodemic/gpwebcam/internal/stream"
	"github.com/darkodemic/gpwebcam/internal/usbnet"
	"github.com/darkodemic/gpwebcam/internal/v4l2"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

const usage = `usage: gpwebcam <command> [flags]

commands:
  run       keep the loopback device fed: camera video whenever a GoPro
            is connected, a placeholder picture otherwise (service mode)
  start     stream one camera session, then exit
  list      list GoPro network interfaces
  version   print the version

Run "gpwebcam <command> -h" for the flags of a command.
`

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := run(os.Args[1:], os.Stdout, log); err != nil {
		if !errors.Is(err, flag.ErrHelp) {
			fmt.Fprintln(os.Stderr, "gpwebcam:", err)
		}
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer, log *slog.Logger) error {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usage)
		return flag.ErrHelp
	}
	switch args[0] {
	case "run":
		return cmdServe(args[1:], log, false)
	case "start":
		return cmdServe(args[1:], log, true)
	case "list":
		return cmdList(args[1:], stdout)
	case "version":
		fmt.Fprintln(stdout, "gpwebcam", version)
		return nil
	case "-h", "-help", "--help", "help":
		fmt.Fprint(stdout, usage)
		return nil
	default:
		fmt.Fprint(os.Stderr, usage)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func cmdList(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("list takes no arguments")
	}
	ifaces, err := usbnet.Find(usbnet.DefaultSysfs)
	if err != nil {
		return err
	}
	for _, i := range ifaces {
		fmt.Fprintf(stdout, "%s\t%s\t%s\n", i.Name, i.Product, i.USBPath)
	}
	return nil
}

// startFlags are the flags of both run and start.
type startFlags struct {
	iface       string
	res         camera.Resolution
	fov         camera.FOV
	port        uint16
	videoNr     int
	label       string
	ffmpeg      string
	dhcpWait    time.Duration
	connectWait time.Duration
	httpTimeout time.Duration
}

func parseStart(name string, args []string) (startFlags, error) {
	f := startFlags{res: camera.Res1080, fov: camera.FOVLinear, port: 8554}
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.StringVar(&f.iface, "iface", "", "GoPro network interface (default: the only one found in sysfs)")
	fs.Var(&f.res, "res", "resolution: 1080 or 720")
	fs.Var(&f.fov, "fov", "field of view: wide, narrow, superview or linear")
	fs.Func("port", "UDP port the camera streams to, 1024-65535 (default 8554)", func(s string) error {
		p, err := stream.Port(s)
		f.port = p
		return err
	})
	fs.IntVar(&f.videoNr, "video-nr", -1, "v4l2loopback device number, /dev/videoN; -1 finds the device by -device-label")
	fs.StringVar(&f.label, "device-label", v4l2.DefaultLabel, "card_label of the v4l2loopback device to use")
	fs.StringVar(&f.ffmpeg, "ffmpeg", "ffmpeg", "ffmpeg executable")
	fs.DurationVar(&f.dhcpWait, "dhcp-wait", 30*time.Second, "how long to wait for an IPv4 address on the interface")
	fs.DurationVar(&f.connectWait, "connect-wait", 20*time.Second, "how long to wait for the camera's HTTP server to answer")
	fs.DurationVar(&f.httpTimeout, "http-timeout", 5*time.Second, "timeout of each HTTP request to the camera")
	if err := fs.Parse(args); err != nil {
		return f, err
	}
	if fs.NArg() > 0 {
		return f, fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	if f.iface != "" {
		if err := usbnet.ValidateName(f.iface); err != nil {
			return f, err
		}
	}
	if f.videoNr < -1 || f.videoNr > v4l2.MaxDeviceNumber {
		return f, fmt.Errorf("-video-nr %d: must be -1 (find by label) or 0 to %d", f.videoNr, v4l2.MaxDeviceNumber)
	}
	// card_label is at most 31 bytes in v4l2loopback.
	if f.label == "" || len(f.label) > 31 {
		return f, fmt.Errorf("-device-label %q: must be 1 to 31 bytes", f.label)
	}
	if f.dhcpWait <= 0 || f.connectWait <= 0 || f.httpTimeout <= 0 {
		return f, fmt.Errorf("-dhcp-wait, -connect-wait and -http-timeout must be positive")
	}
	if strings.ContainsRune(f.ffmpeg, '/') {
		return f, nil
	}
	p, err := exec.LookPath(f.ffmpeg)
	if err != nil {
		return f, fmt.Errorf("ffmpeg: %w", err)
	}
	f.ffmpeg = p
	return f, nil
}

// errNoCamera means no GoPro interface is present right now.
var errNoCamera = fmt.Errorf("no GoPro network interface (USB vendor %s) found; is the camera on and set to GoPro Connect?", usbnet.GoProVendorID)

func findIface(name string) (usbnet.Interface, error) {
	if name != "" {
		return usbnet.Lookup(usbnet.DefaultSysfs, name)
	}
	found, err := usbnet.Find(usbnet.DefaultSysfs)
	if err != nil {
		return usbnet.Interface{}, err
	}
	switch len(found) {
	case 0:
		return usbnet.Interface{}, errNoCamera
	case 1:
		return found[0], nil
	default:
		names := make([]string, len(found))
		for i, f := range found {
			names[i] = f.Name
		}
		return usbnet.Interface{}, fmt.Errorf("several GoPro interfaces (%s); choose one with -iface", strings.Join(names, ", "))
	}
}
