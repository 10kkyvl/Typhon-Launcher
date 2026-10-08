//go:build !windows

package download

import "net/netip"

func adapterDetailOf(uint32) (adapterDetail, error) { return adapterDetail{}, nil }

// Without a way to read an adapter's name servers, names are not resolved in
// interface mode on this system: only address literals work.
func adapterDNS(ifaceInfo) ([]netip.Addr, error) { return nil, nil }

// A socket bound to an address is not held to that adapter on these systems
// (weak host model), and the library's own sockets take no interface option,
// so the mode is refused instead of run with a leak.
func systemHostCheck(ifaceInfo, bool, bool) error { return errNetIfaceUnsupport }
