package download

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"typhon/internal/account"
	"typhon/internal/settings"
)

func (s *memStore) snapshot() (account.Credential, bool, []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cred, s.present, append([]string(nil), s.users...)
}

func (r *netRig) workingProxyFor(t *testing.T, user, pass string) *client {
	t.Helper()
	r.store.present, r.store.cred = true, account.Credential{Token: pass, Username: user}
	r.followSettings(t)
	r.reconcile(t)
	cl := r.client()
	if cl == nil {
		t.Fatalf("no client to start from: %+v", r.state())
	}
	return cl
}

func (r *netRig) restart(t *testing.T) *netRig {
	t.Helper()
	m, err := newManagerAt(t.TempDir(), r.svc)
	if err != nil {
		t.Fatalf("restart manager: %v", err)
	}
	closePieceCompletionOnCleanup(t, m)
	withTestContext(t, m)
	next := &netRig{m: m, svc: r.svc, net: r.net, builds: &buildLog{}, store: r.store}
	m.netEnv = netEnv{interfaces: r.net.interfaces, probe: r.net.probe, dns: r.net.dnsOf, hostCheck: r.net.hostCheck}
	m.buildClient = next.builds.build
	m.proxyStore = r.store
	t.Cleanup(m.teardownClient)
	return next
}

func TestRejectedSettingsLeaveTheStoredPasswordOfTheWorkingLoginAlone(t *testing.T) {
	rejected := []struct {
		name  string
		patch func(*settings.Settings)
	}{
		{"host that is not a host", func(s *settings.Settings) { s.ProxyHost = "not a host" }},
		{"port out of range", func(s *settings.Settings) { s.ProxyPort = 70000 }},
		{"login with a colon", func(s *settings.Settings) { s.ProxyUsername = "a:b" }},
	}
	for _, tc := range rejected {
		t.Run(tc.name, func(t *testing.T) {
			r := newNetRig(t, func(s *settings.Settings) { viaProxy(s); s.ProxyUsername = "alice" })
			first := r.workingProxyFor(t, "alice", "pw1")

			if err := r.m.SetProxyPassword("bob", "pw2"); err != nil {
				t.Fatalf("SetProxyPassword: %v", err)
			}
			next := r.svc.GetSettings()
			next.ProxyUsername = "bob"
			tc.patch(&next)
			if err := r.svc.SaveSettings(next); err == nil {
				t.Fatal("the settings were accepted, the case needs a rejected save")
			}

			cred, present, users := r.store.snapshot()
			if !present || cred.Username != "alice" || cred.Token != "pw1" || len(users) != 0 {
				t.Fatalf("store = %+v present=%v saved for %v: the password of the login that works was replaced before the settings were accepted", cred, present, users)
			}
			r.reconcile(t)
			if st := r.state(); st.State != NetworkOK || r.client() != first || clientClosed(first) {
				t.Fatalf("state = %+v: the working client was touched", st)
			}

			after := r.restart(t)
			after.reconcile(t)
			if st := after.state(); st.State != NetworkOK || st.Code != "" {
				t.Fatalf("after a restart state = %+v, want the proxy of alice back", st)
			}
			if p := after.builds.lastPlan(); p.puser != "alice" || p.ppass != "pw1" {
				t.Fatalf("after a restart plan = %+v", p)
			}
		})
	}
}

func TestPendingProxyPasswordReachesTheStoreWithTheAcceptedSettings(t *testing.T) {
	for _, typed := range []string{"bob", "  bob  "} {
		t.Run(fmt.Sprintf("typed %q", typed), func(t *testing.T) {
			r := newNetRig(t, nil)
			r.followSettings(t)
			r.reconcile(t)
			builtBefore := r.builds.built()

			if err := r.m.SetProxyPassword(typed, "hunter2"); err != nil {
				t.Fatalf("SetProxyPassword: %v", err)
			}
			if _, present, users := r.store.snapshot(); present || len(users) != 0 {
				t.Fatalf("store holds %v before any settings with that login were accepted", users)
			}
			if len(r.m.netKick) != 0 {
				t.Fatal("a password for a login that is not saved must not wake the monitor")
			}

			next := r.svc.GetSettings()
			viaProxy(&next)
			next.ProxyUsername = "bob"
			if err := r.svc.SaveSettings(next); err != nil {
				t.Fatal(err)
			}
			cred, present, _ := r.store.snapshot()
			if !present || cred.Username != "bob" || cred.Token != "hunter2" {
				t.Fatalf("store = %+v present=%v after the settings were accepted", cred, present)
			}
			r.reconcile(t)
			if st := r.state(); st.State != NetworkOK || st.Mode != settings.NetworkProxy {
				t.Fatalf("state = %+v, want the proxy up on the first Apply", st)
			}
			if p := r.builds.lastPlan(); p.puser != "bob" || p.ppass != "hunter2" || r.builds.built() != builtBefore+1 {
				t.Fatalf("plan = %+v, builds = %d", p, r.builds.built())
			}
		})
	}
}

