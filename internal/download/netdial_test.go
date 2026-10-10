package download

import (
	"bufio"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"typhon/internal/settings"
)

type socksServer struct {
	ln   net.Listener
	user string
	pass string
	// reply is the SOCKS reply code sent to CONNECT; zero means success.
	reply byte
	// silent makes the server accept and then say nothing.
	silent   bool
	accepted chan struct{}

	mu      sync.Mutex
	targets []string
}

func startSOCKS(t *testing.T, s *socksServer) *socksServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	s.ln = ln
	s.accepted = make(chan struct{}, 8)
	t.Cleanup(func() {
		if err := ln.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			t.Errorf("close socks listener: %v", err)
		}
	})
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go s.serve(c)
		}
	}()
	return s
}

func (s *socksServer) addr() string { return s.ln.Addr().String() }

func (s *socksServer) seen() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.targets...)
}

func (s *socksServer) serve(c net.Conn) {
	defer closeQuietly(c) // the peer may be gone already
	s.accepted <- struct{}{}
	if s.silent {
		logged(io.Copy(io.Discard, c)) // holds the connection open until the client leaves
		return
	}
	head := make([]byte, 2)
	if _, err := io.ReadFull(c, head); err != nil {
		return
	}
	methods := make([]byte, head[1])
	if _, err := io.ReadFull(c, methods); err != nil {
		return
	}
	offered := func(m byte) bool {
		for _, x := range methods {
			if x == m {
				return true
			}
		}
		return false
	}
	if s.user != "" {
		if !offered(2) {
			logged(c.Write([]byte{5, 0xff})) // the client cannot answer; the close follows
			return
		}
		if _, err := c.Write([]byte{5, 2}); err != nil {
			return
		}
		ver := make([]byte, 2)
		if _, err := io.ReadFull(c, ver); err != nil {
			return
		}
		user := make([]byte, ver[1])
		if _, err := io.ReadFull(c, user); err != nil {
			return
		}
		plen := make([]byte, 1)
		if _, err := io.ReadFull(c, plen); err != nil {
			return
		}
		pass := make([]byte, plen[0])
		if _, err := io.ReadFull(c, pass); err != nil {
			return
		}
		if string(user) != s.user || string(pass) != s.pass {
			logged(c.Write([]byte{1, 1}))
			return
		}
		if _, err := c.Write([]byte{1, 0}); err != nil {
			return
		}
	} else if _, err := c.Write([]byte{5, 0}); err != nil {
		return
	}

	req := make([]byte, 4)
	if _, err := io.ReadFull(c, req); err != nil {
		return
	}
	var host string
	switch req[3] {
	case 1:
		ip := make([]byte, 4)
		if _, err := io.ReadFull(c, ip); err != nil {
			return
		}
		host = net.IP(ip).String()
	case 3:
		n := make([]byte, 1)
		if _, err := io.ReadFull(c, n); err != nil {
			return
		}
		name := make([]byte, n[0])
		if _, err := io.ReadFull(c, name); err != nil {
			return
		}
		host = string(name)
	case 4:
		ip := make([]byte, 16)
		if _, err := io.ReadFull(c, ip); err != nil {
			return
		}
		host = net.IP(ip).String()
	}
	port := make([]byte, 2)
	if _, err := io.ReadFull(c, port); err != nil {
		return
	}
	s.mu.Lock()
	s.targets = append(s.targets, net.JoinHostPort(host, strconv.Itoa(int(port[0])<<8|int(port[1]))))
	s.mu.Unlock()
	if _, err := c.Write([]byte{5, s.reply, 0, 1, 0, 0, 0, 0, 0, 0}); err != nil {
		return
	}
	if s.reply == 0 {
		logged(io.Copy(c, c)) // echoes what the client sends
	}
}

func socksPlan(t *testing.T, s *socksServer, user, pass string) netPlan {
	t.Helper()
	host, port := hostPort(t, s.addr())
	return netPlan{mode: settings.NetworkProxy, ptype: settings.ProxySOCKS5, phost: host, pport: port, puser: user, ppass: pass}
}

