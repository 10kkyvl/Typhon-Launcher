package heartbeat

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"typhon/internal/library"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func decodeKeys(t *testing.T, body []byte) map[string]json.RawMessage {
	t.Helper()
	var m map[string]json.RawMessage
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	return m
}

func sortedKeys(m map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func takeRequest(t *testing.T, ch chan capturedRequest) capturedRequest {
	t.Helper()
	select {
	case req := <-ch:
		return req
	default:
		t.Fatal("expected a request, server saw none")
		return capturedRequest{}
	}
}

func assertNoRequests(t *testing.T, ch chan capturedRequest) {
	t.Helper()
	select {
	case req := <-ch:
		t.Fatalf("expected no requests, got %s: %s", req.path, req.body)
	default:
	}
}

func waitHeartbeat(t *testing.T, ch chan capturedRequest, match func(map[string]json.RawMessage) bool) map[string]json.RawMessage {
	t.Helper()
	for {
		req := waitFor(t, ch, "/presence/heartbeat")
		m := decodeKeys(t, req.body)
		if match(m) {
			return m
		}
	}
}

func waitKickConsumed(t *testing.T, svc *Service) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for len(svc.kick) != 0 {
		if time.Now().After(deadline) {
			t.Fatal("loop did not consume the kick")
		}
		runtime.Gosched()
	}
}

func TestSendPayloadKeys(t *testing.T) {
	base := []string{"app_version", "arch", "os", "session_id", "state"}
	identified := []string{"app_version", "arch", "game_id", "installation_id", "os", "session_id", "state"}
	tests := []struct {
		name     string
		policy   Policy
		playing  bool
		wantKeys []string
		wantGame string
		wantNull bool
	}{
		{name: "anonymous_idle", policy: Policy{Enabled: true}, wantKeys: base},
		{name: "anonymous_playing", policy: Policy{Enabled: true}, playing: true, wantKeys: base},
		{name: "identified_idle", policy: Policy{Enabled: true, Identified: true}, wantKeys: identified, wantNull: true},
		{name: "identified_playing", policy: Policy{Enabled: true, Identified: true}, playing: true, wantKeys: identified, wantGame: "42"},
		{name: "disabled_idle", policy: Policy{}},
		{name: "disabled_playing", policy: Policy{}, playing: true},
		{name: "disabled_identified_idle", policy: Policy{Identified: true}},
		{name: "disabled_identified_playing", policy: Policy{Identified: true}, playing: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv, reqs := newCapturingServer(t, http.StatusNoContent)
			svc := newBareService(t, srv, func(id string) string { return id })
			svc.SetPolicy(tc.policy)
			if tc.playing {
				svc.SessionStarted(library.Game{ID: "g1", CanonicalGameID: "42"})
			}

			svc.send(context.Background())

			if !tc.policy.Enabled {
				assertNoRequests(t, reqs)
				return
			}
			req := takeRequest(t, reqs)
			if !strings.HasSuffix(req.path, "/presence/heartbeat") {
				t.Fatalf("unexpected path %q", req.path)
			}
			m := decodeKeys(t, req.body)
			if got := sortedKeys(m); !reflect.DeepEqual(got, tc.wantKeys) {
				t.Fatalf("keys = %v, want %v (body %s)", got, tc.wantKeys, req.body)
			}
			wantState := stateIdle
			if tc.playing {
				wantState = statePlaying
			}
			if string(m["state"]) != `"`+wantState+`"` {
				t.Fatalf("state = %s, want %q", m["state"], wantState)
			}
			if tc.policy.Identified {
				if string(m["installation_id"]) != `"`+svc.identity.InstallationID+`"` {
					t.Fatalf("installation_id = %s", m["installation_id"])
				}
				switch {
				case tc.wantNull && string(m["game_id"]) != "null":
					t.Fatalf("game_id = %s, want null", m["game_id"])
				case tc.wantGame != "" && string(m["game_id"]) != `"`+tc.wantGame+`"`:
					t.Fatalf("game_id = %s, want %q", m["game_id"], tc.wantGame)
				}
			}
		})
	}
}

func TestDisabledPolicySendsNothing(t *testing.T) {
	tests := []struct {
		name   string
		set    bool
		policy Policy
	}{
		{name: "zero_value_never_set"},
		{name: "explicit_zero", set: true},
		{name: "disabled_anonymous", set: true, policy: Policy{Identified: false}},
		{name: "disabled_identified", set: true, policy: Policy{Identified: true}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv, reqs := newCapturingServer(t, http.StatusNoContent)
			svc := newBareService(t, srv, func(id string) string { return id })
			svc.interval = time.Millisecond
			if tc.set {
				svc.SetPolicy(tc.policy)
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if err := svc.ServiceStartup(ctx, application.ServiceOptions{}); err != nil {
				t.Fatalf("ServiceStartup: %v", err)
			}
			svc.SessionStarted(library.Game{ID: "g1", CanonicalGameID: "42"})
			waitKickConsumed(t, svc)
			svc.send(ctx)
			svc.SessionStopped("g1")
			waitKickConsumed(t, svc)
			svc.send(ctx)
			if err := svc.ServiceShutdown(); err != nil {
				t.Fatalf("ServiceShutdown: %v", err)
			}

			assertNoRequests(t, reqs)
		})
	}
}