func TestPendingProxyPasswordFailingToReachTheStoreTakesTheNetworkDown(t *testing.T) {
	r := newNetRig(t, nil)
	r.followSettings(t)
	r.reconcile(t)
	built := r.builds.built()

	if err := r.m.SetProxyPassword("bob", "hunter2"); err != nil {
		t.Fatal(err)
	}
	r.store.saveErr = errors.New("keychain locked")
	next := r.svc.GetSettings()
	viaProxy(&next)
	next.ProxyUsername = "bob"
	if err := r.svc.SaveSettings(next); err != nil {
		t.Fatal(err)
	}
	r.reconcile(t)
	if st := r.state(); st.State != NetworkDown || st.Code != "download.proxy_credentials_failed" {
		t.Fatalf("state = %+v, want down with the credentials code", st)
	}
	if r.builds.built() != built {
		t.Fatal("a proxy client was built without the password the user entered")
	}

	r.store.saveErr = nil
	r.reconcile(t)
	if st := r.state(); st.State != NetworkOK {
		t.Fatalf("state = %+v: the password that did not reach the store must be tried again", st)
	}
	if p := r.builds.lastPlan(); p.puser != "bob" || p.ppass != "hunter2" {
		t.Fatalf("plan = %+v", p)
	}
	if cred, _, _ := r.store.snapshot(); cred.Username != "bob" || cred.Token != "hunter2" {
		t.Fatalf("store = %+v", cred)
	}
}

func TestPendingProxyPasswordNeverLandsUnderAnotherLogin(t *testing.T) {
	r := newNetRig(t, nil)
	r.followSettings(t)
	r.reconcile(t)

	if err := r.m.SetProxyPassword("bob", "hunter2"); err != nil {
		t.Fatal(err)
	}
	next := r.svc.GetSettings()
	viaProxy(&next)
	next.ProxyUsername = "carol"
	if err := r.svc.SaveSettings(next); err != nil {
		t.Fatal(err)
	}
	r.reconcile(t)
	if cred, present, users := r.store.snapshot(); present || len(users) != 0 {
		t.Fatalf("store = %+v saved for %v: the password typed for bob was kept for carol", cred, users)
	}
	if p := r.builds.lastPlan(); p.puser != "carol" || p.ppass != "" {
		t.Fatalf("plan = %+v", p)
	}

	r.saveProxyUser(t, "bob")
	r.reconcile(t)
	if cred, present, users := r.store.snapshot(); present || len(users) != 0 {
		t.Fatalf("store = %+v saved for %v: the password of an abandoned entry came back with its login", cred, users)
	}
	if p := r.builds.lastPlan(); p.puser != "bob" || p.ppass != "" {
		t.Fatalf("plan = %+v", p)
	}
}

func TestPendingProxyPasswordSurvivesSavesThatKeepTheLogin(t *testing.T) {
	r := newNetRig(t, func(s *settings.Settings) { viaProxy(s); s.ProxyUsername = "alice" })
	r.workingProxyFor(t, "alice", "pw1")

	if err := r.m.SetProxyPassword("bob", "pw2"); err != nil {
		t.Fatal(err)
	}
	next := r.svc.GetSettings()
	next.Theme = "light"
	if err := r.svc.SaveSettings(next); err != nil {
		t.Fatal(err)
	}
	r.saveProxyUser(t, "bob")
	cred, _, _ := r.store.snapshot()
	if cred.Username != "bob" || cred.Token != "pw2" {
		t.Fatalf("store = %+v: an unrelated save in between lost the password", cred)
	}
}