func TestSOCKS5Dial(t *testing.T) {
	cases := []struct {
		name         string
		server       *socksServer
		user, pass   string
		target       string
		wantAuthFail bool
		wantErr      bool
		wantErrText  string
	}{
		{name: "no credentials", target: "203.0.113.5:6881"},
		{name: "hostname goes to the proxy unresolved", target: "tracker.example:80"},
		{name: "login accepted", server: &socksServer{user: "u", pass: "secret"}, user: "u", pass: "secret", target: "203.0.113.5:6881"},
		{name: "wrong password", server: &socksServer{user: "u", pass: "secret"}, user: "u", pass: "wrong", target: "203.0.113.5:6881", wantErr: true, wantAuthFail: true},
		{name: "unknown user", server: &socksServer{user: "u", pass: "secret"}, user: "x", pass: "secret", target: "203.0.113.5:6881", wantErr: true, wantAuthFail: true},
		{name: "proxy wants a login and none is set", server: &socksServer{user: "u", pass: "secret"}, target: "203.0.113.5:6881", wantErr: true, wantAuthFail: true},
		{name: "target refuses", server: &socksServer{reply: 5}, target: "203.0.113.5:6881", wantErr: true, wantErrText: "connection refused"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.server == nil {
				c.server = &socksServer{}
			}
			srv := startSOCKS(t, c.server)
			dial, err := socks5Dial(socksPlan(t, srv, c.user, c.pass))
			if err != nil {
				t.Fatalf("socks5Dial: %v", err)
			}
			conn, err := dial(t.Context(), "tcp", c.target)
			if c.wantErr {
				if err == nil {
					closeQuietly(conn) // the dial was meant to fail
					t.Fatal("dial succeeded, want an error")
				}
				if got := errors.Is(err, errProxyAuthFailed); got != c.wantAuthFail {
					t.Fatalf("auth failure = %v for %v, want %v", got, err, c.wantAuthFail)
				}
				if c.wantErrText != "" && !strings.Contains(err.Error(), c.wantErrText) {
					t.Fatalf("error %q does not mention %q", err, c.wantErrText)
				}
				return
			}
			if err != nil {
				t.Fatalf("dial: %v", err)
			}
			t.Cleanup(func() { closeQuietly(conn) }) // the echo side ends with the test
			if got := conn.RemoteAddr().String(); got != c.target && !strings.HasPrefix(c.target, "tracker") {
				t.Fatalf("RemoteAddr = %s, want the peer %s and not the proxy", got, c.target)
			}
			if _, err := conn.Write([]byte("ping")); err != nil {
				t.Fatalf("write: %v", err)
			}
			buf := make([]byte, 4)
			if _, err := io.ReadFull(conn, buf); err != nil || string(buf) != "ping" {
				t.Fatalf("echo = %q, %v", buf, err)
			}
			if seen := srv.seen(); len(seen) != 1 || seen[0] != c.target {
				t.Fatalf("proxy saw %v, want %s", seen, c.target)
			}
		})
	}
}

func TestSOCKS5DialProxyDown(t *testing.T) {
	srv := startSOCKS(t, &socksServer{})
	plan := socksPlan(t, srv, "", "")
	if err := srv.ln.Close(); err != nil {
		t.Fatal(err)
	}
	dial, err := socks5Dial(plan)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := dial(t.Context(), "tcp", "203.0.113.5:6881")
	if err == nil {
		closeQuietly(conn) // the dial was meant to fail
		t.Fatal("dial through a dead proxy succeeded")
	}
	if errors.Is(err, errProxyAuthFailed) {
		t.Fatalf("a dead proxy is not a wrong password: %v", err)
	}
}

