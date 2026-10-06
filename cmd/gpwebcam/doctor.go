package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/darkodemic/gpwebcam/internal/camera"
	"github.com/darkodemic/gpwebcam/internal/record"
	"github.com/darkodemic/gpwebcam/internal/settings"
	"github.com/darkodemic/gpwebcam/internal/stream"
	"github.com/darkodemic/gpwebcam/internal/usbnet"
	"github.com/darkodemic/gpwebcam/internal/v4l2"
)

// errChecksFailed makes doctor exit with status 1 after it printed why.
var errChecksFailed = errors.New("some checks failed")

// report prints check results and counts problems.
type report struct {
	w            io.Writer
	fails, warns int
}

func (r *report) section(name string) { fmt.Fprintf(r.w, "\n%s\n", name) }

func (r *report) ok(format string, a ...any) {
	fmt.Fprintf(r.w, "  ok    %s\n", fmt.Sprintf(format, a...))
}

func (r *report) warn(msg string, hints ...string) {
	r.warns++
	r.line("WARN", msg, hints)
}

func (r *report) fail(msg string, hints ...string) {
	r.fails++
	r.line("FAIL", msg, hints)
}

func (r *report) line(level, msg string, hints []string) {
	fmt.Fprintf(r.w, "  %-4s  %s\n", level, msg)
	for _, h := range hints {
		fmt.Fprintf(r.w, "        %s\n", h)
	}
}

// doctorFlags are the settings doctor checks against; they match run's.
type doctorFlags struct {
	ffmpeg  string
	iface   string
	videoNr int
	label   string
	port    uint16
	timeout time.Duration
}

func cmdDoctor(args []string, stdout io.Writer) error {
	f := doctorFlags{port: 8554}
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.StringVar(&f.ffmpeg, "ffmpeg", "ffmpeg", "ffmpeg executable")
	fs.StringVar(&f.iface, "iface", "", "GoPro network interface (default: every one found)")
	fs.IntVar(&f.videoNr, "video-nr", -1, "v4l2loopback device number; -1 finds the device by -device-label")
	fs.StringVar(&f.label, "device-label", v4l2.DefaultLabel, "card_label of the v4l2loopback device")
	fs.Func("port", "UDP port the camera streams to (default 8554)", func(s string) error {
		p, err := stream.Port(s)
		f.port = p
		return err
	})
	fs.DurationVar(&f.timeout, "http-timeout", 3*time.Second, "timeout of each request to the camera")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	if f.iface != "" {
		if err := usbnet.ValidateName(f.iface); err != nil {
			return err
		}
	}
	if f.videoNr < -1 || f.videoNr > v4l2.MaxDeviceNumber || f.label == "" || len(f.label) > 31 || f.timeout <= 0 {
		return fmt.Errorf("invalid -video-nr, -device-label or -http-timeout")
	}

	r := &report{w: stdout}
	fmt.Fprintf(stdout, "gpwebcam %s doctor\n", version)
	checkFFmpeg(r, f.ffmpeg)
	checkLoopback(r, f)
	checkService(r)
	set := checkSettings(r)
	checkCamera(r, f)
	checkFirewall(r, f.port)
	checkNotifications(r)
	checkTray(r, set)
	checkRecordings(r)

	fmt.Fprintf(stdout, "\n%d problem(s), %d warning(s).\n", r.fails, r.warns)
	if r.fails > 0 {
		return errChecksFailed
	}
	return nil
}

// output runs a program with a timeout and returns its standard output.
func output(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).Output()
	return string(out), err
}

