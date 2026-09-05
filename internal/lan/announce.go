package lan

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"typhon/internal/uierr"

	"golang.org/x/net/ipv4"
	"golang.org/x/time/rate"
)

const (
	multicastPort    = 42816
	maxAnnounceBytes = 1200
	recvBufferSize   = 1500
	readPollInterval = 250 * time.Millisecond
	peerTTL          = 35 * time.Second
	announceInterval = 10 * time.Second
	maxPeers         = 64
	maxOffers        = 256
	rateBurst        = 20
	rateWindow       = 10 * time.Second
	tsSkew           = 5 * time.Minute
)

var multicastGroup = netip.MustParseAddrPort("239.255.71.84:42816")

var (
	errAnnounceTooLarge     = errors.New("lan: announce datagram too large")
	errAnnounceSourceAddr   = errors.New("lan: announce source address is not local")
	errAnnounceBadJSON      = errors.New("lan: announce is not valid json")
	errAnnounceVersion      = errors.New("lan: announce protocol version unsupported")
	errAnnounceID           = uierr.New("lan.invalid_peer_id", "lan: announce id invalid")
	errAnnounceOwnID        = errors.New("lan: announce is from ourselves")
	errAnnounceHost         = errors.New("lan: announce host invalid")
	errAnnouncePort         = errors.New("lan: announce port invalid")
	errAnnounceInfoHash     = uierr.New("lan.invalid_info_hash", "lan: announce infohash invalid")
	errAnnounceTitle        = errors.New("lan: announce title invalid")
	errAnnounceVersionField = errors.New("lan: announce version field invalid")
	errAnnounceGameID       = errors.New("lan: announce gameId invalid")
	errAnnounceSize         = errors.New("lan: announce size invalid")
	errAnnounceExe          = errors.New("lan: announce exe invalid")
	errAnnounceTimestamp    = errors.New("lan: announce timestamp out of range")
)

var (
	idRe        = regexp.MustCompile(`^[0-9a-f]{32}$`)
	infoHashRe  = regexp.MustCompile(`^[0-9a-f]{40}$`)
	gameIDRe    = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	driveLetter = regexp.MustCompile(`^[A-Za-z]:`)
)

// transport lets announce.go be exercised in a single process without a
// real network: production uses newMulticast, tests use newLoopback.
type transport interface {
	Send(ctx context.Context, payload []byte) error
	Recv(ctx context.Context) ([]byte, netip.Addr, error)
	Close() error
}