func TestSOCKS5DialCancelledWhileTheProxyIsSilent(t *testing.T) {
	srv := startSOCKS(t, &socksServer{silent: true})
	dial, err := socks5Dial(socksPlan(t, srv, "u", "p"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	go func() {
		<-srv.accepted
		cancel()
	}()
	conn, err := dial(ctx, "tcp", "203.0.113.5:6881")
	if err == nil {
		closeQuietly(conn) // the dial was meant to fail
		t.Fatal("dial succeeded against a silent proxy")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func TestSOCKS5DialRefusesOtherNetworks(t *testing.T) {
	dial, err := socks5Dial(netPlan{mode: settings.NetworkProxy, ptype: settings.ProxySOCKS5, phost: "127.0.0.1", pport: 1})
	if err != nil {
		t.Fatal(err)
	}
	if conn, err := dial(t.Context(), "udp", "203.0.113.5:6881"); err == nil {
		closeQuietly(conn) // the dial was meant to fail
		t.Fatal("a udp dial through socks5 must fail")
	}
}

type connectServer struct {
	ln net.Listener
	// status is the CONNECT answer; zero means 200.
	status int
	// after is written right behind a 200.
	after string
	// garbage replaces the whole answer.
	garbage string

	mu       sync.Mutex
	accepted int
	targets  []string
	auths    []string
}

func startConnect(t *testing.T, s *connectServer) *connectServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	s.ln = ln
	t.Cleanup(func() {
		if err := ln.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			t.Errorf("close proxy listener: %v", err)
		}
	})
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go s.serve(c)
		}
	}()
	return s
}

func (s *connectServer) serve(c net.Conn) {
	defer closeQuietly(c) // the peer may be gone already
	s.mu.Lock()
	s.accepted++
	s.mu.Unlock()
	br := bufio.NewReader(c)
	req, err := http.ReadRequest(br)
	if err != nil {
		return
	}
	s.mu.Lock()
	s.targets = append(s.targets, req.Host)
	s.auths = append(s.auths, req.Header.Get("Proxy-Authorization"))
	s.mu.Unlock()
	if req.Method != http.MethodConnect {
		return
	}
	if s.garbage != "" {
		logged(io.WriteString(c, s.garbage))
		return
	}
	status := s.status
	if status == 0 {
		status = 200
	}
	if status != 200 {
		logged(io.WriteString(c, "HTTP/1.1 "+strconv.Itoa(status)+" "+http.StatusText(status)+"\r\nContent-Length: 0\r\n\r\n"))
		return
	}
	logged(io.WriteString(c, "HTTP/1.1 200 Connection established\r\n\r\n"+s.after))
	logged(io.Copy(c, br))
}

func (s *connectServer) plan(t *testing.T, user, pass string) netPlan {
	t.Helper()
	host, port := hostPort(t, s.ln.Addr().String())
	return netPlan{mode: settings.NetworkProxy, ptype: settings.ProxyHTTP, phost: host, pport: port, puser: user, ppass: pass}
}

func TestHTTPConnectDial(t *testing.T) {
	wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("user:p@ss:word"))
	cases := []struct {
		name       string
		server     *connectServer
		user, pass string
		wantAuth   string
		wantIs     error
		wantData   string
	}{
		{name: "tunnel without login", wantAuth: ""},
		{name: "tunnel with login", user: "user", pass: "p@ss:word", wantAuth: wantAuth},
		{name: "407 is a wrong login", server: &connectServer{status: 407}, user: "user", pass: "x", wantAuth: "Basic " + base64.StdEncoding.EncodeToString([]byte("user:x")), wantIs: errProxyAuthFailed},
		{name: "403 is a refusal", server: &connectServer{status: 403}, wantIs: errProxyFailed},
		{name: "not http at all", server: &connectServer{garbage: "SSH-2.0-OpenSSH\r\n"}, wantIs: errProxyFailed},
		{name: "bytes behind the 200 are not lost", server: &connectServer{after: "hello"}, wantData: "hello"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.server == nil {
				c.server = &connectServer{}
			}
			srv := startConnect(t, c.server)
			dial := httpConnectDial(srv.plan(t, c.user, c.pass))
			conn, err := dial(t.Context(), "tcp", "203.0.113.5:6881")
			if c.wantIs != nil {
				if err == nil {
					closeQuietly(conn) // the dial was meant to fail
					t.Fatal("dial succeeded, want an error")
				}
				if !errors.Is(err, c.wantIs) {
					t.Fatalf("error = %v, want %v", err, c.wantIs)
				}
			} else {
				if err != nil {
					t.Fatalf("dial: %v", err)
				}
				t.Cleanup(func() { closeQuietly(conn) }) // the echo side ends with the test
				if got := conn.RemoteAddr().String(); got != "203.0.113.5:6881" {
					t.Fatalf("RemoteAddr = %s, want the peer", got)
				}
				if c.wantData != "" {
					buf := make([]byte, len(c.wantData))
					if _, err := io.ReadFull(conn, buf); err != nil || string(buf) != c.wantData {
						t.Fatalf("data behind the 200 = %q, %v", buf, err)
					}
				} else {
					if _, err := conn.Write([]byte("ping")); err != nil {
						t.Fatalf("write: %v", err)
					}
					buf := make([]byte, 4)
					if _, err := io.ReadFull(conn, buf); err != nil || string(buf) != "ping" {
						t.Fatalf("tunnel echo = %q, %v", buf, err)
					}
				}
			}
			srv.mu.Lock()
			defer srv.mu.Unlock()
			if len(srv.targets) != 1 || srv.targets[0] != "203.0.113.5:6881" {
				t.Fatalf("proxy was asked for %v", srv.targets)
			}
			if srv.auths[0] != c.wantAuth {
				t.Fatalf("Proxy-Authorization = %q, want %q", srv.auths[0], c.wantAuth)
			}
		})
	}
}