func checkFFmpeg(r *report, name string) {
	r.section("ffmpeg")
	path, err := exec.LookPath(name)
	if err != nil {
		r.fail("ffmpeg is not installed",
			"Arch: sudo pacman -S ffmpeg; Debian, Ubuntu: sudo apt install ffmpeg;",
			"Fedora: ffmpeg from RPM Fusion (sudo dnf swap ffmpeg-free ffmpeg --allowerasing).")
		return
	}
	ver := "(unknown version)"
	if out, err := output(path, "-hide_banner", "-version"); err == nil {
		if fields := strings.Fields(out); len(fields) >= 3 {
			ver = fields[2]
		}
	}
	out, err := output(path, "-hide_banner", "-decoders")
	if err != nil {
		r.fail(fmt.Sprintf("%s does not run: %v", path, err))
		return
	}
	dec := h264Decoders(out)
	switch {
	case contains(dec, "h264"):
		r.ok("ffmpeg %s (%s) with the h264 decoder", ver, path)
	case contains(dec, "libopenh264"):
		r.warn(fmt.Sprintf("ffmpeg %s (%s) decodes H.264 only through libopenh264, which is untested with GoPro streams", ver, path),
			"Fedora: prefer ffmpeg from RPM Fusion (sudo dnf swap ffmpeg-free ffmpeg --allowerasing).")
	default:
		r.fail(fmt.Sprintf("ffmpeg %s (%s) has no H.264 decoder", ver, path),
			"Fedora: install ffmpeg from RPM Fusion (sudo dnf swap ffmpeg-free ffmpeg --allowerasing),",
			"or the openh264 library from the Cisco repository.")
	}
	if err := stream.ProbeVAAPI(context.Background(), path); err == nil {
		r.ok("VAAPI hardware decoding works; -hwdec auto uses it")
	} else {
		r.ok("no VAAPI hardware decoding; gpwebcam decodes in software")
	}
}

// h264Decoders returns the software H.264 decoders in `ffmpeg -decoders`
// output: the native h264 and libopenh264, not hardware wrappers.
func h264Decoders(out string) []string {
	var names []string
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) >= 2 && strings.HasPrefix(f[0], "V") && (f[1] == "h264" || f[1] == "libopenh264") {
			names = append(names, f[1])
		}
	}
	return names
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func checkLoopback(r *report, f doctorFlags) {
	r.section("v4l2loopback")
	ver, err := os.ReadFile("/sys/module/v4l2loopback/version")
	if err != nil {
		r.fail("the v4l2loopback module is not loaded",
			"Install it: v4l2loopback-dkms (Arch, Debian, Ubuntu) or akmod-v4l2loopback (Fedora, RPM Fusion),",
			"then reboot, or load it now: sudo modprobe v4l2loopback")
	} else {
		r.ok("module version %s is loaded", strings.TrimSpace(string(ver)))
	}

	devs := loopbackDevices("/sys")
	if len(devs) == 0 && err == nil {
		r.fail("the module is loaded, but there is no loopback device")
	}
	for _, d := range devs {
		fmt.Fprintf(r.w, "        %s %q\n", d.path, d.name)
	}

	opts, optsErr := moduleOptions()
	switch {
	case optsErr != nil:
		r.warn(fmt.Sprintf("cannot read the module configuration: %v", optsErr))
	case len(opts) == 0:
		r.warn("no v4l2loopback options are configured",
			"The gpwebcam package installs /usr/lib/modprobe.d/99-gpwebcam.conf; reinstall it, or create a file like it.")
	default:
		for _, o := range opts {
			r.ok("%s", o)
		}
	}
	if _, err := os.Stat("/etc/modprobe.d/99-gpwebcam.conf"); err == nil {
		r.warn("/etc/modprobe.d/99-gpwebcam.conf replaces the packaged configuration")
	}

	f2 := startFlags{videoNr: f.videoNr, label: f.label}
	path, err := findDevice(f2)
	if err != nil {
		r.fail(err.Error(),
			"Reboot after installing the package, so the module is loaded with its configuration.")
		return
	}
	info, err := v4l2.CheckLoopback(path)
	switch {
	case errors.Is(err, os.ErrPermission):
		r.fail(err.Error(),
			"Log in on a local seat (systemd gives that user access), or join the video group:",
			"sudo usermod -aG video $USER, then log in again.")
		return
	case err != nil:
		r.fail(err.Error())
		return
	}
	r.ok("%s %q is a v4l2loopback device this user can write to", path, info.Card)
	u, err := v4l2.WatchUsage(path)
	switch {
	case errors.Is(err, v4l2.ErrNoUsageEvents):
		r.warn("this v4l2loopback does not report when applications use the camera, so camera mode demand works like always",
			"Newer v4l2loopback versions report it; 0.15.4 does.")
	case err != nil:
		r.warn(fmt.Sprintf("cannot watch %s for applications using it: %v", path, err))
	default:
		u.Close()
		r.ok("the module reports when applications use the camera, so camera mode demand works")
	}
}

