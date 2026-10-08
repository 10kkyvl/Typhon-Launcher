package download

import (
	"errors"
	"net"
	"testing"

	"typhon/internal/settings"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/storage"
)

func busyPortErr() error {
	return &net.OpError{Op: "listen", Net: "udp6", Err: errors.New("bind: Only one usage of each socket address is normally permitted.")}
}

func TestNewClientKeepsTryingRandomPorts(t *testing.T) {
	cases := []struct {
		name      string
		failures  int
		failWith  func() error
		wantErr   bool
		wantPorts []int
	}{
		{"fixed port free", 0, busyPortErr, false, []int{listenPort}},
		{"one random port taken", 2, busyPortErr, false, []int{listenPort, 0, 0}},
		{"several random ports taken", 4, busyPortErr, false, []int{listenPort, 0, 0, 0, 0}},
		{"every port taken", randomPortAttempts + 1, busyPortErr, true, nil},
		{"not a port problem", 1, func() error { return errors.New("storage is broken") }, true, []int{listenPort}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var ports []int
			orig := openTorrentClient
			t.Cleanup(func() { openTorrentClient = orig })
			openTorrentClient = func(tc *torrent.ClientConfig) (*torrent.Client, error) {
				ports = append(ports, tc.ListenPort)
				if len(ports) <= c.failures {
					return nil, c.failWith()
				}
				tc.ListenPort = 0
				tc.NoDHT = true
				tc.DisableTrackers = true
				tc.NoDefaultPortForwarding = true
				return orig(tc)
			}

			cl, err := newClient(t.Context(), settings.Defaults(), t.TempDir(), storage.NewMapPieceCompletion(), netPlan{mode: settings.NetworkDirect})
			if c.wantErr {
				if err == nil {
					cl.close()
					t.Fatalf("newClient succeeded after ports %v, want an error", ports)
				}
				if c.wantPorts != nil && !equalInts(ports, c.wantPorts) {
					t.Fatalf("ports tried = %v, want %v", ports, c.wantPorts)
				}
				return
			}
			if err != nil {
				t.Fatalf("newClient after ports %v: %v", ports, err)
			}
			cl.close()
			if !equalInts(ports, c.wantPorts) {
				t.Fatalf("ports tried = %v, want %v", ports, c.wantPorts)
			}
		})
	}
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