func TestHTTPConnectDialProxyDown(t *testing.T) {
	srv := startConnect(t, &connectServer{})
	plan := srv.plan(t, "", "")
	if err := srv.ln.Close(); err != nil {
		t.Fatal(err)
	}
	conn, err := httpConnectDial(plan)(t.Context(), "tcp", "203.0.113.5:6881")
	if err == nil {
		closeQuietly(conn) // the dial was meant to fail
		t.Fatal("dial through a dead proxy succeeded")
	}
	if !errors.Is(err, errProxyUnreachable) {
		t.Fatalf("error = %v, want errProxyUnreachable", err)
	}
}

func TestHTTPConnectDialRejectsOtherNetworks(t *testing.T) {
	srv := startConnect(t, &connectServer{})
	if _, err := httpConnectDial(srv.plan(t, "", ""))(t.Context(), "udp", "203.0.113.5:6881"); err == nil {
		t.Fatal("a udp dial through an http proxy must fail")
	}
}

func TestTestProxy(t *testing.T) {
	t.Run("socks5 accepted without login", func(t *testing.T) {
		srv := startSOCKS(t, &socksServer{})
		if err := testProxy(t.Context(), socksPlan(t, srv, "", "")); err != nil {
			t.Fatalf("testProxy: %v", err)
		}
		if seen := srv.seen(); len(seen) != 0 {
			t.Fatalf("the test connected somewhere through the proxy: %v", seen)
		}
	})
	t.Run("socks5 login accepted", func(t *testing.T) {
		srv := startSOCKS(t, &socksServer{user: "u", pass: "p"})
		if err := testProxy(t.Context(), socksPlan(t, srv, "u", "p")); err != nil {
			t.Fatalf("testProxy: %v", err)
		}
	})
	t.Run("socks5 wrong password", func(t *testing.T) {
		srv := startSOCKS(t, &socksServer{user: "u", pass: "p"})
		if err := testProxy(t.Context(), socksPlan(t, srv, "u", "nope")); !errors.Is(err, errProxyAuthFailed) {
			t.Fatalf("error = %v, want errProxyAuthFailed", err)
		}
	})
	t.Run("socks5 wants a login and none is set", func(t *testing.T) {
		srv := startSOCKS(t, &socksServer{user: "u", pass: "p"})
		if err := testProxy(t.Context(), socksPlan(t, srv, "", "")); !errors.Is(err, errProxyAuthFailed) {
			t.Fatalf("error = %v, want errProxyAuthFailed", err)
		}
	})
	t.Run("not a socks5 server", func(t *testing.T) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := ln.Close(); err != nil {
				t.Errorf("close listener: %v", err)
			}
		})
		go func() {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			logged(io.WriteString(c, "HTTP/1.1 400 Bad Request\r\n\r\n"))
			closeQuietly(c) // the answer is written, nothing more to say
		}()
		host, port := hostPort(t, ln.Addr().String())
		p := netPlan{mode: settings.NetworkProxy, ptype: settings.ProxySOCKS5, phost: host, pport: port}
		if err := testProxy(t.Context(), p); !errors.Is(err, errProxyFailed) {
			t.Fatalf("error = %v, want errProxyFailed", err)
		}
	})
	t.Run("socks5 unreachable", func(t *testing.T) {
		srv := startSOCKS(t, &socksServer{})
		plan := socksPlan(t, srv, "", "")
		if err := srv.ln.Close(); err != nil {
			t.Fatal(err)
		}
		if err := testProxy(t.Context(), plan); !errors.Is(err, errProxyUnreachable) {
			t.Fatalf("error = %v, want errProxyUnreachable", err)
		}
	})
	t.Run("silent proxy is cut by the context", func(t *testing.T) {
		srv := startSOCKS(t, &socksServer{silent: true})
		ctx, cancel := context.WithCancel(t.Context())
		go func() {
			<-srv.accepted
			cancel()
		}()
		if err := testProxy(ctx, socksPlan(t, srv, "", "")); !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want context.Canceled", err)
		}
	})
	for _, status := range []int{200, 403, 502} {
		t.Run("http answers "+strconv.Itoa(status), func(t *testing.T) {
			srv := startConnect(t, &connectServer{status: status})
			if err := testProxy(t.Context(), srv.plan(t, "u", "p")); err != nil {
				t.Fatalf("testProxy: %v", err)
			}
		})
	}
	t.Run("http 407", func(t *testing.T) {
		srv := startConnect(t, &connectServer{status: 407})
		if err := testProxy(t.Context(), srv.plan(t, "u", "p")); !errors.Is(err, errProxyAuthFailed) {
			t.Fatalf("error = %v, want errProxyAuthFailed", err)
		}
	})
	t.Run("http unreachable", func(t *testing.T) {
		srv := startConnect(t, &connectServer{})
		plan := srv.plan(t, "", "")
		if err := srv.ln.Close(); err != nil {
			t.Fatal(err)
		}
		if err := testProxy(t.Context(), plan); !errors.Is(err, errProxyUnreachable) {
			t.Fatalf("error = %v, want errProxyUnreachable", err)
		}
	})
}