func TestPolicySwitchesOnTheFly(t *testing.T) {
	srv, reqs := newCapturingServer(t, http.StatusNoContent)
	svc := newBareService(t, srv, func(id string) string { return id })
	svc.SetPolicy(Policy{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := svc.ServiceStartup(ctx, application.ServiceOptions{}); err != nil {
		t.Fatalf("ServiceStartup: %v", err)
	}
	svc.send(ctx)
	assertNoRequests(t, reqs)

	svc.SetPolicy(Policy{Enabled: true})
	anon := waitHeartbeat(t, reqs, func(map[string]json.RawMessage) bool { return true })
	if _, ok := anon["installation_id"]; ok {
		t.Fatalf("anonymous heartbeat carries installation_id: %v", sortedKeys(anon))
	}
	if _, ok := anon["game_id"]; ok {
		t.Fatalf("anonymous heartbeat carries game_id: %v", sortedKeys(anon))
	}

	svc.SessionStarted(library.Game{ID: "g1", CanonicalGameID: "42"})
	svc.SetPolicy(Policy{Enabled: true, Identified: true})
	ident := waitHeartbeat(t, reqs, func(m map[string]json.RawMessage) bool {
		_, ok := m["installation_id"]
		return ok && string(m["state"]) == `"playing"`
	})
	if string(ident["installation_id"]) != `"`+svc.identity.InstallationID+`"` || string(ident["game_id"]) != `"42"` {
		t.Fatalf("unexpected identified heartbeat: %v", ident)
	}

	svc.SetPolicy(Policy{})
	svc.SessionStopped("g1")
	svc.send(ctx)
	if err := svc.ServiceShutdown(); err != nil {
		t.Fatalf("ServiceShutdown: %v", err)
	}

	for {
		select {
		case req := <-reqs:
			if strings.HasSuffix(req.path, "/presence/disconnect") {
				t.Fatalf("disconnect sent while disabled: %s", req.body)
			}
			if m := decodeKeys(t, req.body); string(m["state"]) == `"idle"` {
				t.Fatalf("heartbeat sent after disabling: %s", req.body)
			}
		default:
			return
		}
	}
}

func TestSetPolicyPokesOnlyOnEnabledChange(t *testing.T) {
	srv, _ := newCapturingServer(t, http.StatusNoContent)
	svc := newBareService(t, srv, nil)
	svc.mu.Lock()
	svc.cancel = func() {}
	svc.mu.Unlock()

	tests := []struct {
		name     string
		policy   Policy
		wantKick bool
	}{
		{name: "zero_to_zero", policy: Policy{}},
		{name: "enable", policy: Policy{Enabled: true}, wantKick: true},
		{name: "same_policy", policy: Policy{Enabled: true}},
		{name: "identify", policy: Policy{Enabled: true, Identified: true}, wantKick: true},
		{name: "disable", policy: Policy{Identified: true}},
	}
	for _, tc := range tests {
		svc.SetPolicy(tc.policy)
		got := len(svc.kick) == 1
		if got != tc.wantKick {
			t.Fatalf("%s: kick = %v, want %v", tc.name, got, tc.wantKick)
		}
		select {
		case <-svc.kick:
		default:
		}
	}
}

func TestDisconnectPayloadByPolicy(t *testing.T) {
	tests := []struct {
		name     string
		policy   Policy
		wantKeys []string
	}{
		{name: "anonymous", policy: Policy{Enabled: true}, wantKeys: []string{"session_id"}},
		{name: "identified", policy: Policy{Enabled: true, Identified: true}, wantKeys: []string{"installation_id", "session_id"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv, reqs := newCapturingServer(t, http.StatusNoContent)
			svc := newBareService(t, srv, nil)
			svc.SetPolicy(tc.policy)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if err := svc.ServiceStartup(ctx, application.ServiceOptions{}); err != nil {
				t.Fatalf("ServiceStartup: %v", err)
			}
			waitFor(t, reqs, "/presence/heartbeat")
			if err := svc.ServiceShutdown(); err != nil {
				t.Fatalf("ServiceShutdown: %v", err)
			}

			req := waitFor(t, reqs, "/presence/disconnect")
			m := decodeKeys(t, req.body)
			if got := sortedKeys(m); !reflect.DeepEqual(got, tc.wantKeys) {
				t.Fatalf("disconnect keys = %v, want %v", got, tc.wantKeys)
			}
			if string(m["session_id"]) != `"`+svc.identity.SessionID+`"` {
				t.Fatalf("session_id = %s", m["session_id"])
			}
		})
	}
}
