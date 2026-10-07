package download

import (
	"bufio"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/net/proxy"
)

const (
	dialTimeout      = 30 * time.Second
	handshakeTimeout = 15 * time.Second
	probeTarget      = "127.0.0.1:9"
)

var errDirectBlocked = errors.New("direct connection is not allowed while the proxy is on")

var errBadTarget = errors.New("not an address the proxy may be asked to reach")

// maxTargetLen is the longest host name, a colon and the longest port.
const maxTargetLen = 253 + len(":65535")

// validProxyTarget says whether target may be written into a proxy request.
// Peer addresses come from magnet links, trackers and other peers, and the
// library does not check them; written into a CONNECT line as they are, a
// line break in one would let its sender add headers or a second request.
func validProxyTarget(target string) error {
	if len(target) > maxTargetLen {
		return fmt.Errorf("%w: %d bytes", errBadTarget, len(target))
	}
	host, port, err := net.SplitHostPort(target)
	if err != nil {
		// The text of err carries the address as it is, line breaks included.
		return fmt.Errorf("%w: %q: not host:port", errBadTarget, target)
	}
	if n, err := strconv.ParseUint(port, 10, 16); err != nil || n == 0 {
		return fmt.Errorf("%w: %q: port", errBadTarget, target)
	}
	if !validProxyHost(host) {
		return fmt.Errorf("%w: %q: host", errBadTarget, target)
	}
	return nil
}

// validProxyHost accepts an address without a zone or a plain ASCII DNS name;
// a name outside ASCII has to come as punycode.
func validProxyHost(host string) bool {
	if ip, err := netip.ParseAddr(host); err == nil {
		return ip.Zone() == ""
	}
	name := strings.TrimSuffix(host, ".")
	if name == "" || len(name) > 253 {
		return false
	}
	for _, label := range strings.Split(name, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for i := 0; i < len(label); i++ {
			c := label[i]
			letter := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
			if !letter && (c < '0' || c > '9') && c != '-' && c != '_' {
				return false
			}
		}
	}
	return true
}

type dialFunc func(ctx context.Context, network, addr string) (net.Conn, error)

// netDialer adapts a dial function to the peer dialer the torrent client
// takes; the network is fixed per dialer, as the client expects.
type netDialer struct {
	network string
	dial    dialFunc
}

func (d netDialer) DialerNetwork() string { return d.network }

func (d netDialer) Dial(ctx context.Context, addr string) (net.Conn, error) {
	return d.dial(ctx, d.network, addr)
}

type dialFamily struct {
	network string
	ip      netip.Addr
}

// bindDialer dials from one local address per IP family. net.Dialer only
// honours LocalAddr for the family that address belongs to, so a name that
// resolves to both families has to be dialled once per family.
type bindDialer struct {
	ip4, ip6 netip.Addr
	control  func(network, address string, c syscall.RawConn) error
	ctx      context.Context
	// resolver resolves host names through the adapter; nil means the system
	// one, which the interface mode never leaves in place.
	resolver *net.Resolver
	// dnsPort is 53 unless a test says otherwise.
	dnsPort string
}

func (b *bindDialer) families(network string) ([]dialFamily, error) {
	var out []dialFamily
	switch network {
	case "tcp", "tcp4", "tcp6":
	default:
		return nil, fmt.Errorf("unsupported network %q", network)
	}
	if network != "tcp6" && b.ip4.IsValid() {
		out = append(out, dialFamily{"tcp4", b.ip4})
	}
	if network != "tcp4" && b.ip6.IsValid() {
		out = append(out, dialFamily{"tcp6", b.ip6})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("the bound adapter has no %s address", network)
	}
	return out, nil
}

func (b *bindDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	families, err := b.families(network)
	if err != nil {
		return nil, err
	}
	var errs []error
	for _, f := range families {
		d := net.Dialer{
			LocalAddr: &net.TCPAddr{IP: f.ip.AsSlice()},
			Timeout:   dialTimeout,
			Control:   b.control,
			Resolver:  b.resolver,
		}
		conn, err := d.DialContext(ctx, f.network, addr)
		if err == nil {
			return conn, nil
		}
		errs = append(errs, err)
		if ctx.Err() != nil {
			break
		}
	}
	return nil, errors.Join(errs...)
}

