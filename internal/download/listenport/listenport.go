package listenport

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"strings"
)

const (
	First    = 49152
	Attempts = 16
	count    = 65536 - First
)

// Random draws from the dynamic port range. Its size divides 65536, so taking
// two random bytes modulo the size has no bias. A torrent client takes one port
// number for TCP and UDP over IPv4 and IPv6, and port 0 hands out the next
// number in a row on Windows, which sits in the same range Hyper-V or Docker
// excluded from UDP: only a port of our own choosing leaves that range.
func Random() (int, error) {
	var b [2]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 0, fmt.Errorf("pick a torrent port: %w", err)
	}
	return First + int(binary.BigEndian.Uint16(b[:]))%count, nil
}

func IsListenError(err error) bool {
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "bind") ||
		strings.Contains(text, "listen") ||
		strings.Contains(text, "address already in use")
}
