package download

import (
	"context"
	"net"
	"syscall"
	"testing"

	"golang.org/x/sys/windows"
)

func loopbackIndex(t *testing.T) uint32 {
	t.Helper()
	list, err := systemInterfaces()
	if err != nil {
		t.Fatalf("systemInterfaces: %v", err)
	}
	for _, ifc := range list {
		if ifc.Loopback && ifc.Index != 0 {
			return ifc.Index
		}
	}
	t.Fatal("no loopback adapter")
	return 0
}

// The routing table cannot be asked from a test, but the socket can be asked
// what it was told. Windows takes the index in network order and hands it back
// in host order; an index it does not know is refused outright, which is what
// keeps a stale adapter from being bound silently.
func TestUnicastControlSetsTheSocketOption(t *testing.T) {
	index := loopbackIndex(t)
	var got int
	var readErr error
	read := func(network, address string, c syscall.RawConn) error {
		if err := unicastControl(index)(network, address, c); err != nil {
			return err
		}
		if err := c.Control(func(fd uintptr) {
			got, readErr = windows.GetsockoptInt(windows.Handle(fd), windows.IPPROTO_IP, ipUnicastIf)
		}); err != nil {
			return err
		}
		return readErr
	}
	lc := net.ListenConfig{Control: read}
	pc, err := lc.ListenPacket(context.Background(), "udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("ListenPacket with the unicast hook: %v", err)
	}
	if err := pc.Close(); err != nil {
		t.Fatal(err)
	}
	if int64(got) != int64(index) {
		t.Fatalf("IP_UNICAST_IF reads back %d, want the adapter index %d", got, index)
	}
}

func TestUnicastControlRefusesAnUnknownAdapter(t *testing.T) {
	lc := net.ListenConfig{Control: unicastControl(65000)}
	pc, err := lc.ListenPacket(context.Background(), "udp4", "127.0.0.1:0")
	if err == nil {
		closeQuietly(pc) // the listen was meant to fail
		t.Fatal("a socket pinned to an adapter that does not exist was created")
	}
}

func TestUnicastControlWithoutIndexIsNoHook(t *testing.T) {
	if unicastControl(0) != nil {
		t.Fatal("index 0 means no adapter is known: no socket option may be set")
	}
}