func TestProxyOnlyDial(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := ln.Close(); err != nil {
			t.Errorf("close listener: %v", err)
		}
	})
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			closeQuietly(c) // the guard test only needs the connection to open
		}
	}()
	guardHost, guardPort := hostPort(t, ln.Addr().String())
	guard := proxyOnlyDial(guardHost, guardPort)

	conn, err := guard(t.Context(), "tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("the proxy itself must be reachable: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}

	_, port := hostPort(t, ln.Addr().String())
	for _, addr := range []string{"127.0.0.2:" + strconv.Itoa(port), "127.0.0.1:1", "203.0.113.5:443", "no-port", ""} {
		if conn, err := guard(t.Context(), "tcp", addr); err == nil {
			closeQuietly(conn) // the dial was meant to be refused
			t.Errorf("guard let %q through", addr)
		} else if !errors.Is(err, errDirectBlocked) {
			t.Errorf("guard error for %q = %v, want errDirectBlocked", addr, err)
		}
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	byName := proxyOnlyDial("Proxy.Example", 1080)
	if _, err := byName(ctx, "tcp", "proxy.example:1080"); errors.Is(err, errDirectBlocked) {
		t.Fatalf("host names compare case-insensitively, got %v", err)
	}
}

func TestBlockedPacketConn(t *testing.T) {
	pc := newBlockedPacketConn()
	if _, err := pc.WriteTo([]byte("x"), &net.UDPAddr{IP: net.IPv4(203, 0, 113, 5), Port: 6969}); !errors.Is(err, errDirectBlocked) {
		t.Fatalf("WriteTo = %v, want errDirectBlocked", err)
	}
	done := make(chan error, 1)
	go func() {
		_, _, err := pc.ReadFrom(make([]byte, 16))
		done <- err
	}()
	if err := pc.Close(); err != nil {
		t.Fatal(err)
	}
	if err := pc.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, net.ErrClosed) {
			t.Fatalf("ReadFrom = %v, want net.ErrClosed", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ReadFrom is still blocked after Close")
	}
}

