package usbnet

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// fakeSysfs builds a minimal sysfs tree with a PCI NIC, a GoPro over USB,
// another USB network adapter and a virtual bridge.
func fakeSysfs(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	mkdir := func(p string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Join(root, p), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(p, s string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, p), []byte(s+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	link := func(target, name string) {
		t.Helper()
		if err := os.Symlink(filepath.Join(root, target), filepath.Join(root, name)); err != nil {
			t.Fatal(err)
		}
	}
	mkdir("class/net")

	// PCI NIC: no idVendor in any ancestor below /sys/devices.
	mkdir("devices/pci0000:00/0000:00:1f.6/net/eno1")
	link("devices/pci0000:00/0000:00:1f.6", "devices/pci0000:00/0000:00:1f.6/net/eno1/device")
	link("devices/pci0000:00/0000:00:1f.6/net/eno1", "class/net/eno1")

	// GoPro: the net device hangs off USB interface 3-1:1.0 of device 3-1.
	usb := "devices/pci0000:00/0000:00:14.0/usb3/3-1"
	mkdir(usb + "/3-1:1.0/net/enxd6ab2c3d4e5f")
	write(usb+"/idVendor", "2672")
	write(usb+"/product", "HERO13 Black")
	write(usb+"/serial", "C3531325012345")
	link(usb+"/3-1:1.0", usb+"/3-1:1.0/net/enxd6ab2c3d4e5f/device")
	link(usb+"/3-1:1.0/net/enxd6ab2c3d4e5f", "class/net/enxd6ab2c3d4e5f")

	// Another USB Ethernet adapter.
	other := "devices/pci0000:00/0000:00:14.0/usb3/3-2"
	mkdir(other + "/3-2:1.0/net/enx001122334455")
	write(other+"/idVendor", "0bda")
	link(other+"/3-2:1.0", other+"/3-2:1.0/net/enx001122334455/device")
	link(other+"/3-2:1.0/net/enx001122334455", "class/net/enx001122334455")

	// Virtual bridge: no device link at all.
	mkdir("devices/virtual/net/docker0")
	link("devices/virtual/net/docker0", "class/net/docker0")
	return root
}

func TestFind(t *testing.T) {
	root := fakeSysfs(t)
	got, err := Find(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("Find returned %d interfaces, want 1: %+v", len(got), got)
	}
	g := got[0]
	if g.Name != "enxd6ab2c3d4e5f" || g.Product != "HERO13 Black" || g.Serial != "C3531325012345" {
		t.Errorf("Find = %+v", g)
	}
}

func TestLookup(t *testing.T) {
	root := fakeSysfs(t)
	if _, err := Lookup(root, "enxd6ab2c3d4e5f"); err != nil {
		t.Errorf("Lookup(GoPro): %v", err)
	}
	for _, name := range []string{"eno1", "enx001122334455", "docker0"} {
		if _, err := Lookup(root, name); !errors.Is(err, errNotGoPro) {
			t.Errorf("Lookup(%s) = %v, want errNotGoPro", name, err)
		}
	}
	if _, err := Lookup(root, "wlan9"); err == nil || errors.Is(err, errNotGoPro) {
		t.Errorf("Lookup(missing) = %v, want a not-found error", err)
	}
}

func TestValidateName(t *testing.T) {
	for _, ok := range []string{"eth0", "enxd6ab2c3d4e5f", "usb0", "gopro-1"} {
		if err := ValidateName(ok); err != nil {
			t.Errorf("ValidateName(%q) = %v", ok, err)
		}
	}
	for _, bad := range []string{"", ".", "..", "../../etc", "a/b", "eth 0", "eth0:1", "sixteen-chars-xx", "eth\n0", "ethé"} {
		if err := ValidateName(bad); err == nil {
			t.Errorf("ValidateName(%q) = nil, want error", bad)
		}
	}
}
