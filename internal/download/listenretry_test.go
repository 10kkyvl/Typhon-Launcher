package download

import (
	"errors"
	"fmt"
	"net"
	"testing"

	"typhon/internal/download/listenport"
	"typhon/internal/settings"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/storage"
)

func busyPortErr() error {
	return &net.OpError{Op: "listen", Net: "udp6", Err: errors.New("bind: Only one usage of each socket address is normally permitted.")}
}

func forbiddenPortErr() error {
	return fmt.Errorf("subsequent listen: %w", &net.OpError{Op: "listen", Net: "udp4", Err: errors.New("bind: An attempt was made to access a socket in a way forbidden by its access permissions.")})
}

func assertRetriesOnRandomPorts(t *testing.T, ports []int) {
	t.Helper()
	if len(ports) == 0 || ports[0] != listenPort {
		t.Fatalf("ports tried = %v, want the fixed one first", ports)
	}
	for _, p := range ports[1:] {
		if p < listenport.First || p > 65535 {
			t.Fatalf("ports tried = %v, want every retry on a port of the dynamic range", ports)
		}
	}
}

func TestNewClientKeepsTryingRandomPorts(t *testing.T) {
	cases := []struct {
		name      string
		failures  int
		failWith  func() error
		wantErr   bool
		wantTries int
	}{
		{"fixed port free", 0, busyPortErr, false, 1},
		{"one random port taken", 2, busyPortErr, false, 3},
		{"several random ports taken", 4, busyPortErr, false, 5},
		{"every port taken", listenport.Attempts + 1, busyPortErr, true, listenport.Attempts + 1},
		{"not a port problem", 1, func() error { return errors.New("storage is broken") }, true, 1},
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
				return openOnRandomPort(orig, tc)
			}

			cl, err := newClient(t.Context(), settings.Defaults(), t.TempDir(), storage.NewMapPieceCompletion(), netPlan{mode: settings.NetworkDirect})
			if c.wantErr {
				if err == nil {
					cl.close()
					t.Fatalf("newClient succeeded after ports %v, want an error", ports)
				}
			} else {
				if err != nil {
					t.Fatalf("newClient after ports %v: %v", ports, err)
				}
				cl.close()
			}
			if len(ports) != c.wantTries {
				t.Fatalf("ports tried = %v, want %d attempts", ports, c.wantTries)
			}
			assertRetriesOnRandomPorts(t, ports)
		})
	}
}

// Windows hands out the next free port after the previous one, and Hyper-V or
// Docker keep hundreds of them out of UDP. Retrying on port 0 stayed inside
// that range for every attempt, which left the launcher without a torrent
// client.
func TestNewClientEscapesAnExcludedPortRange(t *testing.T) {
	const first, last = 63150, 63549
	next := 63200
	var ports []int
	orig := openTorrentClient
	t.Cleanup(func() { openTorrentClient = orig })
	openTorrentClient = func(tc *torrent.ClientConfig) (*torrent.Client, error) {
		port := tc.ListenPort
		if port == 0 {
			port = next
			next++
		}
		ports = append(ports, port)
		if port == listenPort || (port >= first && port <= last) {
			return nil, forbiddenPortErr()
		}
		return openOnRandomPort(orig, tc)
	}

	cl, err := newClient(t.Context(), settings.Defaults(), t.TempDir(), storage.NewMapPieceCompletion(), netPlan{mode: settings.NetworkDirect})
	if err != nil {
		t.Fatalf("newClient after ports %v: %v", ports, err)
	}
	cl.close()
	assertRetriesOnRandomPorts(t, ports)
}

