package lan

import (
	"encoding/json"
	"errors"
	"net/netip"
	"strings"
	"testing"
	"time"
)

var (
	testSelfID = strings.Repeat("0", 32)
	testPeerID = strings.Repeat("1", 32)
	testHash40 = strings.Repeat("2", 40)
)

func validAnnounceMsg(now time.Time) announceMsg {
	return announceMsg{
		V:        1,
		ID:       testPeerID,
		Host:     "desk-1",
		Port:     42817,
		GameID:   "game-1",
		Title:    "Some Game",
		Version:  "1.0",
		Exe:      "bin/game.exe",
		Size:     1024,
		InfoHash: testHash40,
		TS:       now.Unix(),
	}
}

func marshalMsg(t *testing.T, msg announceMsg) []byte {
	t.Helper()
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return data
}

func TestAnnounceRejects(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	privateAddr := netip.MustParseAddr("192.168.1.50")
	publicAddr := netip.MustParseAddr("8.8.8.8")

	cases := []struct {
		name    string
		raw     func(t *testing.T) []byte
		srcAddr netip.Addr
		wantErr error
	}{
		{
			name: "oversized datagram",
			raw: func(t *testing.T) []byte {
				return []byte(strings.Repeat("a", 2000))
			},
			srcAddr: privateAddr,
			wantErr: errAnnounceTooLarge,
		},
		{
			name: "unsupported version",
			raw: func(t *testing.T) []byte {
				m := validAnnounceMsg(now)
				m.V = 2
				return marshalMsg(t, m)
			},
			srcAddr: privateAddr,
			wantErr: errAnnounceVersion,
		},
		{
			name: "unknown field",
			raw: func(t *testing.T) []byte {
				type withExtra struct {
					announceMsg
					Extra string `json:"extra"`
				}
				data, err := json.Marshal(withExtra{announceMsg: validAnnounceMsg(now), Extra: "x"})
				if err != nil {
					t.Fatalf("marshal: %v", err)
				}
				return data
			},
			srcAddr: privateAddr,
			wantErr: errAnnounceBadJSON,
		},
		{
			name: "not json",
			raw: func(t *testing.T) []byte {
				return []byte("not json at all")
			},
			srcAddr: privateAddr,
			wantErr: errAnnounceBadJSON,
		},
		{
			name: "bad id",
			raw: func(t *testing.T) []byte {
				m := validAnnounceMsg(now)
				m.ID = "not-hex"
				return marshalMsg(t, m)
			},
			srcAddr: privateAddr,
			wantErr: errAnnounceID,
		},
		{
			name: "own id",
			raw: func(t *testing.T) []byte {
				m := validAnnounceMsg(now)
				m.ID = testSelfID
				return marshalMsg(t, m)
			},
			srcAddr: privateAddr,
			wantErr: errAnnounceOwnID,
		},
		{
			name: "bad infohash",
			raw: func(t *testing.T) []byte {
				m := validAnnounceMsg(now)
				m.InfoHash = "zz"
				return marshalMsg(t, m)
			},
			srcAddr: privateAddr,
			wantErr: errAnnounceInfoHash,
		},
		{
			name: "exe with dotdot",
			raw: func(t *testing.T) []byte {
				m := validAnnounceMsg(now)
				m.Exe = "../evil.exe"
				return marshalMsg(t, m)
			},
			srcAddr: privateAddr,
			wantErr: errAnnounceExe,
		},
		{
			name: "exe absolute",
			raw: func(t *testing.T) []byte {
				m := validAnnounceMsg(now)
				m.Exe = "/etc/passwd"
				return marshalMsg(t, m)
			},
			srcAddr: privateAddr,
			wantErr: errAnnounceExe,
		},
		{
			name: "exe drive letter",
			raw: func(t *testing.T) []byte {
				m := validAnnounceMsg(now)
				m.Exe = `C:\Windows\System32\evil.exe`
				return marshalMsg(t, m)
			},
			srcAddr: privateAddr,
			wantErr: errAnnounceExe,
		},
		{
			name: "negative size",
			raw: func(t *testing.T) []byte {
				m := validAnnounceMsg(now)
				m.Size = -1
				return marshalMsg(t, m)
			},
			srcAddr: privateAddr,
			wantErr: errAnnounceSize,
		},
		{
			name: "size too large",
			raw: func(t *testing.T) []byte {
				m := validAnnounceMsg(now)
				m.Size = (1 << 48) + 1
				return marshalMsg(t, m)
			},
			srcAddr: privateAddr,
			wantErr: errAnnounceSize,
		},
		{
			name: "timestamp an hour old",
			raw: func(t *testing.T) []byte {
				m := validAnnounceMsg(now.Add(-time.Hour))
				return marshalMsg(t, m)
			},
			srcAddr: privateAddr,
			wantErr: errAnnounceTimestamp,
		},
		{
			name: "public source address",
			raw: func(t *testing.T) []byte {
				return marshalMsg(t, validAnnounceMsg(now))
			},
			srcAddr: publicAddr,
			wantErr: errAnnounceSourceAddr,
		},
		{
			name: "empty title",
			raw: func(t *testing.T) []byte {
				m := validAnnounceMsg(now)
				m.Title = ""
				return marshalMsg(t, m)
			},
			srcAddr: privateAddr,
			wantErr: errAnnounceTitle,
		},
		{
			name: "title too long",
			raw: func(t *testing.T) []byte {
				m := validAnnounceMsg(now)
				m.Title = strings.Repeat("x", 513)
				return marshalMsg(t, m)
			},
			srcAddr: privateAddr,
			wantErr: errAnnounceTitle,
		},
	}

	for _, tc := range cases {
		_, err := decodeAnnounce(tc.raw(t), tc.srcAddr, testSelfID, now)
		if !errors.Is(err, tc.wantErr) {
			t.Errorf("%s: decodeAnnounce error = %v, want %v", tc.name, err, tc.wantErr)
		}
	}
}

