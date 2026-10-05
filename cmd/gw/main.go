// Command gw makes a GoPro connected over USB usable as a Linux webcam
// through a v4l2loopback device.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/darkodemic/gw/internal/camera"
	"github.com/darkodemic/gw/internal/stream"
	"github.com/darkodemic/gw/internal/usbnet"
	"github.com/darkodemic/gw/internal/v4l2"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

const usage = `usage: gw <command> [flags]

commands:
  start     stream the camera into a v4l2loopback device
  list      list GoPro network interfaces
  version   print the version

Run "gw <command> -h" for the flags of a command.
`

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := run(os.Args[1:], os.Stdout, log); err != nil {
		if !errors.Is(err, flag.ErrHelp) {
			fmt.Fprintln(os.Stderr, "gw:", err)
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
	case "start":
		return cmdStart(args[1:], log)
	case "list":
		return cmdList(args[1:], stdout)
	case "version":
		fmt.Fprintln(stdout, "gw", version)
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

type startFlags struct {
	iface       string
	res         camera.Resolution
	fov         camera.FOV
	port        uint16
	videoNr     int
	ffmpeg      string
	dhcpWait    time.Duration
	connectWait time.Duration
	httpTimeout time.Duration
}

func parseStart(args []string) (startFlags, error) {
	f := startFlags{res: camera.Res1080, fov: camera.FOVLinear, port: 8554}
	fs := flag.NewFlagSet("start", flag.ContinueOnError)
	fs.StringVar(&f.iface, "iface", "", "GoPro network interface (default: the only one found in sysfs)")
	fs.Var(&f.res, "res", "resolution: 1080 or 720")
	fs.Var(&f.fov, "fov", "field of view: wide, narrow, superview or linear")
	fs.Func("port", "UDP port the camera streams to, 1024-65535 (default 8554)", func(s string) error {
		p, err := stream.Port(s)
		f.port = p
		return err
	})
	fs.IntVar(&f.videoNr, "video-nr", 42, "v4l2loopback device number, /dev/videoN")
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
		return usbnet.Interface{}, fmt.Errorf("no GoPro network interface (USB vendor %s) found; is the camera on and set to GoPro Connect?", usbnet.GoProVendorID)
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

func cmdStart(args []string, log *slog.Logger) error {
	f, err := parseStart(args)
	if err != nil {
		return err
	}
	device, err := v4l2.DevicePath(f.videoNr)
	if err != nil {
		return err
	}
	info, err := v4l2.CheckLoopback(device)
	if err != nil {
		return err
	}
	iface, err := findIface(f.iface)
	if err != nil {
		return err
	}
	log.Info("found camera", "iface", iface.Name, "product", iface.Product, "device", device, "card", info.Card)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	// After the first signal, a second one kills gw at once, in case
	// stopping the camera hangs; ffmpeg dies with it through Pdeathsig.
	go func() {
		<-ctx.Done()
		stop()
	}()

	waitCtx, cancel := context.WithTimeout(ctx, f.dhcpWait)
	host, err := usbnet.WaitIPv4(waitCtx, iface.Name, 250*time.Millisecond)
	cancel()
	if err != nil {
		return interrupted(ctx, err, log)
	}
	camAddr, err := camera.AddressFor(host)
	if err != nil {
		return err
	}
	log.Info("link is up", "host", host, "camera", camAddr)

	cam := camera.NewClient(netip.AddrPortFrom(camAddr, camera.HTTPPort), host.Addr(), f.httpTimeout)
	err = cam.StartWebcam(ctx, camera.StartOptions{
		Res:       f.res,
		FOV:       f.fov,
		Port:      f.port,
		Poll:      500 * time.Millisecond,
		Connect:   f.connectWait,
		Streaming: 10 * time.Second,
	})
	// Stop even after a failed start: the camera may be half way into
	// webcam mode.
	defer func() {
		// An unplugged camera has nothing left to stop.
		if _, err := net.InterfaceByName(iface.Name); err != nil {
			log.Info("camera is gone, nothing to stop", "iface", iface.Name)
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
		return interrupted(ctx, err, log)
	}
	log.Info("webcam started", "res", f.res, "fov", f.fov, "port", f.port)

	kctx, cancelKeepAlive := context.WithCancel(ctx)
	defer cancelKeepAlive()
	go keepAlive(kctx, cam, f.httpTimeout, log)

	err = stream.Run(ctx, stream.Config{
		FFmpeg:      f.ffmpeg,
		Listen:      netip.AddrPortFrom(host.Addr(), f.port),
		Device:      device,
		ReadTimeout: 5 * time.Second,
	}, os.Stderr, 3*time.Second)
	cancelKeepAlive()
	if err == nil {
		log.Info("stopped on signal")
	}
	return err
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