func TestBindDialerFamilies(t *testing.T) {
	v4, v6 := netip.MustParseAddr("10.0.0.2"), netip.MustParseAddr("fd00::2")
	cases := []struct {
		name     string
		b        bindDialer
		network  string
		want     []string
		wantFail bool
	}{
		{"both, any tcp", bindDialer{ip4: v4, ip6: v6}, "tcp", []string{"tcp4", "tcp6"}, false},
		{"both, tcp4", bindDialer{ip4: v4, ip6: v6}, "tcp4", []string{"tcp4"}, false},
		{"both, tcp6", bindDialer{ip4: v4, ip6: v6}, "tcp6", []string{"tcp6"}, false},
		{"v4 only, any tcp", bindDialer{ip4: v4}, "tcp", []string{"tcp4"}, false},
		{"v4 only, tcp6", bindDialer{ip4: v4}, "tcp6", nil, true},
		{"v6 only, tcp4", bindDialer{ip6: v6}, "tcp4", nil, true},
		{"udp is not dialled here", bindDialer{ip4: v4}, "udp", nil, true},
		{"no addresses", bindDialer{}, "tcp", nil, true},
	}
	for _, c := range cases {
		got, err := c.b.families(c.network)
		if c.wantFail {
			if err == nil {
				t.Errorf("%s: families = %v, want an error", c.name, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		var names []string
		for _, f := range got {
			names = append(names, f.network)
		}
		if strings.Join(names, ",") != strings.Join(c.want, ",") {
			t.Errorf("%s: families = %v, want %v", c.name, names, c.want)
		}
	}
}

// Binding to 127.0.0.2 stands in for an adapter address: the listener has to
// see the connection come from it, not from whatever address the routing table
// would pick.
func TestBindDialerDialsFromTheBoundAddress(t *testing.T) {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := ln.Close(); err != nil {
			t.Errorf("close listener: %v", err)
		}
	})
	remote := make(chan net.Addr, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			remote <- nil
			return
		}
		remote <- c.RemoteAddr()
		closeQuietly(c) // only the peer address was wanted
	}()

	var calls []string
	b := &bindDialer{
		ip4: netip.MustParseAddr("127.0.0.2"),
		control: func(network, _ string, _ syscall.RawConn) error {
			calls = append(calls, network)
			return nil
		},
	}
	conn, err := b.DialContext(t.Context(), "tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("DialContext: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	got := <-remote
	if got == nil || tcpAddrOf(t, got).IP.String() != "127.0.0.2" {
		t.Fatalf("the listener saw %v, want a connection from 127.0.0.2", got)
	}
	if len(calls) != 1 || calls[0] != "tcp4" {
		t.Fatalf("control hook calls = %v, want one for tcp4", calls)
	}
}

func TestBindDialerReportsEveryFamilyThatFailed(t *testing.T) {
	b := &bindDialer{ip4: netip.MustParseAddr("127.0.0.2"), ip6: netip.MustParseAddr("::1")}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	// Port 1 on loopback is closed: both families fail, and neither error may
	// be swallowed.
	conn, err := b.DialContext(ctx, "tcp", "localhost:1")
	if err == nil {
		closeQuietly(conn) // the dial was meant to fail
		t.Fatal("dial to a closed port succeeded")
	}
}

func TestBindDialerListenPacket(t *testing.T) {
	b := &bindDialer{ip4: netip.MustParseAddr("127.0.0.2"), ctx: t.Context()}

	pc, err := b.listenPacket("udp4", ":0")
	if err != nil {
		t.Fatalf("listenPacket: %v", err)
	}
	t.Cleanup(func() { closeQuietly(pc) }) // the socket has no other owner
	if ip := udpAddrOf(t, pc.LocalAddr()).IP.String(); ip != "127.0.0.2" {
		t.Fatalf("udp tracker socket bound to %s, want the adapter address", ip)
	}

	blocked, err := b.listenPacket("udp6", ":0")
	if err != nil {
		t.Fatalf("listenPacket(udp6): %v", err)
	}
	t.Cleanup(func() { closeQuietly(blocked) }) // the socket is a stub with nothing to release
	if _, err := blocked.WriteTo([]byte("x"), &net.UDPAddr{IP: net.IPv4(203, 0, 113, 5), Port: 6969}); !errors.Is(err, errDirectBlocked) {
		t.Fatalf("a family the adapter has no address for must carry nothing, WriteTo = %v", err)
	}

	dead := &bindDialer{ip4: netip.MustParseAddr("198.51.100.99"), ctx: t.Context()}
	pc2, err := dead.listenPacket("udp4", ":0")
	if err != nil {
		t.Fatalf("a socket that cannot bind must not surface as an error, the library panics on it: %v", err)
	}
	t.Cleanup(func() { closeQuietly(pc2) }) // the socket is a stub with nothing to release
	if _, err := pc2.WriteTo([]byte("x"), &net.UDPAddr{IP: net.IPv4(203, 0, 113, 5), Port: 6969}); !errors.Is(err, errDirectBlocked) {
		t.Fatalf("an unbindable address must fall back to a socket that carries nothing, WriteTo = %v", err)
	}
}

func TestClassifySOCKSError(t *testing.T) {
	cases := []struct {
		err  error
		want error
	}{
		{errors.New("socks connect tcp 127.0.0.1:1->x:2: username/password authentication failed"), errProxyAuthFailed},
		{errors.New("no acceptable authentication methods"), errProxyAuthFailed},
		{errors.New("unsupported authentication method 9"), errProxyAuthFailed},
		{errors.New("unknown error connection refused"), nil},
	}
	for _, c := range cases {
		got := classifySOCKSError(c.err)
		if c.want == nil {
			if errors.Is(got, errProxyAuthFailed) {
				t.Errorf("%v classified as an auth failure", c.err)
			}
			continue
		}
		if !errors.Is(got, c.want) {
			t.Errorf("%v classified as %v, want %v", c.err, got, c.want)
		}
	}
	if got := classifySOCKSError(context.Canceled); !errors.Is(got, context.Canceled) || errors.Is(got, errProxyAuthFailed) {
		t.Errorf("a cancelled dial must stay cancelled, got %v", got)
	}
}

func TestProtocolError(t *testing.T) {
	cases := []struct {
		name string
		in   error
		is   error
		not  error
	}{
		{"nil stays nil", nil, nil, errProxyFailed},
		{"a cut connection is the proxy's fault", io.ErrUnexpectedEOF, errProxyFailed, nil},
		{"a reset too", syscall.ECONNRESET, errProxyFailed, nil},
		{"auth failure keeps its reason", errProxyAuthFailed, errProxyAuthFailed, errProxyFailed},
		{"cancellation is not the proxy's fault", context.Canceled, context.Canceled, errProxyFailed},
		{"deadline neither", context.DeadlineExceeded, context.DeadlineExceeded, errProxyFailed},
	}
	for _, c := range cases {
		got := protocolError(c.in)
		if c.in == nil {
			if got != nil {
				t.Errorf("%s: %v", c.name, got)
			}
			continue
		}
		if !errors.Is(got, c.is) {
			t.Errorf("%s: %v is not %v", c.name, got, c.is)
		}
		if c.not != nil && errors.Is(got, c.not) {
			t.Errorf("%s: %v must not be %v", c.name, got, c.not)
		}
	}
}

func TestTestProxyWhenTheProxyHangsUp(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := ln.Close(); err != nil {
			t.Errorf("close listener: %v", err)
		}
	})
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			closeQuietly(c) // hangs up without a word
		}
	}()
	host, port := hostPort(t, ln.Addr().String())
	for _, kind := range []string{settings.ProxySOCKS5, settings.ProxyHTTP} {
		p := netPlan{mode: settings.NetworkProxy, ptype: kind, phost: host, pport: port}
		if err := testProxy(t.Context(), p); !errors.Is(err, errProxyFailed) {
			t.Errorf("%s: error = %v, want errProxyFailed", kind, err)
		}
	}
}

