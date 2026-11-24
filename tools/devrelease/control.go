package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

// failMode is a failure the source produces on demand. Each one is a kind
// of failure a real release server or network can cause, and ends in the
// launcher as the selfupdate.* code that failure produces (the devmock
// tests in launcher_devmock_test.go hold the mapping).
type failMode string

const (
	failNone         failMode = ""
	failManifest500  failMode = "manifest-500"
	failBadSignature failMode = "bad-signature"
	failUnknownKey   failMode = "unknown-key"
	failArtifact500  failMode = "artifact-500"
	failStall        failMode = "stall"
	failHash         failMode = "hash"
	failSize         failMode = "size"
)

var failModes = []failMode{failManifest500, failBadSignature, failUnknownKey, failArtifact500, failStall, failHash, failSize}

func parseFailMode(s string) (failMode, bool) {
	if s == "" {
		return failNone, true
	}
	for _, m := range failModes {
		if string(m) == s {
			return m, true
		}
	}
	return failNone, false
}

// control is what can be switched while the source runs. changed is closed
// and replaced on every switch, so a stalled download learns at once that
// its stall is over.
type control struct {
	mu         sync.Mutex
	seconds    int
	mode       failMode
	changed    chan struct{}
	lastClient string
	lastOffer  string
}

func newControl(seconds int) *control {
	return &control{seconds: seconds, changed: make(chan struct{})}
}

func (c *control) pace() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.seconds
}

func (c *control) current() failMode {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.mode
}

func (c *control) set(mode *failMode, seconds *int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if mode != nil {
		c.mode = *mode
	}
	if seconds != nil {
		c.seconds = *seconds
	}
	close(c.changed)
	c.changed = make(chan struct{})
}

func (c *control) offered(client, version string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastClient, c.lastOffer = client, version
}

// holdWhileStalled blocks while the stall mode is on, until it is switched
// off or the client goes away.
func (c *control) holdWhileStalled(ctx context.Context) error {
	for {
		c.mu.Lock()
		mode, changed := c.mode, c.changed
		c.mu.Unlock()
		if mode != failStall {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-changed:
		}
	}
}

func (c *control) report(versions []string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	mode := string(c.mode)
	if mode == "" {
		mode = "none"
	}
	names := make([]string, len(failModes))
	for i, m := range failModes {
		names[i] = string(m)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "fail=%s\n", mode)
	fmt.Fprintf(&b, "seconds=%d\n", c.seconds)
	fmt.Fprintf(&b, "versions=%s\n", strings.Join(versions, ","))
	if c.lastOffer != "" {
		client := c.lastClient
		if client == "" {
			client = "unknown"
		}
		fmt.Fprintf(&b, "last-client=%s offered=%s\n", client, c.lastOffer)
	}
	fmt.Fprintf(&b, "modes=%s\n", strings.Join(names, ","))
	return b.String()
}

func (s *releaseServer) versions() []string {
	out := make([]string, len(s.releases))
	for i, rel := range s.releases {
		out[i] = rel.version
	}
	return out
}

func (s *releaseServer) writeReport(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if _, err := w.Write([]byte(s.control.report(s.versions()))); err != nil {
		slog.Warn("devrelease: write control state", "error", err)
	}
}

func (s *releaseServer) handleControlGet(w http.ResponseWriter, _ *http.Request) {
	s.writeReport(w)
}

// handleControlPost switches the source: ?fail=<mode> (empty for normal)
// and/or ?seconds=<n> for the pace of downloads that start afterwards.
func (s *releaseServer) handleControlPost(w http.ResponseWriter, r *http.Request) {
	// A page open in the developer's browser can POST to 127.0.0.1 as well;
	// the browser marks such a request with Origin, a terminal does not.
	if r.Header.Get("Origin") != "" {
		http.Error(w, "control is for local tools, not web pages", http.StatusForbidden)
		return
	}
	q := r.URL.Query()
	var mode *failMode
	if q.Has("fail") {
		m, ok := parseFailMode(q.Get("fail"))
		if !ok {
			http.Error(w, fmt.Sprintf("unknown fail mode %q; known: %v", q.Get("fail"), failModes), http.StatusBadRequest)
			return
		}
		mode = &m
	}
	var seconds *int
	if q.Has("seconds") {
		n, err := strconv.Atoi(q.Get("seconds"))
		if err != nil || n < 0 {
			http.Error(w, "seconds must be a whole number of seconds, 0 or more", http.StatusBadRequest)
			return
		}
		seconds = &n
	}
	s.control.set(mode, seconds)
	slog.Info("devrelease: control", "fail", q.Get("fail"), "seconds", q.Get("seconds"))
	s.writeReport(w)
}

// loopbackOnly refuses any peer that is not this machine. The listener binds
// 127.0.0.1 already; this holds even if the handler is ever served elsewhere.
func loopbackOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		ip := net.ParseIP(host)
		if err != nil || ip == nil || !ip.IsLoopback() {
			http.Error(w, "loopback only", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