type loopDev struct{ path, name string }

// loopbackDevices lists the virtual video4linux devices, which on a normal
// system are the v4l2loopback ones.
func loopbackDevices(sysfs string) []loopDev {
	dir := filepath.Join(sysfs, "class", "video4linux")
	entries, _ := os.ReadDir(dir)
	var out []loopDev
	for _, e := range entries {
		real, err := filepath.EvalSymlinks(filepath.Join(dir, e.Name()))
		if err != nil || !strings.Contains(real, "/devices/virtual/") || !strings.HasPrefix(e.Name(), "video") {
			continue
		}
		name, _ := os.ReadFile(filepath.Join(real, "name"))
		out = append(out, loopDev{path: "/dev/" + e.Name(), name: strings.TrimSpace(string(name))})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].path < out[j].path })
	return out
}

// moduleOptions returns the v4l2loopback lines of the effective modprobe
// configuration.
func moduleOptions() ([]string, error) {
	if _, err := exec.LookPath("modprobe"); err != nil {
		return nil, errors.New("modprobe is not installed")
	}
	out, err := output("modprobe", "-c")
	if err != nil {
		return nil, fmt.Errorf("modprobe -c: %w", err)
	}
	return optionLines(out), nil
}

func optionLines(modprobeC string) []string {
	var lines []string
	for _, l := range strings.Split(modprobeC, "\n") {
		if strings.HasPrefix(l, "options v4l2loopback ") {
			lines = append(lines, strings.TrimSpace(l))
		}
	}
	return lines
}

func checkService(r *report) {
	r.section("service")
	if _, err := exec.LookPath("systemctl"); err != nil {
		r.warn("systemctl not found; start gpwebcam run yourself")
		return
	}
	unreachable := func() {
		r.warn("cannot reach this user's systemd",
			"Run gpwebcam doctor as yourself in your desktop session, not with sudo or in a container.")
	}
	// is-system-running prints a state even when the manager is degraded;
	// nothing at all means this process cannot reach the user's systemd.
	if state, _ := output("systemctl", "--user", "is-system-running"); strings.TrimSpace(state) == "" {
		unreachable()
		return
	}
	enabled, _ := output("systemctl", "--user", "is-enabled", "gpwebcam.service")
	active, _ := output("systemctl", "--user", "is-active", "gpwebcam.service")
	enabled, active = strings.TrimSpace(enabled), strings.TrimSpace(active)
	if active == "" {
		// is-enabled reads unit files without the manager; is-active
		// needs it. In a container is-system-running says "offline".
		unreachable()
		return
	}
	switch {
	case enabled == "" || enabled == "not-found":
		r.warn("gpwebcam.service is not installed for systemd",
			"The package installs it to /usr/lib/systemd/user/.")
	case active == "active":
		r.ok("gpwebcam.service is %s and %s", enabled, active)
	case active == "failed":
		r.fail("gpwebcam.service failed",
			"See: journalctl --user -u gpwebcam -e")
	default:
		r.warn(fmt.Sprintf("gpwebcam.service is %s and %s", enabled, active),
			"Start it: systemctl --user enable --now gpwebcam.service")
	}
}