var unsafeTargets = map[string]string{
	"crlf in the host":    "a\r\nX-Injected: 1:80",
	"crlf in the port":    "example.com:80\r\nX-Injected: 1",
	"lf only":             "example.com\n:80",
	"space in the host":   "exa mple.com:80",
	"tab in the host":     "example\t.com:80",
	"nul in the host":     "example\x00.com:80",
	"no port":             "example.com",
	"empty port":          "example.com:",
	"port zero":           "example.com:0",
	"port too big":        "example.com:70000",
	"port with a sign":    "example.com:+80",
	"port is a word":      "example.com:http",
	"empty host":          ":80",
	"empty target":        "",
	"label too long":      strings.Repeat("a", 64) + ".example:80",
	"name too long":       strings.Repeat("a.", 130) + "com:80",
	"empty label":         "a..b:80",
	"leading hyphen":      "-a.example:80",
	"non ascii name":      "пир.example:80",
	"ipv6 with a zone":    "[fe80::1%eth0]:80",
	"ipv6 without braces": "2001:db8::1:80",
	"an url":              "http://example.com:80",
	"userinfo":            "user@example.com:80",
	"target too long":     strings.Repeat("a", 300) + ":80",
}

func TestProxyTargetValidation(t *testing.T) {
	good := []string{
		"203.0.113.5:6881",
		"[2001:db8::1]:6881",
		"tracker.example:80",
		"tracker.example.:80",
		"_dmarc.example:80",
		"203.0.113.5:1",
		"203.0.113.5:65535",
		"xn--e1afmkfd.example:80",
	}
	for _, target := range good {
		t.Run("accepts "+target, func(t *testing.T) {
			if err := validProxyTarget(target); err != nil {
				t.Fatalf("validProxyTarget(%q) = %v", target, err)
			}
		})
	}
	for name, target := range unsafeTargets {
		t.Run("refuses "+name, func(t *testing.T) {
			if err := validProxyTarget(target); !errors.Is(err, errBadTarget) {
				t.Fatalf("validProxyTarget(%q) = %v, want errBadTarget", target, err)
			}
		})
	}
}

