package camera

import (
	"fmt"
	"net/netip"
)

// AddressFor derives the camera's address from the host's address on the
// USB link. The camera is a DHCP server at .51 of a /24 of the form
// 172.2X.1YZ.0, where XYZ are the last three digits of its serial number
// (Open GoPro spec). Anything else means the host address did not come from
// the camera, for example NetworkManager's "shared" mode (10.42.0.1).
func AddressFor(host netip.Prefix) (netip.Addr, error) {
	a := host.Addr()
	if !a.Is4() {
		return netip.Addr{}, fmt.Errorf("host address %s is not IPv4", a)
	}
	b := a.As4()
	if host.Bits() != 24 || b[0] != 172 || b[1] < 20 || b[1] > 29 || b[2] < 100 || b[2] > 199 {
		return netip.Addr{}, fmt.Errorf("host address %s is not in a GoPro network (172.2X.1YZ.0/24); "+
			"the interface must get its address from the camera's DHCP server, not from NetworkManager's shared mode or a static setting", host)
	}
	if b[3] == 51 {
		return netip.Addr{}, fmt.Errorf("host address %s is the camera's own address", host)
	}
	b[3] = 51
	return netip.AddrFrom4(b), nil
}
