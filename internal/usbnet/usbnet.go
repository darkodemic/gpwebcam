// Package usbnet finds the network interfaces that a GoPro exposes over USB
// and waits for the host side of that link to get an IPv4 address.
//
// Interfaces are identified by the USB vendor ID of the device behind them,
// read from sysfs, never by guessing from names or link state.
package usbnet

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// GoProVendorID is the USB vendor ID of GoPro devices, as sysfs prints it.
const GoProVendorID = "2672"

// DefaultSysfs is the mount point of sysfs.
const DefaultSysfs = "/sys"

// Interface is a network interface backed by a GoPro USB device.
type Interface struct {
	Name    string // kernel interface name, e.g. "enx0a1b2c3d4e5f"
	USBPath string // sysfs directory of the USB device
	Product string // USB product string, e.g. "HERO13 Black"; may be empty
	Serial  string // USB serial number; may be empty
}

// ValidateName checks that name can be a Linux network interface name.
// It runs before the name is used in a sysfs path.
func ValidateName(name string) error {
	// IFNAMSIZ is 16 including the terminating NUL.
	if name == "" || len(name) > 15 {
		return fmt.Errorf("interface name %q: must be 1 to 15 bytes", name)
	}
	if name == "." || name == ".." {
		return fmt.Errorf("interface name %q is not allowed", name)
	}
	for _, r := range name {
		if r == '/' || r == ':' || r <= ' ' || r > '~' {
			return fmt.Errorf("interface name %q: contains %q", name, r)
		}
	}
	return nil
}

// Find returns every network interface whose USB device has the GoPro vendor
// ID, sorted by name. sysfs is the sysfs mount point, normally DefaultSysfs.
func Find(sysfs string) ([]Interface, error) {
	entries, err := os.ReadDir(filepath.Join(sysfs, "class", "net"))
	if err != nil {
		return nil, fmt.Errorf("list network interfaces: %w", err)
	}
	var found []Interface
	for _, e := range entries {
		iface, err := lookup(sysfs, e.Name())
		if err != nil {
			continue // not a USB device, or not a GoPro
		}
		found = append(found, iface)
	}
	sort.Slice(found, func(i, j int) bool { return found[i].Name < found[j].Name })
	return found, nil
}

// Lookup returns the interface called name if a GoPro is behind it.
func Lookup(sysfs, name string) (Interface, error) {
	if err := ValidateName(name); err != nil {
		return Interface{}, err
	}
	return lookup(sysfs, name)
}

// errNotGoPro means the interface exists but no GoPro USB device is behind it.
var errNotGoPro = errors.New("not a GoPro USB network interface")

func lookup(sysfs, name string) (Interface, error) {
	netDir := filepath.Join(sysfs, "class", "net", name)
	if _, err := os.Stat(netDir); err != nil {
		return Interface{}, fmt.Errorf("interface %s: %w", name, err)
	}
	// "device" links to the USB interface (e.g. .../1-2/1-2:1.0). The USB
	// device is the nearest ancestor that has an idVendor attribute.
	dev, err := filepath.EvalSymlinks(filepath.Join(netDir, "device"))
	if err != nil {
		return Interface{}, fmt.Errorf("interface %s: %w", name, errNotGoPro)
	}
	devices, err := filepath.EvalSymlinks(filepath.Join(sysfs, "devices"))
	if err != nil {
		return Interface{}, fmt.Errorf("interface %s: %w", name, err)
	}
	for dir := dev; strings.HasPrefix(dir, devices+string(filepath.Separator)); dir = filepath.Dir(dir) {
		vendor, err := readAttr(dir, "idVendor")
		if err != nil {
			continue
		}
		if vendor != GoProVendorID {
			return Interface{}, fmt.Errorf("interface %s: USB vendor %s: %w", name, vendor, errNotGoPro)
		}
		product, _ := readAttr(dir, "product")
		serial, _ := readAttr(dir, "serial")
		return Interface{Name: name, USBPath: dir, Product: product, Serial: serial}, nil
	}
	return Interface{}, fmt.Errorf("interface %s: %w", name, errNotGoPro)
}

func readAttr(dir, attr string) (string, error) {
	b, err := os.ReadFile(filepath.Join(dir, attr))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

// WaitIPv4 polls the interface until it has an IPv4 address and returns it
// with its prefix. The camera runs a DHCP server on the link, so the address
// appears some seconds after the interface does.
func WaitIPv4(ctx context.Context, name string, poll time.Duration) (netip.Prefix, error) {
	t := time.NewTicker(poll)
	defer t.Stop()
	for {
		p, err := ipv4(name)
		if err == nil {
			return p, nil
		}
		select {
		case <-ctx.Done():
			return netip.Prefix{}, fmt.Errorf("wait for IPv4 address on %s: %w (last error: %v)", name, ctx.Err(), err)
		case <-t.C:
		}
	}
}

func ipv4(name string) (netip.Prefix, error) {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return netip.Prefix{}, err
	}
	addrs, err := iface.Addrs()
	if err != nil {
		return netip.Prefix{}, err
	}
	for _, a := range addrs {
		ipnet, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		addr, ok := netip.AddrFromSlice(ipnet.IP)
		if !ok {
			continue
		}
		addr = addr.Unmap()
		if !addr.Is4() || addr.IsLinkLocalUnicast() {
			continue
		}
		ones, _ := ipnet.Mask.Size()
		return netip.PrefixFrom(addr, ones), nil
	}
	return netip.Prefix{}, fmt.Errorf("no IPv4 address on %s", name)
}