// The clients the tests stand in for newClient with must ride out a port that
// Windows refuses for UDP, or the test that owns the client never gets one: the
// teardown tests of the net rig hung on it.
func TestStandInClientsKeepTryingRandomPorts(t *testing.T) {
	builders := []struct {
		name string
		open func(t *testing.T) error
	}{
		{"offline client", func(t *testing.T) error {
			offlineClient(t)
			return nil
		}},
		{"net rig build", func(t *testing.T) error {
			c, err := (&buildLog{}).build(t.Context(), settings.Defaults(), t.TempDir(), storage.NewMapPieceCompletion(), netPlan{mode: settings.NetworkDirect})
			if err != nil {
				return err
			}
			t.Cleanup(c.close)
			return nil
		}},
	}
	for _, b := range builders {
		t.Run(b.name, func(t *testing.T) {
			attempts := 0
			orig := openTorrentClient
			t.Cleanup(func() { openTorrentClient = orig })
			openTorrentClient = func(tc *torrent.ClientConfig) (*torrent.Client, error) {
				attempts++
				if attempts <= 3 {
					return nil, forbiddenPortErr()
				}
				return openOnRandomPort(orig, tc)
			}

			if err := b.open(t); err != nil {
				t.Fatalf("client after %d attempts: %v", attempts, err)
			}
			if attempts != 4 {
				t.Fatalf("client opened on attempt %d, want the fourth after three refused ports", attempts)
			}
		})
	}
}

// The stand-ins open a real client on a port of their own: the fixed port may
// be held by the launcher on this machine, and port 0 lands in the range Windows
// excluded from UDP as surely as the sequential port it replaces, which is what
// made TestHaltedClientRefusesEveryAnnounceChannel fail once in three runs.
func TestOpenOnRandomPortRidesOutRefusedPorts(t *testing.T) {
	cases := []struct {
		name      string
		failures  int
		failWith  func() error
		wantErr   bool
		wantTries int
	}{
		{"first port free", 0, forbiddenPortErr, false, 1},
		{"three ports refused", 3, forbiddenPortErr, false, 4},
		{"every port refused", listenport.Attempts + 1, forbiddenPortErr, true, listenport.Attempts + 1},
		{"not a port problem", 1, func() error { return errors.New("storage is broken") }, true, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var ports []int
			open := func(tc *torrent.ClientConfig) (*torrent.Client, error) {
				ports = append(ports, tc.ListenPort)
				if len(ports) <= c.failures {
					return nil, c.failWith()
				}
				return new(torrent.Client), nil
			}
			tc := clientConfig(settings.Defaults(), t.TempDir(), listenPort, nonClosingCompletion{storage.NewMapPieceCompletion()})

			cl, err := openOnRandomPort(open, tc)
			if c.wantErr != (err != nil) {
				t.Fatalf("openOnRandomPort after ports %v: err = %v, want error %v", ports, err, c.wantErr)
			}
			if !c.wantErr && cl == nil {
				t.Fatal("no error and no client")
			}
			if len(ports) != c.wantTries {
				t.Fatalf("ports tried = %v, want %d attempts", ports, c.wantTries)
			}
			for _, p := range ports {
				if p < listenport.First || p > 65535 {
					t.Fatalf("ports tried = %v, want every attempt on a port of the dynamic range, never 0 or the fixed one", ports)
				}
			}
		})
	}
}

func TestOpenTestClientGivesUp(t *testing.T) {
	cases := []struct {
		name         string
		fail         func() error
		wantAttempts int
	}{
		{"every port refused", forbiddenPortErr, listenport.Attempts + 1},
		{"not a port problem", func() error { return errors.New("storage is broken") }, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			attempts := 0
			orig := openTorrentClient
			t.Cleanup(func() { openTorrentClient = orig })
			openTorrentClient = func(*torrent.ClientConfig) (*torrent.Client, error) {
				attempts++
				return nil, c.fail()
			}

			cl, _, err := openTestClient(func() (*torrent.ClientConfig, error) {
				return clientConfig(settings.Defaults(), t.TempDir(), 0, nonClosingCompletion{storage.NewMapPieceCompletion()}), nil
			})
			if err == nil {
				cl.Close()
				t.Fatal("a client that never listened was returned")
			}
			if attempts != c.wantAttempts {
				t.Fatalf("attempts = %d, want %d", attempts, c.wantAttempts)
			}
		})
	}
}
