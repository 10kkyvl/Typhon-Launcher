package selfupdate

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"typhon/internal/account"
)

type betaStand struct {
	srv     *httptest.Server
	pub     ed25519.PublicKey
	content []byte

	mu      sync.Mutex
	header  http.Header
	channel string
	failing bool
}

func newBetaStand(t *testing.T, version, channel string) *betaStand {
	t.Helper()
	priv, pub := testKeyPair(t)
	st := &betaStand{pub: pub, content: bytes.Repeat([]byte("typhon installer "), 100), channel: channel}
	st.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		st.mu.Lock()
		content := bytes.Clone(st.content)
		st.mu.Unlock()
		if r.URL.Path != ManifestPath {
			if _, err := w.Write(content); err != nil {
				t.Errorf("write artifact: %v", err)
			}
			return
		}
		st.mu.Lock()
		st.header = r.Header.Clone()
		ch := st.channel
		failing := st.failing
		st.mu.Unlock()
		if failing {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		m := releaseManifest(version)
		art := &m.Artifacts[0]
		art.URL = st.srv.URL + "/artifact/" + art.Name
		art.Size = int64(len(bytes.Repeat([]byte("typhon installer "), 100)))
		art.SHA256 = hashOf(bytes.Repeat([]byte("typhon installer "), 100))
		if ch != "" {
			w.Header().Set(ChannelHeader, ch)
		}
		if _, err := w.Write(signManifest(t, priv, m)); err != nil {
			t.Errorf("write manifest: %v", err)
		}
	}))
	t.Cleanup(st.srv.Close)
	return st
}

func (st *betaStand) seen() http.Header {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.header
}

func (st *betaStand) service(t *testing.T, dir, current string, access BetaAccess) *Service {
	t.Helper()
	client, err := newClientWithKey(st.srv.URL, st.pub)
	if err != nil {
		t.Fatalf("newClientWithKey: %v", err)
	}
	return &Service{dir: dir, notes: mustNotesStore(t, dir), store: mustStore(t, dir), client: client, currentVersion: current, beta: access}
}

func (st *betaStand) artifactName() string {
	return releaseManifest("0.0.1").Artifacts[0].Name
}

func TestFetchManifestBetaHeaders(t *testing.T) {
	boom := errors.New("vault locked")
	tester := func() (string, bool, error) { return "tok-123", true, nil }
	cases := []struct {
		name        string
		access      BetaAccess
		serverChan  string
		wantHeaders bool
		wantChannel string
		wantErr     error
	}{
		{"no access configured", nil, "", false, ChannelStable, nil},
		{"opted out or not a tester", func() (string, bool, error) { return "", false, nil }, "stable", false, ChannelStable, nil},
		{"tester on beta", tester, "beta", true, ChannelBeta, nil},
		{"tester but server answers stable", tester, "stable", true, ChannelStable, nil},
		{"old server without the header", tester, "", true, ChannelStable, nil},
		{"unknown channel is stable", tester, "nightly", true, ChannelStable, nil},
		{"access failure stops the check", func() (string, bool, error) { return "", false, boom }, "", false, "", boom},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			st := newBetaStand(t, "1.2.0", c.serverChan)
			client, err := newClientWithKey(st.srv.URL, st.pub)
			if err != nil {
				t.Fatal(err)
			}
			_, channel, err := client.fetchManifest(context.Background(), c.access)
			if c.wantErr != nil {
				if !errors.Is(err, c.wantErr) {
					t.Fatalf("error = %v, want %v", err, c.wantErr)
				}
				if st.seen() != nil {
					t.Fatal("a request went out although beta access could not be resolved")
				}
				return
			}
			if err != nil {
				t.Fatalf("fetchManifest: %v", err)
			}
			if channel != c.wantChannel {
				t.Fatalf("channel = %q, want %q", channel, c.wantChannel)
			}
			h := st.seen()
			if c.wantHeaders {
				if h.Get(ChannelHeader) != ChannelBeta || h.Get("Authorization") != "Bearer tok-123" {
					t.Fatalf("headers = %v, want the beta channel and the bearer token", h)
				}
				return
			}
			if h.Get(ChannelHeader) != "" || h.Get("Authorization") != "" {
				t.Fatalf("headers = %v, want neither the channel nor a token", h)
			}
		})
	}
}