func TestAnnounceAcceptsValid(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	msg, err := decodeAnnounce(marshalMsg(t, validAnnounceMsg(now)), netip.MustParseAddr("10.0.0.5"), testSelfID, now)
	if err != nil {
		t.Fatalf("decodeAnnounce: %v", err)
	}
	if msg.ID != testPeerID {
		t.Fatalf("ID = %q, want %q", msg.ID, testPeerID)
	}
}

func TestAnnounceRateLimit(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	addr := netip.MustParseAddr("192.168.1.7")
	l := newSourceLimiter()

	for i := 0; i < 20; i++ {
		if !l.allow(addr, now) {
			t.Fatalf("datagram %d unexpectedly rate-limited", i+1)
		}
	}
	if l.allow(addr, now) {
		t.Fatal("21st datagram within the window should be rejected")
	}

	other := netip.MustParseAddr("192.168.1.8")
	if !l.allow(other, now) {
		t.Fatal("a different source IP must have its own budget")
	}
}

func TestPeerExpiry(t *testing.T) {
	table := newPeerTable()
	start := time.Unix(1_800_000_000, 0)
	msg := validAnnounceMsg(start)
	if !table.observe(msg, netip.MustParseAddr("192.168.1.9"), start) {
		t.Fatal("observe rejected a fresh, well within capacity announce")
	}

	if peers := table.list(start.Add(peerTTL - time.Second)); len(peers) != 1 {
		t.Fatalf("peers just under TTL = %d, want 1", len(peers))
	}
	if offers := table.available(start.Add(peerTTL - time.Second)); len(offers) != 1 {
		t.Fatalf("offers just under TTL = %d, want 1", len(offers))
	}

	after := start.Add(peerTTL + time.Second)
	if peers := table.list(after); len(peers) != 0 {
		t.Fatalf("peers after TTL = %d, want 0", len(peers))
	}
	if offers := table.available(after); len(offers) != 0 {
		t.Fatalf("offers after TTL = %d, want 0", len(offers))
	}
}

func TestPeerTableCapacity(t *testing.T) {
	table := newPeerTable()
	now := time.Unix(1_800_000_000, 0)
	for i := 0; i < maxPeers; i++ {
		msg := validAnnounceMsg(now)
		msg.ID = hexID(byte(i))
		if !table.observe(msg, netip.MustParseAddr("192.168.1.9"), now) {
			t.Fatalf("peer %d unexpectedly rejected for capacity", i)
		}
	}
	msg := validAnnounceMsg(now)
	msg.ID = hexID(byte(maxPeers))
	if table.observe(msg, netip.MustParseAddr("192.168.1.9"), now) {
		t.Fatal("peer beyond maxPeers must be rejected")
	}
}

func hexID(b byte) string {
	const hexDigits = "0123456789abcdef"
	out := make([]byte, 32)
	for i := range out {
		out[i] = hexDigits[b%16]
	}
	// Make each id unique despite the repeating fill above.
	out[0] = hexDigits[(b>>4)%16]
	out[1] = hexDigits[b%16]
	return string(out)
}
