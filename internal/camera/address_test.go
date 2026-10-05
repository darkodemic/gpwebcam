package camera

import (
	"net/netip"
	"testing"
)

func TestAddressFor(t *testing.T) {
	for host, want := range map[string]string{
		"172.28.103.54/24": "172.28.103.51",
		"172.20.161.52/24": "172.20.161.51",
		"172.29.199.53/24": "172.29.199.51",
	} {
		got, err := AddressFor(netip.MustParsePrefix(host))
		if err != nil || got.String() != want {
			t.Errorf("AddressFor(%s) = %v, %v; want %s", host, got, err, want)
		}
	}
	for _, host := range []string{
		"10.42.0.1/24",     // NetworkManager shared mode
		"172.17.0.1/16",    // Docker bridge
		"172.28.103.54/16", // right range, wrong mask
		"172.19.103.54/24", // second octet out of range
		"172.28.99.54/24",  // third octet out of range
		"172.28.103.51/24", // the camera's own address
		"192.168.1.20/24",
	} {
		if got, err := AddressFor(netip.MustParsePrefix(host)); err == nil {
			t.Errorf("AddressFor(%s) = %v, want error", host, got)
		}
	}
}
