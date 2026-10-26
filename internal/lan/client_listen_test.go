package lan

import (
	"errors"
	"fmt"
	"net"
	"testing"

	"typhon/internal/download/listenport"
	"typhon/internal/settings"

	"github.com/anacrolix/torrent"
	tstorage "github.com/anacrolix/torrent/storage"
)

var errStorageBroken = errors.New("storage is broken")

func forbiddenPortErr() error {
	return fmt.Errorf("subsequent listen: %w", &net.OpError{Op: "listen", Net: "udp4", Err: errors.New("bind: An attempt was made to access a socket in a way forbidden by its access permissions.")})
}

type countingStorage struct {
	tstorage.ClientImplCloser
	closed *int
}

func (c countingStorage) Close() error {
	*c.closed++
	return c.ClientImplCloser.Close()
}

func stubOpenTorrentClient(t *testing.T, stub func(tc *torrent.ClientConfig) (*torrent.Client, error)) {
	t.Helper()
	orig := openTorrentClient
	t.Cleanup(func() { openTorrentClient = orig })
	openTorrentClient = stub
}

// Port 0 is no stand-in for a free port: Windows hands out the next number in
// a row, which sits in the range Hyper-V or Docker excluded from UDP as surely
// as the one newLANClient just retried from, so the real client is opened on
// random ports of the dynamic range until one listens.
func openForReal(tc *torrent.ClientConfig) (*torrent.Client, error) {
	var err error
	for attempt := 0; attempt <= listenport.Attempts; attempt++ {
		if tc.ListenPort, err = listenport.Random(); err != nil {
			return nil, err
		}
		var cl *torrent.Client
		if cl, err = torrent.NewClient(tc); err == nil {
			return cl, nil
		}
		if !listenport.IsListenError(err) {
			return nil, err
		}
	}
	return nil, err
}

func TestNewLANClientKeepsTryingRandomPorts(t *testing.T) {
	cases := []struct {
		name      string
		failures  int
		failWith  func() error
		wantErr   bool
		wantTries int
	}{
		{"preferred port free", 0, forbiddenPortErr, false, 1},
		{"preferred port taken", 1, forbiddenPortErr, false, 2},
		{"several random ports taken", 4, forbiddenPortErr, false, 5},
		{"every port taken", listenport.Attempts + 1, forbiddenPortErr, true, listenport.Attempts + 1},
		{"not a port problem", 1, func() error { return errStorageBroken }, true, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var ports []int
			closed := 0
			stubOpenTorrentClient(t, func(tc *torrent.ClientConfig) (*torrent.Client, error) {
				ports = append(ports, tc.ListenPort)
				closer, ok := tc.DefaultStorage.(tstorage.ClientImplCloser)
				if !ok {
					t.Fatalf("default storage %T cannot be closed", tc.DefaultStorage)
				}
				tc.DefaultStorage = countingStorage{ClientImplCloser: closer, closed: &closed}
				if len(ports) <= c.failures {
					return nil, c.failWith()
				}
				return openForReal(tc)
			})

			cl, err := newLANClient(settings.Defaults(), t.TempDir(), listenPort)
			if c.wantErr {
				if err == nil {
					cl.close()
					t.Fatalf("newLANClient succeeded after ports %v, want an error", ports)
				}
				if closed != len(ports) {
					t.Fatalf("closed %d default storages after %d attempts, want every failed one closed", closed, len(ports))
				}
			} else {
				if err != nil {
					t.Fatalf("newLANClient after ports %v: %v", ports, err)
				}
				failedClosed := closed
				cl.close()
				if failedClosed != c.failures {
					t.Fatalf("closed %d default storages after %d failures, want one per failed attempt", failedClosed, c.failures)
				}
			}
			if len(ports) != c.wantTries {
				t.Fatalf("ports tried = %v, want %d attempts", ports, c.wantTries)
			}
			if ports[0] != listenPort {
				t.Fatalf("ports tried = %v, want the preferred one first", ports)
			}
			for _, p := range ports[1:] {
				if p < listenport.First || p > 65535 {
					t.Fatalf("ports tried = %v, want every retry on a port of the dynamic range", ports)
				}
			}
		})
	}
}

func TestNewLANClientErrorWrapsTheLastListenError(t *testing.T) {
	lastErr := errors.New("bind: the last port is taken")
	attempts := 0
	stubOpenTorrentClient(t, func(*torrent.ClientConfig) (*torrent.Client, error) {
		attempts++
		if attempts == listenport.Attempts+1 {
			return nil, &net.OpError{Op: "listen", Net: "udp4", Err: lastErr}
		}
		return nil, forbiddenPortErr()
	})

	cl, err := newLANClient(settings.Defaults(), t.TempDir(), listenPort)
	if err == nil {
		cl.close()
		t.Fatal("newLANClient succeeded with every port refused")
	}
	if !errors.Is(err, lastErr) {
		t.Fatalf("error %v does not wrap the error of the last attempt", err)
	}
}

func TestNewLANClientReturnsANonListenErrorUnchanged(t *testing.T) {
	attempts := 0
	stubOpenTorrentClient(t, func(*torrent.ClientConfig) (*torrent.Client, error) {
		attempts++
		return nil, errStorageBroken
	})

	cl, err := newLANClient(settings.Defaults(), t.TempDir(), listenPort)
	if err == nil {
		cl.close()
		t.Fatal("newLANClient succeeded with a broken client")
	}
	if !errors.Is(err, errStorageBroken) {
		t.Fatalf("error %v does not wrap the cause", err)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1: a non-listen error is not retried", attempts)
	}
}

// Windows hands out the next free port after the previous one, and Hyper-V or
// Docker keep hundreds of them out of UDP. Retrying on port 0 stayed inside
// that range for every attempt, so LAN sharing never started.
func TestNewLANClientEscapesAnExcludedPortRange(t *testing.T) {
	const first, last = 63150, 63549
	next := 63200
	var ports []int
	stubOpenTorrentClient(t, func(tc *torrent.ClientConfig) (*torrent.Client, error) {
		port := tc.ListenPort
		if port == 0 {
			port = next
			next++
		}
		ports = append(ports, port)
		if port == listenPort || (port >= first && port <= last) {
			return nil, forbiddenPortErr()
		}
		return openForReal(tc)
	})

	cl, err := newLANClient(settings.Defaults(), t.TempDir(), listenPort)
	if err != nil {
		t.Fatalf("newLANClient after ports %v: %v", ports, err)
	}
	cl.close()
	if len(ports) < 2 || ports[0] != listenPort {
		t.Fatalf("ports tried = %v, want the preferred one first and a retry after it", ports)
	}
	for _, p := range ports[1:] {
		if p < listenport.First || p > 65535 {
			t.Fatalf("ports tried = %v, want every retry on a port of the dynamic range", ports)
		}
	}
}