func TestConnectStatusSendsNothingForAnUnsafeTarget(t *testing.T) {
	for name, target := range unsafeTargets {
		t.Run(name, func(t *testing.T) {
			client, server := net.Pipe()
			t.Cleanup(func() { closeQuietly(client); closeQuietly(server) })
			if err := client.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
				t.Fatal(err)
			}
			got := make(chan []byte, 1)
			go func() {
				data, err := io.ReadAll(server) // ends when the client side is closed
				if err != nil {
					data = append(data, "read error: "+err.Error()...)
				}
				got <- data
			}()
			_, _, err := connectStatus(client, bufio.NewReader(client), target, "")
			closeQuietly(client)
			if !errors.Is(err, errBadTarget) {
				t.Fatalf("connectStatus(%q) = %v, want errBadTarget", target, err)
			}
			if data := <-got; len(data) != 0 {
				t.Fatalf("%q reached the proxy: %q", target, data)
			}
		})
	}
}

func TestHTTPConnectDialRefusesAnUnsafeTargetBeforeTouchingTheProxy(t *testing.T) {
	for name, target := range unsafeTargets {
		t.Run(name, func(t *testing.T) {
			srv := startConnect(t, &connectServer{})
			conn, err := httpConnectDial(srv.plan(t, "u", "p"))(t.Context(), "tcp", target)
			if err == nil {
				closeQuietly(conn) // the dial was meant to fail
				t.Fatalf("dial of %q succeeded", target)
			}
			if !errors.Is(err, errBadTarget) {
				t.Fatalf("dial of %q = %v, want errBadTarget", target, err)
			}
			srv.mu.Lock()
			defer srv.mu.Unlock()
			if srv.accepted != 0 || len(srv.targets) != 0 {
				t.Fatalf("the proxy saw %d connections and %v", srv.accepted, srv.targets)
			}
		})
	}
}

func TestSOCKS5DialRefusesAnUnsafeTargetBeforeTouchingTheProxy(t *testing.T) {
	for name, target := range unsafeTargets {
		t.Run(name, func(t *testing.T) {
			srv := startSOCKS(t, &socksServer{})
			dial, err := socks5Dial(socksPlan(t, srv, "", ""))
			if err != nil {
				t.Fatal(err)
			}
			conn, err := dial(t.Context(), "tcp", target)
			if err == nil {
				closeQuietly(conn) // the dial was meant to fail
				t.Fatalf("dial of %q succeeded", target)
			}
			if !errors.Is(err, errBadTarget) {
				t.Fatalf("dial of %q = %v, want errBadTarget", target, err)
			}
			if n := len(srv.accepted); n != 0 || len(srv.seen()) != 0 {
				t.Fatalf("the proxy saw %d connections and %v", n, srv.seen())
			}
		})
	}
}
