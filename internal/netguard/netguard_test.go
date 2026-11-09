package netguard

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"testing"
)

func TestBlockedAddrRefusesLocalRangesAndAllowsPublicOnes(t *testing.T) {
	blocked := []string{"127.0.0.1", "::1", "10.1.2.3", "172.16.0.9", "192.168.1.1", "169.254.10.10", "100.64.0.1", "224.0.0.1", "0.0.0.0", "::", "fe80::1", "::ffff:127.0.0.1", "::ffff:10.0.0.1"}
	for _, raw := range blocked {
		if !BlockedAddr(netip.MustParseAddr(raw)) {
			t.Errorf("%s was allowed", raw)
		}
	}
	allowed := []string{"93.184.216.34", "8.8.8.8", "2606:4700:4700::1111", "::ffff:93.184.216.34"}
	for _, raw := range allowed {
		if BlockedAddr(netip.MustParseAddr(raw)) {
			t.Errorf("%s was blocked", raw)
		}
	}
	if !BlockedName("localhost") || !BlockedName("api.localhost.") || !BlockedName(" ") || BlockedName("example.com") {
		t.Fatal("BlockedName does not match the expected names")
	}
}

type dialRecorder struct{ dialled []string }

func (d *dialRecorder) dial(_ context.Context, _, address string) (net.Conn, error) {
	d.dialled = append(d.dialled, address)
	return nil, errors.New("recorded, not connected")
}

func TestDialRefusesPrivateDestinationsBeforeDialling(t *testing.T) {
	for _, address := range []string{"127.0.0.1:80", "[::1]:6969", "10.0.0.5:8080", "localhost:80"} {
		rec := &dialRecorder{}
		_, err := Dial(context.Background(), rec.dial, nil, "tcp", address)
		if !errors.Is(err, ErrBlocked) {
			t.Fatalf("%s: err = %v, want %v", address, err, ErrBlocked)
		}
		if len(rec.dialled) != 0 {
			t.Fatalf("%s: dialled %v although blocked", address, rec.dialled)
		}
	}
}

func TestDialUsesTheCheckedAddressAndSkipsBlockedResolutions(t *testing.T) {
	resolve := func(_ context.Context, host string) ([]netip.Addr, error) {
		if host != "tracker.example" {
			t.Fatalf("unexpected host %q", host)
		}
		// A rebinding-style answer: one private, one public address.
		return []netip.Addr{netip.MustParseAddr("192.168.0.10"), netip.MustParseAddr("93.184.216.34")}, nil
	}
	rec := &dialRecorder{}
	_, err := Dial(context.Background(), rec.dial, resolve, "tcp", "tracker.example:80")
	if err == nil {
		t.Fatal("expected the recorder's error to surface")
	}
	if len(rec.dialled) != 1 || rec.dialled[0] != "93.184.216.34:80" {
		t.Fatalf("dialled %v, want only the public literal", rec.dialled)
	}

	onlyPrivate := func(context.Context, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("172.20.0.1")}, nil
	}
	rec = &dialRecorder{}
	if _, err := Dial(context.Background(), rec.dial, onlyPrivate, "tcp", "evil.example:80"); !errors.Is(err, ErrBlocked) || len(rec.dialled) != 0 {
		t.Fatalf("private-only resolution: err = %v, dialled %v", err, rec.dialled)
	}
}
