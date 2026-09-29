package download

import (
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"strings"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// ifTypePPP is what dial-up style VPNs (L2TP, PPTP, SSTP) register as.
const ifTypePPP = 23

const (
	tcpipInterfaces  = `SYSTEM\CurrentControlSet\Services\Tcpip\Parameters\Interfaces\`
	tcpip6Interfaces = `SYSTEM\CurrentControlSet\Services\Tcpip6\Parameters\Interfaces\`
)

func adapterDetailOf(index uint32) (adapterDetail, error) {
	row := windows.MibIfRow2{InterfaceIndex: index}
	if err := windows.GetIfEntry2Ex(windows.MibIfEntryNormal, &row); err != nil {
		return adapterDetail{}, fmt.Errorf("GetIfEntry2Ex(%d): %w", index, err)
	}
	return adapterDetail{
		Description: windows.UTF16ToString(row.Description[:]),
		Tunnel:      row.Type == ifTypePPP,
	}, nil
}

// adapterDNS reads the name servers of one adapter from where Windows keeps
// them: a static list wins over the one DHCP handed out. The system resolver
// is not asked, because it answers with the servers of whichever adapter it
// prefers, and that is exactly the leak this exists to avoid.
func adapterDNS(ifc ifaceInfo) ([]netip.Addr, error) {
	row := windows.MibIfRow2{InterfaceIndex: ifc.Index}
	if err := windows.GetIfEntry2Ex(windows.MibIfEntryNormal, &row); err != nil {
		return nil, fmt.Errorf("GetIfEntry2Ex(%d): %w", ifc.Index, err)
	}
	guid := row.InterfaceGuid.String()
	var out []netip.Addr
	seen := map[netip.Addr]bool{}
	for _, tree := range []string{tcpipInterfaces, tcpip6Interfaces} {
		servers, err := readNameServers(tree + guid)
		if err != nil {
			return nil, err
		}
		for _, s := range servers {
			if !seen[s] {
				seen[s] = true
				out = append(out, s)
			}
		}
	}
	return out, nil
}

func readNameServers(path string) ([]netip.Addr, error) {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, path, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	var servers []netip.Addr
	var readErr error
	for _, name := range []string{"NameServer", "DhcpNameServer"} {
		text, _, err := key.GetStringValue(name)
		if errors.Is(err, registry.ErrNotExist) {
			continue
		}
		if err != nil {
			readErr = fmt.Errorf("read %s value %s: %w", path, name, err)
			break
		}
		if servers = parseNameServers(text); len(servers) > 0 {
			break
		}
	}
	if err := key.Close(); err != nil && readErr == nil {
		readErr = fmt.Errorf("close %s: %w", path, err)
	}
	if readErr != nil {
		return nil, readErr
	}
	return servers, nil
}

func parseNameServers(text string) []netip.Addr {
	var out []netip.Addr
	for _, token := range strings.FieldsFunc(text, func(r rune) bool { return r == ',' || r == ' ' || r == ';' || r == '\t' }) {
		a, err := netip.ParseAddr(token)
		if err != nil || a.Zone() != "" || a.IsUnspecified() {
			// A name server that is not an address cannot be dialled; one
			// fewer only narrows what resolves.
			slog.Debug("skip unusable name server", "value", token)
			continue
		}
		out = append(out, a.Unmap())
	}
	return out
}

// systemHostCheck refuses an adapter Windows would let a socket bound to it
// send through another one. With the strong host model, the default, a packet
// whose source is this adapter's address leaves through this adapter or not
// at all, which is what makes the library's own sockets safe to bind by
// address alone. VPN software can switch WeakHostSend on, and then they are not.
func systemHostCheck(ifc ifaceInfo, v4, v6 bool) error {
	for _, f := range []struct {
		family uint16
		used   bool
	}{{windows.AF_INET, v4}, {windows.AF_INET6, v6}} {
		if !f.used {
			continue
		}
		row := windows.MibIpInterfaceRow{Family: f.family, InterfaceIndex: ifc.Index}
		if err := windows.GetIpInterfaceEntry(&row); err != nil {
			return fmt.Errorf("%w: %s: %w", errNetIfaceHostCheck, ifc.Name, err)
		}
		if weakSend(row) {
			return fmt.Errorf("%w: %s", errNetIfaceWeakHost, ifc.Name)
		}
	}
	return nil
}

func weakSend(row windows.MibIpInterfaceRow) bool { return row.WeakHostSend != 0 }