type announceMsg struct {
	V        int    `json:"v"`
	ID       string `json:"id"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	GameID   string `json:"gameId"`
	Title    string `json:"title"`
	Version  string `json:"version"`
	Exe      string `json:"exe"`
	Size     int64  `json:"size"`
	InfoHash string `json:"infoHash"`
	TS       int64  `json:"ts"`
}

func encodeAnnounce(msg announceMsg) ([]byte, error) {
	data, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("marshal announce: %w", err)
	}
	if len(data) > maxAnnounceBytes {
		return nil, fmt.Errorf("lan: encoded announce is %d bytes, over the %d limit", len(data), maxAnnounceBytes)
	}
	return data, nil
}

// decodeAnnounce validates every field of a received datagram before any of
// it is used, one distinct error per cause (invariant 25/32): a caller that
// only learns "invalid" can't tell a hostile peer from a stale build.
func decodeAnnounce(raw []byte, srcAddr netip.Addr, selfID string, now time.Time) (announceMsg, error) {
	if len(raw) > maxAnnounceBytes {
		return announceMsg{}, errAnnounceTooLarge
	}
	if !isLocalSource(srcAddr) {
		return announceMsg{}, errAnnounceSourceAddr
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var msg announceMsg
	if err := dec.Decode(&msg); err != nil {
		return announceMsg{}, fmt.Errorf("%w: %w", errAnnounceBadJSON, err)
	}
	if dec.More() {
		return announceMsg{}, errAnnounceBadJSON
	}

	if msg.V != 1 {
		return announceMsg{}, errAnnounceVersion
	}
	if !idRe.MatchString(msg.ID) {
		return announceMsg{}, errAnnounceID
	}
	if msg.ID == selfID {
		return announceMsg{}, errAnnounceOwnID
	}
	if !validHost(msg.Host) {
		return announceMsg{}, errAnnounceHost
	}
	if msg.Port < 1 || msg.Port > 65535 {
		return announceMsg{}, errAnnouncePort
	}
	if !infoHashRe.MatchString(msg.InfoHash) {
		return announceMsg{}, errAnnounceInfoHash
	}
	if !validTitle(msg.Title) {
		return announceMsg{}, errAnnounceTitle
	}
	if len([]rune(msg.Version)) > 64 {
		return announceMsg{}, errAnnounceVersionField
	}
	if !gameIDRe.MatchString(msg.GameID) {
		return announceMsg{}, errAnnounceGameID
	}
	if msg.Size < 0 || msg.Size > 1<<48 {
		return announceMsg{}, errAnnounceSize
	}
	if !validExe(msg.Exe) {
		return announceMsg{}, errAnnounceExe
	}
	skew := time.Unix(msg.TS, 0)
	if skew.Before(now.Add(-tsSkew)) || skew.After(now.Add(tsSkew)) {
		return announceMsg{}, errAnnounceTimestamp
	}
	return msg, nil
}

func validHost(host string) bool {
	r := []rune(host)
	if len(r) < 1 || len(r) > 63 {
		return false
	}
	for _, c := range r {
		if !unicode.IsPrint(c) {
			return false
		}
	}
	return true
}

func validTitle(title string) bool {
	n := len([]rune(title))
	return n >= 1 && n <= 512
}

func validExe(exe string) bool {
	if exe == "" || len([]rune(exe)) > 260 {
		return false
	}
	if strings.HasPrefix(exe, "/") || strings.HasPrefix(exe, "\\") {
		return false
	}
	if driveLetter.MatchString(exe) {
		return false
	}
	for _, seg := range strings.FieldsFunc(exe, func(r rune) bool { return r == '/' || r == '\\' }) {
		if seg == ".." || seg == "" {
			return false
		}
	}
	return true
}

// isLocalSource is the mirror of internal/sources/feed/guard.go's
// blockedAddr: that one keeps a fetch off private networks, this one
// requires an announce come from one, since anything else cannot be a LAN
// peer and the datagram's stated host/port must never be trusted alone.
func isLocalSource(addr netip.Addr) bool {
	addr = addr.Unmap()
	if !addr.IsValid() {
		return false
	}
	return addr.IsLoopback() || addr.IsPrivate() || addr.IsLinkLocalUnicast()
}

func sanitizeText(s string, maxRunes int) string {
	var b strings.Builder
	n := 0
	for _, r := range s {
		if n >= maxRunes {
			break
		}
		if r == unicode.ReplacementChar || !unicode.IsPrint(r) {
			continue
		}
		b.WriteRune(r)
		n++
	}
	return b.String()
}

func reasonFor(err error) string {
	switch {
	case errors.Is(err, errAnnounceTooLarge):
		return "too_large"
	case errors.Is(err, errAnnounceSourceAddr):
		return "bad_source_addr"
	case errors.Is(err, errAnnounceBadJSON):
		return "bad_json"
	case errors.Is(err, errAnnounceVersion):
		return "bad_version"
	case errors.Is(err, errAnnounceID):
		return "bad_id"
	case errors.Is(err, errAnnounceOwnID):
		return "own_id"
	case errors.Is(err, errAnnounceHost):
		return "bad_host"
	case errors.Is(err, errAnnouncePort):
		return "bad_port"
	case errors.Is(err, errAnnounceInfoHash):
		return "bad_infohash"
	case errors.Is(err, errAnnounceTitle):
		return "bad_title"
	case errors.Is(err, errAnnounceVersionField):
		return "bad_version_field"
	case errors.Is(err, errAnnounceGameID):
		return "bad_gameid"
	case errors.Is(err, errAnnounceSize):
		return "bad_size"
	case errors.Is(err, errAnnounceExe):
		return "bad_exe"
	case errors.Is(err, errAnnounceTimestamp):
		return "bad_ts"
	default:
		return "unknown"
	}
}

// sourceLimiter bounds how many announces per source IP get parsed at all,
// independent of whether their content is valid: 20 datagrams per 10s.
type sourceLimiter struct {
	mu       sync.Mutex
	limiters map[netip.Addr]*rate.Limiter
}

func newSourceLimiter() *sourceLimiter {
	return &sourceLimiter{limiters: map[netip.Addr]*rate.Limiter{}}
}

func (l *sourceLimiter) allow(addr netip.Addr, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	lim, ok := l.limiters[addr]
	if !ok {
		lim = rate.NewLimiter(rate.Every(rateWindow/rateBurst), rateBurst)
		l.limiters[addr] = lim
	}
	return lim.AllowN(now, 1)
}

// prune drops limiters that have not been touched recently so a churn of
// short-lived sources does not grow this map without bound.
func (l *sourceLimiter) prune(now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for addr, lim := range l.limiters {
		if lim.TokensAt(now) >= rateBurst {
			delete(l.limiters, addr)
		}
	}
}

type offerKey struct {
	peerID   string
	infoHash string
}

// peerTable holds what has been heard from the LAN recently. Every read
// method takes "now" explicitly and prunes against it rather than reading
// time.Now() itself, so expiry is testable with an injected clock and no
// background timer is required for correctness.
type peerTable struct {
	mu     sync.Mutex
	peers  map[string]Peer
	offers map[offerKey]Offer
}

func newPeerTable() *peerTable {
	return &peerTable{peers: map[string]Peer{}, offers: map[offerKey]Offer{}}
}

func (t *peerTable) observe(msg announceMsg, srcAddr netip.Addr, now time.Time) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.pruneLocked(now)

	host := sanitizeText(msg.Host, 63)
	if _, known := t.peers[msg.ID]; !known && len(t.peers) >= maxPeers {
		return false
	}
	t.peers[msg.ID] = Peer{
		ID:       msg.ID,
		Host:     host,
		Addr:     srcAddr.String(),
		Port:     msg.Port,
		LastSeen: now,
	}

	key := offerKey{peerID: msg.ID, infoHash: msg.InfoHash}
	if _, known := t.offers[key]; !known && len(t.offers) >= maxOffers {
		return true
	}
	t.offers[key] = Offer{
		PeerID:    msg.ID,
		Host:      host,
		Addr:      srcAddr.String(),
		Port:      msg.Port,
		GameID:    msg.GameID,
		Title:     sanitizeText(msg.Title, 512),
		Version:   msg.Version,
		Exe:       msg.Exe,
		SizeBytes: msg.Size,
		InfoHash:  msg.InfoHash,
		LastSeen:  now,
	}
	return true
}

func (t *peerTable) pruneLocked(now time.Time) {
	cutoff := now.Add(-peerTTL)
	for id, p := range t.peers {
		if p.LastSeen.Before(cutoff) {
			delete(t.peers, id)
		}
	}
	for k, o := range t.offers {
		if o.LastSeen.Before(cutoff) {
			delete(t.offers, k)
		}
	}
}

func (t *peerTable) list(now time.Time) []Peer {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.pruneLocked(now)
	out := make([]Peer, 0, len(t.peers))
	for _, p := range t.peers {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (t *peerTable) available(now time.Time) []Offer {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.pruneLocked(now)
	out := make([]Offer, 0, len(t.offers))
	for _, o := range t.offers {
		out = append(out, o)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].PeerID != out[j].PeerID {
			return out[i].PeerID < out[j].PeerID
		}
		return out[i].InfoHash < out[j].InfoHash
	})
	return out
}

func (t *peerTable) find(peerID, infoHash string, now time.Time) (Offer, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.pruneLocked(now)
	o, ok := t.offers[offerKey{peerID: peerID, infoHash: infoHash}]
	return o, ok
}

// statCounters backs Stats: what was seen, sent and rejected, and why.
type statCounters struct {
	mu       sync.Mutex
	sent     int64
	received int64
	rejected map[string]int64
}

func newStatCounters() *statCounters {
	return &statCounters{rejected: map[string]int64{}}
}

func (s *statCounters) addSent(n int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sent += n
}

func (s *statCounters) addReceived() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.received++
}

func (s *statCounters) reject(reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rejected[reason]++
}

func (s *statCounters) snapshot() (sent, received int64, rejected map[string]int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rejected = make(map[string]int64, len(s.rejected))
	for k, v := range s.rejected {
		rejected[k] = v
	}
	return s.sent, s.received, rejected
}

// loopbackTransport fans a datagram out to a fixed set of addresses instead
// of joining a multicast group, so tests can run two Services in one
// process without any real network traffic.
type loopbackTransport struct {
	conn  *net.UDPConn
	peers []netip.AddrPort
}

func newLoopback(peers []netip.AddrPort, listen netip.AddrPort) (transport, error) {
	conn, err := net.ListenUDP("udp", net.UDPAddrFromAddrPort(listen))
	if err != nil {
		return nil, fmt.Errorf("listen %s: %w", listen, err)
	}
	return &loopbackTransport{conn: conn, peers: peers}, nil
}

func (l *loopbackTransport) Send(ctx context.Context, payload []byte) error {
	for _, p := range l.peers {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err := l.conn.WriteToUDPAddrPort(payload, p); err != nil {
			return fmt.Errorf("send to %s: %w", p, err)
		}
	}
	return nil
}

func (l *loopbackTransport) Recv(ctx context.Context) ([]byte, netip.Addr, error) {
	buf := make([]byte, recvBufferSize)
	for {
		if err := ctx.Err(); err != nil {
			return nil, netip.Addr{}, err
		}
		if err := l.conn.SetReadDeadline(time.Now().Add(readPollInterval)); err != nil {
			return nil, netip.Addr{}, fmt.Errorf("set read deadline: %w", err)
		}
		n, addr, err := l.conn.ReadFromUDPAddrPort(buf)
		if err != nil {
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				continue
			}
			return nil, netip.Addr{}, err
		}
		out := make([]byte, n)
		copy(out, buf[:n])
		return out, addr.Addr().Unmap(), nil
	}
}

func (l *loopbackTransport) Close() error {
	return l.conn.Close()
}

// multicastTransport is the real, on-LAN transport: one UDP socket bound to
// the announce port, joined to the multicast group on every up, non-loopback
// interface that supports multicast.
type multicastTransport struct {
	pc    *ipv4.PacketConn
	conn  *net.UDPConn
	group *net.UDPAddr
}

func newMulticast(group netip.AddrPort, ifaces []net.Interface) (transport, error) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: int(group.Port())})
	if err != nil {
		return nil, fmt.Errorf("listen multicast port %d: %w", group.Port(), err)
	}
	pc := ipv4.NewPacketConn(conn)
	groupAddr := net.UDPAddrFromAddrPort(netip.AddrPortFrom(group.Addr(), group.Port()))

	joined := 0
	for i := range ifaces {
		ifi := ifaces[i]
		if ifi.Flags&net.FlagUp == 0 || ifi.Flags&net.FlagLoopback != 0 || ifi.Flags&net.FlagMulticast == 0 {
			continue
		}
		if err := pc.JoinGroup(&ifi, groupAddr); err != nil {
			continue
		}
		joined++
	}
	if joined == 0 {
		if cerr := conn.Close(); cerr != nil {
			return nil, fmt.Errorf("no multicast-capable interface joined %s (and close failed: %w)", group, cerr)
		}
		return nil, fmt.Errorf("lan: no interface joined multicast group %s", group)
	}
	return &multicastTransport{pc: pc, conn: conn, group: groupAddr}, nil
}

func (m *multicastTransport) Send(ctx context.Context, payload []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	_, err := m.pc.WriteTo(payload, nil, m.group)
	return err
}

func (m *multicastTransport) Recv(ctx context.Context) ([]byte, netip.Addr, error) {
	buf := make([]byte, recvBufferSize)
	for {
		if err := ctx.Err(); err != nil {
			return nil, netip.Addr{}, err
		}
		if err := m.pc.SetReadDeadline(time.Now().Add(readPollInterval)); err != nil {
			return nil, netip.Addr{}, fmt.Errorf("set read deadline: %w", err)
		}
		n, _, src, err := m.pc.ReadFrom(buf)
		if err != nil {
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				continue
			}
			return nil, netip.Addr{}, err
		}
		udpSrc, ok := src.(*net.UDPAddr)
		if !ok {
			continue
		}
		addr, ok := netip.AddrFromSlice(udpSrc.IP)
		if !ok {
			continue
		}
		out := make([]byte, n)
		copy(out, buf[:n])
		return out, addr.Unmap(), nil
	}
}

func (m *multicastTransport) Close() error {
	return m.conn.Close()
}
