package download

import (
	"errors"
	"net/netip"
	"testing"

	"golang.org/x/sys/windows"
)

func TestParseNameServers(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"10.8.0.1", []string{"10.8.0.1"}},
		{"10.8.0.1,10.8.0.2", []string{"10.8.0.1", "10.8.0.2"}},
		{"10.8.0.1 fd00::1;1.1.1.1", []string{"10.8.0.1", "fd00::1", "1.1.1.1"}},
		{"dns.example, 0.0.0.0, fe80::1%eth0, 9.9.9.9", []string{"9.9.9.9"}},
		{"::ffff:10.0.0.1", []string{"10.0.0.1"}},
	}
	for _, c := range cases {
		got := parseNameServers(c.in)
		var want []netip.Addr
		for _, w := range c.want {
			want = append(want, netip.MustParseAddr(w))
		}
		if len(got) != len(want) {
			t.Errorf("parseNameServers(%q) = %v, want %v", c.in, got, want)
			continue
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("parseNameServers(%q) = %v, want %v", c.in, got, want)
			}
		}
	}
}

func TestWeakSendReadsTheRow(t *testing.T) {
	if weakSend(windows.MibIpInterfaceRow{}) {
		t.Fatal("a row with WeakHostSend off is strong")
	}
	if !weakSend(windows.MibIpInterfaceRow{WeakHostSend: 1}) {
		t.Fatal("a row with WeakHostSend on is weak")
	}
}

// The loopback adapter is on every machine; what its WeakHostSend says is up
// to the machine, but the question itself has to be answerable.
func TestSystemHostCheckCanAskTheSystem(t *testing.T) {
	ifc := ifaceInfo{Name: "loopback", Index: loopbackIndex(t)}
	err := systemHostCheck(ifc, true, false)
	if errors.Is(err, errNetIfaceHostCheck) {
		t.Fatalf("the host model of a real adapter could not be read: %v", err)
	}
	bogus := ifaceInfo{Name: "gone", Index: 65000}
	if err := systemHostCheck(bogus, true, false); !errors.Is(err, errNetIfaceHostCheck) {
		t.Fatalf("an adapter that does not exist must fail the check closed, got %v", err)
	}
}

func TestAdapterDNSAndDetailsOfARealAdapter(t *testing.T) {
	idx := loopbackIndex(t)
	if _, err := adapterDNS(ifaceInfo{Name: "loopback", Index: idx}); err != nil {
		t.Fatalf("adapterDNS: %v", err)
	}
	d, err := adapterDetailOf(idx)
	if err != nil || d.Description == "" {
		t.Fatalf("adapterDetailOf = %+v, %v", d, err)
	}
	if _, err := adapterDNS(ifaceInfo{Name: "gone", Index: 65000}); err == nil {
		t.Fatal("name servers of an adapter that does not exist must be an error, not an empty list")
	}
}