func checkCamera(r *report, f doctorFlags) {
	r.section("camera")
	usb := goproUSB("/sys")
	if len(usb) == 0 {
		r.warn("no GoPro on USB",
			"Turn the camera on and connect it with a cable that carries data.")
		return
	}
	for _, u := range usb {
		if testedModels[u.product] {
			r.ok("%s on USB (%s Mb/s)", u.product, u.speed)
		} else {
			r.warn(fmt.Sprintf("%s on USB (%s Mb/s) has not been tested with gpwebcam", u.product, u.speed),
				"It may work; please report whether it does: https://github.com/darkodemic/gpwebcam/issues")
		}
	}

	var ifaces []usbnet.Interface
	if f.iface != "" {
		i, err := usbnet.Lookup(usbnet.DefaultSysfs, f.iface)
		if err != nil {
			r.fail(err.Error())
			return
		}
		ifaces = []usbnet.Interface{i}
	} else {
		ifaces, _ = usbnet.Find(usbnet.DefaultSysfs)
	}
	if len(ifaces) == 0 {
		r.fail("the GoPro is on USB but has no network interface",
			"On the camera set Preferences > Connections > USB Connection to GoPro Connect, not MTP.",
			"A camera that was just plugged in needs a few seconds.")
		return
	}
	for _, iface := range ifaces {
		checkLink(r, f, iface)
	}
}

func checkLink(r *report, f doctorFlags, iface usbnet.Interface) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	host, err := usbnet.WaitIPv4(ctx, iface.Name, 100*time.Millisecond)
	cancel()
	if err != nil {
		r.fail(fmt.Sprintf("%s has no IPv4 address", iface.Name),
			"The camera gives one by DHCP. Make sure NetworkManager or systemd-networkd manages the",
			"interface with IPv4 set to automatic (see: nmcli device).")
		return
	}
	camAddr, err := camera.AddressFor(host)
	if err != nil {
		r.fail(err.Error(),
			`NetworkManager "shared" mode: nmcli connection modify "<connection>" ipv4.method auto,`,
			`then nmcli connection up "<connection>".`)
		return
	}
	r.ok("%s has %s; the camera is %s", iface.Name, host, camAddr)

	// The kernel picks the source address for a route; any other than the
	// host address on this link means traffic to the camera goes elsewhere.
	if c, err := net.DialUDP("udp4", nil, net.UDPAddrFromAddrPort(netip.AddrPortFrom(camAddr, f.port))); err == nil {
		local := c.LocalAddr().(*net.UDPAddr).AddrPort().Addr().Unmap()
		c.Close()
		if local != host.Addr() {
			r.warn(fmt.Sprintf("traffic to the camera leaves from %s, not %s", local, host.Addr()),
				"A VPN or another route may take it; check: ip route get "+camAddr.String())
		} else {
			r.ok("the route to the camera goes through %s", iface.Name)
		}
	}

	cam := camera.NewClient(netip.AddrPortFrom(camAddr, camera.HTTPPort), host.Addr(), f.timeout)
	ctx, cancel = context.WithTimeout(context.Background(), 2*f.timeout)
	defer cancel()
	info, err := cam.Info(ctx)
	if err != nil {
		r.fail(fmt.Sprintf("the camera does not answer: %v", err),
			"Unplug and replug the cable; a camera turned off and on with the cable attached can hang.")
		return
	}
	r.ok("%s, firmware %s", info.Model, info.Firmware)
	if st, err := cam.Status(ctx); err == nil {
		r.ok("webcam status: %s", st.Status)
	} else {
		r.warn(fmt.Sprintf("webcam status: %v", err))
	}
}

type usbDev struct{ product, speed string }

// goproUSB lists USB devices with the GoPro vendor ID.
func goproUSB(sysfs string) []usbDev {
	dirs, _ := filepath.Glob(filepath.Join(sysfs, "bus", "usb", "devices", "*"))
	var out []usbDev
	for _, d := range dirs {
		v, err := os.ReadFile(filepath.Join(d, "idVendor"))
		if err != nil || strings.TrimSpace(string(v)) != usbnet.GoProVendorID {
			continue
		}
		p, _ := os.ReadFile(filepath.Join(d, "product"))
		s, _ := os.ReadFile(filepath.Join(d, "speed"))
		out = append(out, usbDev{product: strings.TrimSpace(string(p)), speed: strings.TrimSpace(string(s))})
	}
	return out
}