func (b *bindDialer) peerDialers() []netDialer {
	var out []netDialer
	if b.ip4.IsValid() {
		out = append(out, netDialer{network: "tcp4", dial: b.DialContext})
	}
	if b.ip6.IsValid() {
		out = append(out, netDialer{network: "tcp6", dial: b.DialContext})
	}
	return out
}

// listenPacket serves the UDP tracker client. The library panics when this
// hook fails, so a socket that cannot be bound becomes one that carries
// nothing: the announce then fails on write instead of taking the process
// down, and nothing is sent outside the bound adapter.
func (b *bindDialer) listenPacket(network, _ string) (net.PacketConn, error) {
	ip := b.ip4
	switch network {
	case "udp6":
		ip = b.ip6
	case "udp4":
	default:
		if !ip.IsValid() {
			ip = b.ip6
		}
	}
	if !ip.IsValid() {
		return newBlockedPacketConn(), nil
	}
	proto := "udp4"
	if ip.Is6() {
		proto = "udp6"
	}
	lc := net.ListenConfig{Control: b.control}
	pc, err := lc.ListenPacket(b.ctx, proto, net.JoinHostPort(ip.String(), "0"))
	if err != nil {
		slog.Warn("bind udp tracker socket", "error", err)
		return newBlockedPacketConn(), nil
	}
	return pc, nil
}

// blockedPacketConn is a UDP socket that never touches the network.
type blockedPacketConn struct {
	closed chan struct{}
	once   sync.Once
}

func newBlockedPacketConn() *blockedPacketConn {
	return &blockedPacketConn{closed: make(chan struct{})}
}

func (c *blockedPacketConn) ReadFrom([]byte) (int, net.Addr, error) {
	<-c.closed
	return 0, nil, net.ErrClosed
}

func (c *blockedPacketConn) WriteTo([]byte, net.Addr) (int, error) {
	return 0, errDirectBlocked
}

func (c *blockedPacketConn) Close() error {
	c.once.Do(func() { close(c.closed) })
	return nil
}

func (c *blockedPacketConn) LocalAddr() net.Addr              { return &net.UDPAddr{} }
func (c *blockedPacketConn) SetDeadline(time.Time) error      { return nil }
func (c *blockedPacketConn) SetReadDeadline(time.Time) error  { return nil }
func (c *blockedPacketConn) SetWriteDeadline(time.Time) error { return nil }

// proxyOnlyDial lets the HTTP transports of the client reach the proxy and
// nothing else. With a proxy configured they never dial anything but the
// proxy, so any other address means a path the proxy does not cover, and that
// path must fail closed.
func proxyOnlyDial(proxyHost string, proxyPort int) dialFunc {
	wantHost, wantPort := proxyHost, strconv.Itoa(proxyPort)
	d := &net.Dialer{Timeout: dialTimeout}
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil || port != wantPort || !strings.EqualFold(host, wantHost) {
			return nil, fmt.Errorf("%w: %s", errDirectBlocked, addr)
		}
		return d.DialContext(ctx, network, addr)
	}
}

type remoteConn struct {
	net.Conn
	remote net.Addr
}

func (c remoteConn) RemoteAddr() net.Addr { return c.remote }

// withRemote makes a tunnelled connection report the peer as its remote
// address, not the proxy the socket is actually connected to.
func withRemote(c net.Conn, target string) net.Conn {
	ap, err := netip.ParseAddrPort(target)
	if err != nil {
		return c
	}
	return remoteConn{Conn: c, remote: net.TCPAddrFromAddrPort(ap)}
}

func tcpNetwork(network string) error {
	switch network {
	case "tcp", "tcp4", "tcp6":
		return nil
	}
	return fmt.Errorf("unsupported network %q", network)
}

