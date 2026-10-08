package download

import (
	"math/bits"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
)

// The Windows weak host model lets a socket bound to an adapter's address
// still leave through another adapter when the routing table says so. The
// unicast interface option pins the outgoing interface, which the local
// address alone does not.
const (
	ipUnicastIf   = 31
	ipv6UnicastIf = 31
)

func unicastControl(index uint32) func(network, address string, c syscall.RawConn) error {
	if index == 0 {
		return nil
	}
	return func(network, address string, c syscall.RawConn) error {
		var setErr error
		if err := c.Control(func(fd uintptr) {
			h := windows.Handle(fd)
			if strings.HasSuffix(network, "6") {
				setErr = windows.SetsockoptInt(h, windows.IPPROTO_IPV6, ipv6UnicastIf, int(index))
				return
			}
			setErr = windows.SetsockoptInt(h, windows.IPPROTO_IP, ipUnicastIf, int(bits.ReverseBytes32(index)))
		}); err != nil {
			return err
		}
		return setErr
	}
}