func TestFetchManifestBetaRefusesPlainHTTP(t *testing.T) {
	var mu sync.Mutex
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		mu.Lock()
		hits++
		mu.Unlock()
	}))
	defer srv.Close()
	c, err := NewClient(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	c.baseURL = "http://updates.example.com"
	access := func() (string, bool, error) { return "tok-123", true, nil }
	_, _, err = c.fetchManifest(context.Background(), access)
	if !errors.Is(err, account.ErrInsecureBaseURL) {
		t.Fatalf("error = %v, want ErrInsecureBaseURL", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if hits != 0 {
		t.Fatalf("%d requests reached the server, the token must not leave over http", hits)
	}
}

type captureHandler struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (h *captureHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.buf.WriteString(r.Message)
	r.Attrs(func(a slog.Attr) bool {
		h.buf.WriteString(" " + a.Key + "=" + a.Value.String())
		return true
	})
	return nil
}

func (h *captureHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(string) slog.Handler      { return h }

func TestBetaTokenIsNotLogged(t *testing.T) {
	logs := &captureHandler{}
	prev := slog.Default()
	slog.SetDefault(slog.New(logs))
	t.Cleanup(func() { slog.SetDefault(prev) })

	st := newBetaStand(t, "1.2.0", "beta")
	s := st.service(t, t.TempDir(), "1.0.0", func() (string, bool, error) { return "secret-token-xyz", true, nil })
	if _, err := s.CheckForUpdate(context.Background()); err != nil {
		t.Fatalf("CheckForUpdate: %v", err)
	}
	st.mu.Lock()
	st.failing = true
	st.mu.Unlock()
	if _, err := s.CheckForUpdate(context.Background()); err == nil {
		t.Fatal("want an error for the 500 answer")
	}
	logs.mu.Lock()
	defer logs.mu.Unlock()
	if strings.Contains(logs.buf.String(), "secret-token-xyz") {
		t.Fatalf("the session token reached the log: %s", logs.buf.String())
	}
}

func TestCheckReportsChannel(t *testing.T) {
	cases := []struct {
		name, serverChan, want string
	}{
		{"beta", "beta", ChannelBeta},
		{"stable", "stable", ChannelStable},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			st := newBetaStand(t, "1.3.0-beta.2", c.serverChan)
			s := st.service(t, t.TempDir(), "1.3.0-beta.1", func() (string, bool, error) { return "tok", true, nil })
			status, err := s.CheckForUpdate(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if status.Channel != c.want || status.State != StateAvailable || status.AvailableVersion != "1.3.0-beta.2" {
				t.Fatalf("status = %+v", status)
			}
		})
	}
}

func TestBetaBuildGetsStableOfferInsteadOfDowngrade(t *testing.T) {
	cases := []struct {
		name      string
		current   string
		manifest  string
		wantState State
		wantOffer string
	}{
		{"beta with an older stable offers it", "1.3.0-beta.1", "1.2.0", StateIdle, "1.2.0"},
		{"beta with the stable of the same base is an update", "1.3.0-beta.1", "1.3.0", StateAvailable, ""},
		{"beta with a newer stable is an update", "1.3.0-beta.1", "1.4.0", StateAvailable, ""},
		{"stable build never offers a downgrade", "1.3.0", "1.2.0", StateIdle, ""},
		{"older beta manifest is no offer", "1.3.0-beta.3", "1.3.0-beta.1", StateIdle, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			st := newBetaStand(t, c.manifest, "stable")
			s := st.service(t, t.TempDir(), c.current, nil)
			status, err := s.CheckForUpdate(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if status.State != c.wantState || status.StableOffer != c.wantOffer {
				t.Fatalf("status = %+v, want state %s and offer %q", status, c.wantState, c.wantOffer)
			}
			if c.wantOffer != "" {
				if s.pendingArtifact != nil {
					t.Fatal("the downgrade became a pending update that DownloadUpdate would install")
				}
				if _, err := s.DownloadUpdate(context.Background()); err == nil {
					t.Fatal("DownloadUpdate took the stable offer without InstallStable")
				}
			}
		})
	}
}

func TestInstallStable(t *testing.T) {
	st := newBetaStand(t, "1.2.0", "stable")
	dir := t.TempDir()
	started := captureWorkerStarts(t)
	s := st.service(t, dir, "1.3.0-beta.1", nil)

	if _, err := s.InstallStable(context.Background(), "en"); !errors.Is(err, ErrNoStableOffer) {
		t.Fatalf("InstallStable before a check = %v, want ErrNoStableOffer", err)
	}
	if s.busy {
		t.Fatal("service left busy after a refused InstallStable")
	}

	if _, err := s.CheckForUpdate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.InstallStable(context.Background(), "en"); err != nil {
		t.Fatalf("InstallStable: %v", err)
	}
	if len(started.specs) != 1 {
		t.Fatalf("worker started %d times, want 1", len(started.specs))
	}
	spec, err := readUpdateSpec(started.specs[0])
	if err != nil {
		t.Fatal(err)
	}
	wantPath, err := ArtifactPath(dir, "1.2.0", st.artifactName())
	if err != nil {
		t.Fatal(err)
	}
	if spec.Version != "1.2.0" || spec.FromVersion != "1.3.0-beta.1" || spec.InstallerPath != wantPath {
		t.Fatalf("worker spec = %+v, want the verified 1.2.0 installer", spec)
	}
}

func TestInstallStableRejectsTamperedDownload(t *testing.T) {
	st := newBetaStand(t, "1.2.0", "stable")
	started := captureWorkerStarts(t)
	s := st.service(t, t.TempDir(), "1.3.0-beta.1", nil)
	if _, err := s.CheckForUpdate(context.Background()); err != nil {
		t.Fatal(err)
	}
	st.mu.Lock()
	st.content[0] ^= 0xff
	st.mu.Unlock()
	if _, err := s.InstallStable(context.Background(), "en"); err == nil {
		t.Fatal("InstallStable installed an artifact whose hash differs from the signed manifest")
	}
	if len(started.specs) != 0 {
		t.Fatal("the worker started for a tampered artifact")
	}
	if s.busy {
		t.Fatal("service left busy after a failed InstallStable")
	}
}