func socks5Dial(p netPlan) (dialFunc, error) {
	var auth *proxy.Auth
	if p.puser != "" {
		auth = &proxy.Auth{User: p.puser, Password: p.ppass}
	}
	d, err := proxy.SOCKS5("tcp", p.proxyAddr(), auth, &net.Dialer{Timeout: dialTimeout})
	if err != nil {
		return nil, fmt.Errorf("socks5 dialer: %w", err)
	}
	cd, ok := d.(proxy.ContextDialer)
	if !ok {
		return nil, errors.New("socks5 dialer does not support contexts")
	}
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		if err := tcpNetwork(network); err != nil {
			return nil, err
		}
		if err := validProxyTarget(addr); err != nil {
			return nil, err
		}
		c, err := cd.DialContext(ctx, "tcp", addr)
		if err != nil {
			// The library reports a cancelled handshake as the read error the
			// cancellation caused, so the context is asked instead.
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, ctxErr
			}
			return nil, classifySOCKSError(err)
		}
		return withRemote(c, addr), nil
	}, nil
}

// classifySOCKSError picks the wrong-credentials case out of the errors of
// golang.org/x/net/proxy, which reports every failure as plain text.
func classifySOCKSError(err error) error {
	if ctxErr := contextError(err); ctxErr != nil {
		return ctxErr
	}
	text := err.Error()
	for _, marker := range []string{
		"authentication failed",
		"no acceptable authentication methods",
		"unsupported authentication method",
		"invalid username/password",
	} {
		if strings.Contains(text, marker) {
			return fmt.Errorf("%w: %w", errProxyAuthFailed, err)
		}
	}
	return err
}

func contextError(err error) error {
	switch {
	case errors.Is(err, context.Canceled):
		return context.Canceled
	case errors.Is(err, context.DeadlineExceeded):
		return context.DeadlineExceeded
	}
	return nil
}

// protocolError marks a failure in the middle of a proxy handshake, such as a
// connection the proxy cut, as the proxy's fault; what already carries a
// reason of its own, and a cancelled context, is left as it is.
func protocolError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, errProxyAuthFailed), errors.Is(err, errProxyFailed), errors.Is(err, errProxyUnreachable):
		return err
	case contextError(err) != nil:
		return err
	}
	return fmt.Errorf("%w: %w", errProxyFailed, err)
}

func basicAuth(user, pass string) string {
	if user == "" {
		return ""
	}
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))
}

type bufferedConn struct {
	net.Conn
	r *bufio.Reader
}

func (c bufferedConn) Read(p []byte) (int, error) { return c.r.Read(p) }

// timed runs a handshake on conn under ctx and a deadline, so a proxy that
// stops answering cannot hold a dial forever.
func timed(ctx context.Context, conn net.Conn, fn func() error) error {
	deadline := time.Now().Add(handshakeTimeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return err
	}
	stop := context.AfterFunc(ctx, func() {
		// Unblocks a read or write that is stuck inside fn.
		if err := conn.SetDeadline(time.Unix(1, 0)); err != nil {
			slog.Warn("interrupt proxy handshake", "error", err)
		}
	})
	err := fn()
	stop()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	if err != nil {
		return err
	}
	return conn.SetDeadline(time.Time{})
}

// connectStatus sends a CONNECT for target and returns the status of the
// answer. The caller decides what a status means.
func connectStatus(conn net.Conn, br *bufio.Reader, target, auth string) (int, string, error) {
	if err := validProxyTarget(target); err != nil {
		return 0, "", err
	}
	var req strings.Builder
	req.WriteString("CONNECT " + target + " HTTP/1.1\r\nHost: " + target + "\r\n")
	if auth != "" {
		req.WriteString("Proxy-Authorization: " + auth + "\r\n")
	}
	req.WriteString("\r\n")
	if _, err := io.WriteString(conn, req.String()); err != nil {
		return 0, "", err
	}
	resp, err := http.ReadResponse(br, &http.Request{Method: http.MethodConnect})
	if err != nil {
		return 0, "", fmt.Errorf("%w: %w", errProxyFailed, err)
	}
	status, text := resp.StatusCode, resp.Status
	if err := resp.Body.Close(); err != nil {
		return 0, "", err
	}
	return status, text, nil
}