func checkFirewall(r *report, port uint16) {
	r.section("firewall")
	found := false
	for _, svc := range []string{"firewalld", "ufw"} {
		out, _ := output("systemctl", "is-active", svc)
		if strings.TrimSpace(out) != "active" {
			continue
		}
		found = true
		switch svc {
		case "firewalld":
			r.warn("firewalld is active and may drop the camera's video",
				fmt.Sprintf("Allow it: sudo firewall-cmd --add-port=%d/udp (add --permanent to keep it).", port))
		case "ufw":
			r.warn("ufw is active and may drop the camera's video",
				fmt.Sprintf("Allow it: sudo ufw allow in on <GoPro interface> to any port %d proto udp", port))
		}
	}
	if !found {
		r.ok("neither firewalld nor ufw is running (nftables rules need root to check)")
	}
}

func checkSettings(r *report) settings.Settings {
	r.section("settings")
	path, err := settings.Path()
	if err != nil {
		r.warn(err.Error())
		return settings.Defaults()
	}
	s, err := settings.Load(path)
	var uk *settings.UnknownKeysError
	switch {
	case errors.As(err, &uk):
		r.warn(err.Error(), "A typo, or settings of another gpwebcam version; the known ones apply.")
	case err != nil:
		r.warn(err.Error(), "gpwebcam uses the defaults until the file is fixed or deleted.")
	case !fileExists(path):
		r.ok("no settings file yet, the defaults apply (%s)", path)
	default:
		parts := make([]string, len(settings.Keys))
		for i, k := range settings.Keys {
			parts[i] = k + "=" + s.Get(k)
		}
		r.ok("%s: %s", path, strings.Join(parts, " "))
	}
	return s
}

// checkTray looks for a StatusNotifierItem host: KDE, Quickshell, Waybar
// and GNOME with the AppIndicator extension register this watcher name.
func checkTray(r *report, s settings.Settings) {
	r.section("tray")
	if !s.Tray {
		r.ok("the tray icon is off; turn it on with: gpwebcam config tray on")
		return
	}
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		r.warn("no session D-Bus; the tray icon will not show",
			"Run gpwebcam doctor as yourself in your desktop session, not with sudo or in a container.")
		return
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var has bool
	err = conn.BusObject().CallWithContext(ctx, "org.freedesktop.DBus.NameHasOwner", 0, "org.kde.StatusNotifierWatcher").Store(&has)
	if err != nil || !has {
		r.warn("this desktop shows no system tray, so the icon will not show",
			"GNOME needs the AppIndicator extension. Without a tray, use: gpwebcam config")
		return
	}
	r.ok("the desktop has a system tray")
}

// checkRecordings looks at the default recordings folder; the packaged
// unit lets the service write to ~/Videos only.
func checkRecordings(r *report) {
	r.section("recordings")
	dir, err := defaultRecordDir()
	if err != nil {
		r.warn(err.Error())
		return
	}
	videos := filepath.Dir(dir)
	if info, err := os.Stat(videos); err != nil || !info.IsDir() {
		r.warn(fmt.Sprintf("%s does not exist, so the service cannot record there", videos),
			"Create it: mkdir ~/Videos, then: systemctl --user restart gpwebcam")
		return
	}
	var st syscall.Statfs_t
	if err := syscall.Statfs(videos, &st); err == nil {
		if free := st.Bavail * uint64(st.Bsize); free < record.MinFree {
			r.warn(fmt.Sprintf("only %s free in %s; a recording needs %s to start", record.SizeText(free), videos, record.SizeText(record.MinFree)))
			return
		}
	}
	r.ok("recordings go to %s (with -record-dir, to that folder)", dir)
}

func checkNotifications(r *report) {
	r.section("notifications")
	if _, err := exec.LookPath("notify-send"); err != nil {
		r.warn("notify-send not found; desktop notifications are off",
			"Install libnotify (Arch: libnotify; Debian, Ubuntu: libnotify-bin; Fedora: libnotify).")
		return
	}
	if os.Getenv("DBUS_SESSION_BUS_ADDRESS") == "" && !fileExists(fmt.Sprintf("/run/user/%d/bus", syscall.Getuid())) {
		r.warn("no session D-Bus; notifications will not show")
		return
	}
	r.ok("notify-send is available")
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
