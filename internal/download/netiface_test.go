package download

import (
	"net/netip"
	"testing"
)

func TestLooksLikeVPN(t *testing.T) {
	cases := []struct {
		name, desc string
		tunnel     bool
		want       bool
	}{
		{"Ethernet", "Realtek PCIe GbE Family Controller", false, false},
		{"Wi-Fi", "Intel(R) Wi-Fi 6 AX201 160MHz", false, false},
		{"vEthernet (Default Switch)", "Hyper-V Virtual Ethernet Adapter", false, false},
		{"Bluetooth Network Connection", "Bluetooth Device (Personal Area Network)", false, false},
		{"Подключение по локальной сети", "", false, false},
		{"Ethernet 2", "TAP-Windows Adapter V9", false, true},
		{"OpenVPN Data Channel Offload", "", false, true},
		{"wg0", "WireGuard Tunnel", false, true},
		{"NordLynx", "NordLynx Tunnel", false, true},
		{"ProtonVPN", "", false, true},
		{"Tailscale", "Tailscale Tunnel", false, true},
		{"Ethernet 3", "Wintun Userspace Tunnel", false, true},
		{"utun3", "", false, true},
		{"tun0", "", false, true},
		{"tap0", "", false, true},
		{"ppp0", "", false, true},
		{"WAN Miniport (L2TP)", "", false, true},
		{"Corporate", "some dialup entry", true, true},
		{"Laptop LAN", "", false, false},
		{"Fortune", "", false, false},
	}
	for _, c := range cases {
		if got := looksLikeVPN(c.name, c.desc, c.tunnel); got != c.want {
			t.Errorf("looksLikeVPN(%q, %q, %v) = %v, want %v", c.name, c.desc, c.tunnel, got, c.want)
		}
	}
}

func TestUsableAddrs(t *testing.T) {
	in := []netip.Addr{
		netip.MustParseAddr("127.0.0.1"),
		netip.MustParseAddr("169.254.10.4"),
		netip.MustParseAddr("0.0.0.0"),
		netip.MustParseAddr("224.0.0.1"),
		netip.MustParseAddr("10.8.0.2"),
		netip.MustParseAddr("192.168.1.5"),
		netip.MustParseAddr("::1"),
		netip.MustParseAddr("fe80::1"),
		netip.MustParseAddr("fe80::1%eth0"),
		netip.MustParseAddr("2001:db8::7"),
		{},
	}
	v4, v6 := usableAddrs(in)
	if len(v4) != 2 || v4[0].String() != "10.8.0.2" || v4[1].String() != "192.168.1.5" {
		t.Fatalf("v4 = %v", v4)
	}
	if len(v6) != 1 || v6[0].String() != "2001:db8::7" {
		t.Fatalf("v6 = %v", v6)
	}
}

func TestFindInterfaceIgnoresCase(t *testing.T) {
	list := []ifaceInfo{{Name: "Ethernet"}, {Name: "WireGuard Tunnel"}}
	if ifc, ok := findInterface(list, "wireguard tunnel"); !ok || ifc.Name != "WireGuard Tunnel" {
		t.Fatalf("findInterface = %+v, %v", ifc, ok)
	}
	if _, ok := findInterface(list, "Wi-Fi"); ok {
		t.Fatal("found an adapter that is not there")
	}
	if _, ok := findInterface(nil, "Ethernet"); ok {
		t.Fatal("found an adapter in an empty list")
	}
}

func TestNetInterfacesListing(t *testing.T) {
	list := []ifaceInfo{
		{Name: "Loopback Pseudo-Interface 1", Loopback: true, Up: true, Addrs: []netip.Addr{netip.MustParseAddr("127.0.0.1")}},
		{Name: "Wi-Fi", Up: true, Addrs: []netip.Addr{netip.MustParseAddr("192.168.1.5"), netip.MustParseAddr("fe80::1")}},
		{Name: "Ethernet", Up: false},
		{Name: "wg0", Description: "WireGuard Tunnel", Up: true, Addrs: []netip.Addr{netip.MustParseAddr("10.8.0.2")}},
	}
	got := netInterfaces(list)
	if len(got) != 3 {
		t.Fatalf("interfaces = %+v, want the loopback left out", got)
	}
	if got[0].Name != "wg0" || !got[0].VPNLike || !got[0].Up || got[0].Description != "WireGuard Tunnel" {
		t.Fatalf("the vpn adapter comes first: %+v", got[0])
	}
	if got[1].Name != "Ethernet" || got[2].Name != "Wi-Fi" {
		t.Fatalf("the rest are sorted by name: %+v", got)
	}
	if len(got[2].Addresses) != 1 || got[2].Addresses[0] != "192.168.1.5" {
		t.Fatalf("link-local addresses are not offered: %+v", got[2].Addresses)
	}
	if got[1].Addresses == nil {
		t.Fatal("an adapter without addresses must list an empty array, not null")
	}
}

func TestSystemInterfacesSeesTheLoopback(t *testing.T) {
	list, err := systemInterfaces()
	if err != nil {
		t.Fatalf("systemInterfaces: %v", err)
	}
	found := false
	for _, ifc := range list {
		if ifc.Loopback {
			found = true
			if ifc.Index == 0 {
				t.Errorf("loopback %q has no index", ifc.Name)
			}
		}
	}
	if !found {
		t.Fatalf("no loopback among %d adapters", len(list))
	}
}