func httpConnectDial(p netPlan) dialFunc {
	auth := basicAuth(p.puser, p.ppass)
	d := &net.Dialer{Timeout: dialTimeout}
	return func(ctx context.Context, network, target string) (net.Conn, error) {
		if err := tcpNetwork(network); err != nil {
			return nil, err
		}
		if err := validProxyTarget(target); err != nil {
			return nil, err
		}
		conn, err := d.DialContext(ctx, "tcp", p.proxyAddr())
		if err != nil {
			if ctxErr := contextError(err); ctxErr != nil {
				return nil, ctxErr
			}
			return nil, fmt.Errorf("%w: %w", errProxyUnreachable, err)
		}
		br := bufio.NewReader(conn)
		var status int
		var text string
		err = protocolError(timed(ctx, conn, func() error {
			var herr error
			status, text, herr = connectStatus(conn, br, target, auth)
			return herr
		}))
		if err == nil {
			err = connectResult(status, text)
		}
		if err != nil {
			if cerr := conn.Close(); cerr != nil {
				slog.Warn("close proxy connection", "error", cerr)
			}
			return nil, err
		}
		return withRemote(bufferedConn{Conn: conn, r: br}, target), nil
	}
}

func connectResult(status int, text string) error {
	switch {
	case status >= 200 && status < 300:
		return nil
	case status == http.StatusProxyAuthRequired:
		return fmt.Errorf("%w: %s", errProxyAuthFailed, text)
	default:
		return fmt.Errorf("%w: %s", errProxyFailed, text)
	}
}

// socks5Handshake negotiates the method and the credentials and stops there:
// it proves the proxy speaks SOCKS5 and accepts the login without asking it
// to connect anywhere.
func socks5Handshake(rw io.ReadWriter, user, pass string) error {
	greeting := []byte{5, 1, 0}
	if user != "" {
		greeting = []byte{5, 2, 0, 2}
	}
	if _, err := rw.Write(greeting); err != nil {
		return err
	}
	var reply [2]byte
	if _, err := io.ReadFull(rw, reply[:]); err != nil {
		return err
	}
	if reply[0] != 5 {
		return fmt.Errorf("%w: not a SOCKS5 server", errProxyFailed)
	}
	switch reply[1] {
	case 0:
		return nil
	case 2:
		if user == "" {
			return fmt.Errorf("%w: the proxy asks for a login", errProxyAuthFailed)
		}
		if len(user) > 255 || len(pass) > 255 {
			return fmt.Errorf("%w: login is too long", errProxyAuthFailed)
		}
		msg := append([]byte{1, byte(len(user) & 0xff)}, user...)
		msg = append(msg, byte(len(pass)&0xff))
		msg = append(msg, pass...)
		if _, err := rw.Write(msg); err != nil {
			return err
		}
		if _, err := io.ReadFull(rw, reply[:]); err != nil {
			return err
		}
		if reply[1] != 0 {
			return errProxyAuthFailed
		}
		return nil
	case 0xff:
		return fmt.Errorf("%w: no acceptable authentication method", errProxyAuthFailed)
	default:
		return fmt.Errorf("%w: unexpected method %d", errProxyFailed, reply[1])
	}
}

// testProxy checks that the proxy is reachable and accepts the login.
func testProxy(ctx context.Context, p netPlan) error {
	d := net.Dialer{Timeout: handshakeTimeout}
	conn, err := d.DialContext(ctx, "tcp", p.proxyAddr())
	if err != nil {
		if ctxErr := contextError(err); ctxErr != nil {
			return ctxErr
		}
		return fmt.Errorf("%w: %w", errProxyUnreachable, err)
	}
	defer func() {
		if cerr := conn.Close(); cerr != nil {
			slog.Warn("close proxy probe", "error", cerr)
		}
	}()
	return protocolError(timed(ctx, conn, func() error {
		if p.ptype == "http" {
			// Anything but 407 means the proxy took the login; what it
			// thinks of the probe target is beside the point.
			status, text, err := connectStatus(conn, bufio.NewReader(conn), probeTarget, basicAuth(p.puser, p.ppass))
			if err != nil {
				return err
			}
			if status == http.StatusProxyAuthRequired {
				return fmt.Errorf("%w: %s", errProxyAuthFailed, text)
			}
			return nil
		}
		return socks5Handshake(conn, p.puser, p.ppass)
	}))
}

func probeTCP(ctx context.Context, addr string) error {
	d := net.Dialer{Timeout: handshakeTimeout / 3}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	return conn.Close()
}
