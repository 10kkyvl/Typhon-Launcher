package download

import (
	"errors"
	"io"
	"log/slog"
	"net"
	"net/url"
	"strconv"
	"testing"
)

// closeQuietly closes something a test opened. A socket the other side has
// dropped already says nothing about the code under test, so the failure is
// logged and not raised.
func closeQuietly(c io.Closer) {
	if err := c.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
		slog.Debug("close test resource", "error", err)
	}
}

// logged takes the result of a write or a copy that a fake peer makes on a
// connection the client under test may have closed already.
func logged[T any](_ T, err error) {
	if err != nil {
		slog.Debug("fake peer i/o", "error", err)
	}
}

func hostPort(t testing.TB, addr string) (string, int) {
	t.Helper()
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("split %q: %v", addr, err)
	}
	n, err := strconv.Atoi(port)
	if err != nil {
		t.Fatalf("port of %q: %v", addr, err)
	}
	return host, n
}

func tcpAddrOf(t testing.TB, a net.Addr) *net.TCPAddr {
	t.Helper()
	tcp, ok := a.(*net.TCPAddr)
	if !ok {
		t.Fatalf("%v is a %T, want *net.TCPAddr", a, a)
	}
	return tcp
}

func udpAddrOf(t testing.TB, a net.Addr) *net.UDPAddr {
	t.Helper()
	udp, ok := a.(*net.UDPAddr)
	if !ok {
		t.Fatalf("%v is a %T, want *net.UDPAddr", a, a)
	}
	return udp
}

func itoa(n int) string { return strconv.Itoa(n) }

func mustURL(t testing.TB, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse %q: %v", raw, err)
	}
	return u
}