func TestPendingProxyPasswordLifecycle(t *testing.T) {
	cases := []struct {
		name string
		do   func(t *testing.T, r *netRig)
		want account.Credential
	}{
		{"the last call replaces the earlier one", func(t *testing.T, r *netRig) {
			for _, pass := range []string{"first", "second"} {
				if err := r.m.SetProxyPassword("bob", pass); err != nil {
					t.Fatal(err)
				}
			}
			r.saveProxyUser(t, "bob")
		}, account.Credential{Username: "bob", Token: "second"}},
		{"a password for the saved login drops the waiting one", func(t *testing.T, r *netRig) {
			if err := r.m.SetProxyPassword("bob", "waiting"); err != nil {
				t.Fatal(err)
			}
			if err := r.m.SetProxyPassword("alice", "fresh"); err != nil {
				t.Fatal(err)
			}
			r.saveProxyUser(t, "bob")
		}, account.Credential{Username: "alice", Token: "fresh"}},
		{"an empty password drops the waiting one", func(t *testing.T, r *netRig) {
			if err := r.m.SetProxyPassword("bob", "waiting"); err != nil {
				t.Fatal(err)
			}
			if err := r.m.SetProxyPassword("alice", ""); err != nil {
				t.Fatal(err)
			}
			r.saveProxyUser(t, "bob")
		}, account.Credential{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newNetRig(t, func(s *settings.Settings) { viaProxy(s); s.ProxyUsername = "alice" })
			r.workingProxyFor(t, "alice", "pw1")
			tc.do(t, r)
			cred, _, _ := r.store.snapshot()
			if cred != tc.want {
				t.Fatalf("store = %+v, want %+v", cred, tc.want)
			}
		})
	}
}

func TestSetProxyPasswordRefusesALoginTheSettingsWouldRefuse(t *testing.T) {
	cases := []struct {
		name  string
		login string
	}{
		{"colon", "a:b"},
		{"newline", "a\nb"},
		{"too long", strings.Repeat("u", 256)},
		{"invalid utf-8", "a\xffb"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newNetRig(t, func(s *settings.Settings) { viaProxy(s); s.ProxyUsername = "alice" })
			r.workingProxyFor(t, "alice", "pw1")

			err := r.m.SetProxyPassword(tc.login, "pw2")
			if !errors.Is(err, settings.ErrProxyUsernameInvalid) {
				t.Fatalf("SetProxyPassword(%q) = %v, want the error the settings give", tc.login, err)
			}
			cred, present, users := r.store.snapshot()
			if !present || cred.Username != "alice" || cred.Token != "pw1" || len(users) != 0 || r.store.deleted != 0 {
				t.Fatalf("store = %+v present=%v saved for %v: a refused login touched the store", cred, present, users)
			}
			if len(r.m.netKick) != 0 {
				t.Fatal("a refused login must not wake the monitor")
			}
		})
	}
}

func TestSetProxyPasswordAndApplySettingsRace(t *testing.T) {
	t.Run("a login that is never saved never reaches the store", func(t *testing.T) {
		r := newNetRig(t, func(s *settings.Settings) { viaProxy(s); s.ProxyUsername = "alice" })
		r.workingProxyFor(t, "alice", "pw1")

		const workers, rounds = 8, 40
		var wg sync.WaitGroup
		for w := range workers {
			wg.Add(2)
			go func() {
				defer wg.Done()
				for i := range rounds {
					if err := r.m.SetProxyPassword(fmt.Sprintf("stranger%d", w), fmt.Sprintf("pw%d", i)); err != nil {
						t.Error(err)
						return
					}
				}
			}()
			go func() {
				defer wg.Done()
				for i := range rounds {
					next := r.svc.GetSettings()
					next.DownloadRateLimit = int64(i + 1)
					if err := r.svc.SaveSettings(next); err != nil {
						t.Error(err)
						return
					}
				}
			}()
		}
		wg.Wait()

		cred, _, users := r.store.snapshot()
		if len(users) != 0 || cred.Username != "alice" || cred.Token != "pw1" {
			t.Fatalf("store = %+v, %d saves: a login the settings never carried got its password stored", cred, len(users))
		}
	})

	t.Run("a password set while its login is being saved is not lost", func(t *testing.T) {
		r := newNetRig(t, func(s *settings.Settings) { viaProxy(s); s.ProxyUsername = "alice" })
		r.followSettings(t)

		for i := range 60 {
			r.saveProxyUser(t, "alice")
			pass := fmt.Sprintf("pw%d", i)
			var wg sync.WaitGroup
			wg.Add(2)
			go func() {
				defer wg.Done()
				if err := r.m.SetProxyPassword("bob", pass); err != nil {
					t.Error(err)
				}
			}()
			go func() {
				defer wg.Done()
				next := r.svc.GetSettings()
				next.ProxyUsername = "bob"
				if err := r.svc.SaveSettings(next); err != nil {
					t.Error(err)
				}
			}()
			wg.Wait()

			if has, err := r.m.HasProxyPassword(); err != nil || !has {
				t.Fatalf("round %d: HasProxyPassword = %v, %v: the password was lost between the two calls", i, has, err)
			}
			if cred, _, _ := r.store.snapshot(); cred.Username != "bob" || cred.Token != pass {
				t.Fatalf("round %d: store = %+v, want bob/%s", i, cred, pass)
			}
		}
	})
}
