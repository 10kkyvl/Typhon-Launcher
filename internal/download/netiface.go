package download

import (
	"fmt"
	"log/slog"
	"math"
	"net"
	"net/netip"
	"slices"
	"strings"
	"unicode"
)

type ifaceInfo struct {
	Name        string
	Index       uint32
	Up          bool
	Loopback    bool
	Addrs       []netip.Addr
	Description string
	Tunnel      bool
}

type adapterDetail struct {
	Description string
	Tunnel      bool
}

func systemInterfaces() ([]ifaceInfo, error) {
	list, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	out := make([]ifaceInfo, 0, len(list))
	for _, ifc := range list {
		addrs, err := ifc.Addrs()
		if err != nil {
			return nil, fmt.Errorf("addresses of %q: %w", ifc.Name, err)
		}
		info := ifaceInfo{
			Name:     ifc.Name,
			Up:       ifc.Flags&net.FlagUp != 0,
			Loopback: ifc.Flags&net.FlagLoopback != 0,
		}
		if ifc.Index > 0 && int64(ifc.Index) <= math.MaxUint32 {
			info.Index = uint32(ifc.Index)
		}
		for _, a := range addrs {
			ipnet, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip, ok := netip.AddrFromSlice(ipnet.IP)
			if !ok {
				continue
			}
			info.Addrs = append(info.Addrs, ip.Unmap())
		}
		if info.Index != 0 {
			// The description only feeds the "looks like a VPN" label, and an
			// adapter can vanish between the two calls; nothing that decides
			// where traffic goes reads it.
			if d, err := adapterDetailOf(info.Index); err != nil {
				slog.Debug("adapter details", "adapter", info.Name, "error", err)
			} else {
				info.Description = d.Description
				info.Tunnel = d.Tunnel
			}
		}
		out = append(out, info)
	}
	return out, nil
}

// usableAddrs keeps the addresses a socket can be bound to and reached from
// outside: a link-local or loopback address never carries torrent traffic.
func usableAddrs(addrs []netip.Addr) (v4, v6 []netip.Addr) {
	for _, a := range addrs {
		switch {
		case !a.IsValid() || a.Zone() != "" || a.IsUnspecified() || a.IsLoopback() || a.IsMulticast() || a.IsLinkLocalUnicast():
		case a.Is4():
			v4 = append(v4, a)
		case a.Is6():
			v6 = append(v6, a)
		}
	}
	return v4, v6
}

func findInterface(list []ifaceInfo, name string) (ifaceInfo, bool) {
	for _, ifc := range list {
		if strings.EqualFold(ifc.Name, name) {
			return ifc, true
		}
	}
	return ifaceInfo{}, false
}

var vpnWords = []string{
	"wireguard", "wintun", "openvpn", "nordlynx", "nordvpn", "protonvpn", "proton", "tailscale", "zerotier",
	"mullvad", "expressvpn", "surfshark", "windscribe", "cyberghost", "privateinternetaccess", "hamachi",
	"softether", "anyconnect", "forticlient", "globalprotect", "cloudflare warp", "outline", "vpn", "ipsec",
	"l2tp", "pptp", "sstp", "ikev2",
}

var vpnShortWords = []string{"tap", "tun", "utun", "wg", "ppp"}

func looksLikeVPN(name, description string, tunnel bool) bool {
	if tunnel {
		return true
	}
	for _, text := range []string{name, description} {
		lower := strings.ToLower(text)
		for _, w := range vpnWords {
			if strings.Contains(lower, w) {
				return true
			}
		}
		for _, token := range strings.FieldsFunc(lower, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
			for _, w := range vpnShortWords {
				rest, ok := strings.CutPrefix(token, w)
				if ok && strings.Trim(rest, "0123456789") == "" {
					return true
				}
			}
		}
	}
	return false
}

func netInterfaceOf(ifc ifaceInfo) NetInterface {
	out := NetInterface{
		Name:        ifc.Name,
		Description: ifc.Description,
		Addresses:   []string{},
		Up:          ifc.Up,
		VPNLike:     looksLikeVPN(ifc.Name, ifc.Description, ifc.Tunnel),
	}
	for _, a := range ifc.Addrs {
		if a.IsLinkLocalUnicast() || a.IsMulticast() || a.IsUnspecified() {
			continue
		}
		out.Addresses = append(out.Addresses, a.String())
	}
	return out
}

func netInterfaces(list []ifaceInfo) []NetInterface {
	out := make([]NetInterface, 0, len(list))
	for _, ifc := range list {
		if ifc.Loopback {
			continue
		}
		out = append(out, netInterfaceOf(ifc))
	}
	slices.SortStableFunc(out, func(a, b NetInterface) int {
		if a.VPNLike != b.VPNLike {
			if a.VPNLike {
				return -1
			}
			return 1
		}
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	return out
}
