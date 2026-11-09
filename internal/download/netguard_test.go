package download

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"typhon/internal/netguard"
	"typhon/internal/settings"
)

// A torrent's HTTP trackers and webseeds come from the feed. Whatever network
// mode is active, the dialer the engine uses for them must refuse the user's
// own machine and network, while a public destination is dialled as given.
func TestHTTPTrackerDialRefusesPrivateDestinationsInEveryMode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("a request reached the loopback tracker")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	loopback := strings.TrimPrefix(srv.URL, "http://")

	plans := map[string]netPlan{
		"direct":    {mode: settings.NetworkDirect},
		"interface": ifacePlan("127.0.0.1", ""),
	}
	for name, plan := range plans {
		t.Run(name, func(t *testing.T) {
			tc := testConfig(t)
			if _, err := applyNetwork(context.Background(), tc, plan); err != nil {
				t.Fatalf("applyNetwork: %v", err)
			}
			if tc.HTTPDialContext == nil || tc.TrackerDialContext == nil {
				t.Fatal("the HTTP and tracker dialers are left on the library default")
			}
			for _, dial := range []func(context.Context, string, string) (net.Conn, error){tc.HTTPDialContext, tc.TrackerDialContext} {
				conn, err := dial(context.Background(), "tcp", loopback)
				if err == nil {
					if cerr := conn.Close(); cerr != nil {
						t.Errorf("close: %v", cerr)
					}
					t.Fatal("the dialer connected to a loopback tracker")
				}
				if !errors.Is(err, netguard.ErrBlocked) {
					t.Fatalf("dial error = %v, want %v", err, netguard.ErrBlocked)
				}
			}
		})
	}
}

func TestGuardedHTTPDialPassesPublicLiteralsThrough(t *testing.T) {
	var got string
	next := func(_ context.Context, _, address string) (net.Conn, error) {
		got = address
		return nil, errors.New("not connected in the test")
	}
	dial := guardedHTTPDial(next, nil)
	if _, err := dial(context.Background(), "tcp", "93.184.216.34:6969"); err == nil || got != "93.184.216.34:6969" {
		t.Fatalf("public literal: err = %v, dialled %q", err, got)
	}
	got = ""
	if _, err := dial(context.Background(), "tcp", "10.9.8.7:6969"); !errors.Is(err, netguard.ErrBlocked) || got != "" {
		t.Fatalf("private literal: err = %v, dialled %q", err, got)
	}
}

// UDP trackers are announced to through the packet socket the engine asks
// for, after the engine resolved the name itself. In every mode the socket
// must refuse a packet to the user's own machine or network and nothing may
// reach the loopback listener.
func TestUDPTrackerSocketRefusesPrivateDestinationsInEveryMode(t *testing.T) {
	target, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen target: %v", err)
	}
	t.Cleanup(func() {
		if cerr := target.Close(); cerr != nil {
			t.Errorf("close target: %v", cerr)
		}
	})

	plans := map[string]netPlan{
		"direct":    {mode: settings.NetworkDirect},
		"interface": ifacePlan("127.0.0.1", ""),
	}
	for name, plan := range plans {
		t.Run(name, func(t *testing.T) {
			tc := testConfig(t)
			if _, err := applyNetwork(context.Background(), tc, plan); err != nil {
				t.Fatalf("applyNetwork: %v", err)
			}
			if tc.TrackerListenPacket == nil {
				t.Fatal("the UDP tracker socket is left on the library default")
			}
			pc, err := tc.TrackerListenPacket("udp4", "127.0.0.1:0")
			if err != nil {
				t.Fatalf("TrackerListenPacket: %v", err)
			}
			t.Cleanup(func() {
				if cerr := pc.Close(); cerr != nil {
					t.Errorf("close socket: %v", cerr)
				}
			})
			if _, err := pc.WriteTo([]byte("announce"), target.LocalAddr()); !errors.Is(err, netguard.ErrBlocked) {
				t.Fatalf("WriteTo loopback: err = %v, want %v", err, netguard.ErrBlocked)
			}
			if err := target.SetReadDeadline(time.Now().Add(200 * time.Millisecond)); err != nil {
				t.Fatalf("set deadline: %v", err)
			}
			buf := make([]byte, 64)
			if n, from, rerr := target.ReadFrom(buf); rerr == nil {
				t.Fatalf("the loopback tracker received %d bytes from %s", n, from)
			}
		})
	}
}

type packetRecorder struct {
	net.PacketConn
	sent []string
}

func (p *packetRecorder) WriteTo(b []byte, addr net.Addr) (int, error) {
	p.sent = append(p.sent, addr.String())
	return len(b), nil
}

func TestGuardedPacketConnPassesPublicDestinationsThrough(t *testing.T) {
	rec := &packetRecorder{}
	pc := guardedPacketConn{PacketConn: rec}
	public := &net.UDPAddr{IP: net.ParseIP("93.184.216.34"), Port: 6969}
	if n, err := pc.WriteTo([]byte("announce"), public); err != nil || n != len("announce") {
		t.Fatalf("public destination: n = %d, err = %v", n, err)
	}
	private := &net.UDPAddr{IP: net.ParseIP("192.168.1.20"), Port: 6969}
	if _, err := pc.WriteTo([]byte("announce"), private); !errors.Is(err, netguard.ErrBlocked) {
		t.Fatalf("private destination: err = %v, want %v", err, netguard.ErrBlocked)
	}
	if len(rec.sent) != 1 || rec.sent[0] != "93.184.216.34:6969" {
		t.Fatalf("sent %v, want only the public destination", rec.sent)
	}
}
