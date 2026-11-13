package cloudsaves

import (
	"errors"
	"fmt"
	"testing"

	"typhon/internal/account"
	"typhon/internal/uierr"
)

// The player's own requests reach the dialog as a code: an unreachable cloud
// reads "cannot reach the Typhon server", not the generic fallback, and the
// transport's error stays the cause.
func TestOperationsAnswerAnUnreachableCloudWithTheNetworkCode(t *testing.T) {
	unreachable := func() error {
		return fmt.Errorf("test transport: %w", &account.Error{Code: account.CodeNetwork})
	}
	for _, tc := range []struct {
		name string
		arm  func(h *harness)
		op   func(h *harness) error
	}{
		{"ResolveConflict keeping mine", func(h *harness) { h.client.listErr = unreachable() }, func(h *harness) error { return h.svc.ResolveConflict(gameLocal, true) }},
		{"ResolveConflict taking the cloud copy", func(h *harness) { h.client.listErr = unreachable() }, func(h *harness) error { return h.svc.ResolveConflict(gameLocal, false) }},
		{"SyncNow", func(h *harness) { h.client.listErr = unreachable() }, func(h *harness) error { return h.svc.SyncNow(gameLocal) }},
		{"PullRemote", func(h *harness) { h.client.listErr = unreachable() }, func(h *harness) error { return h.svc.PullRemote(gameLocal) }},
		{"DeleteRemote", func(h *harness) { h.client.deleteErr[gameCloud] = unreachable() }, func(h *harness) error { return h.svc.DeleteRemote(gameLocal) }},
		{"Forget", func(h *harness) { h.client.listErr = unreachable() }, func(h *harness) error { return h.svc.Forget() }},
		{"SyncNow failing in the upload", func(h *harness) {
			if err := h.svc.ResolveConflict(gameLocal, true); err != nil {
				h.t.Fatalf("setup: resolve the conflict: %v", err)
			}
			writeTree(h.t, h.saves(), map[string]string{"slot1.sav": "one, played on"})
			h.client.uploadErr = unreachable()
		}, func(h *harness) error { return h.svc.SyncNow(gameLocal) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			h.recordConflict()
			h.open()
			before := h.stateBytes()
			tc.arm(h)

			err := tc.op(h)

			if code := uierr.Code(err); code != codeNetwork {
				t.Fatalf("error = %v (code %q), want %s", err, code, codeNetwork)
			}
			var api *account.Error
			if !errors.As(err, &api) || api.Code != account.CodeNetwork {
				t.Fatalf("error = %v, want the transport's account.Error kept as the cause", err)
			}
			if tc.name != "SyncNow failing in the upload" {
				if got := h.stateBytes(); string(got) != string(before) {
					t.Fatalf("state file changed:\n%s", got)
				}
				return
			}
			// The recorded outcome is the one a pass records: the code from
			// classify and the client's own message.
			if g := h.status(gameLocal); g.State != StateFailed || g.Code != codeNetwork || g.Message != "test transport: network_error" {
				t.Fatalf("recorded status = %+v, want failed %s with the client's message", g, codeNetwork)
			}
		})
	}
}

// Other errors of the account client keep the code a recorded outcome gets
// for them; the launcher's own codes pass through as they are.
func TestOperationsKeepTheMappingOfOtherCloudErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"session expired", &account.Error{Code: account.CodeUnauthenticated, Status: 401}, codeUnauthorized},
		{"rate limited", &account.Error{Code: account.CodeRateLimited, Status: 429}, codeRateLimited},
		{"server error", &account.Error{Code: account.CodeServer, Status: 500}, codeServer},
		{"busy", &account.Error{Code: account.CodeSaveBusy, Status: 503}, codeBusy},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			h.recordConflict()
			h.open()
			h.client.listErr = tc.err

			err := h.svc.ResolveConflict(gameLocal, true)

			if code := uierr.Code(err); code != tc.want {
				t.Fatalf("ResolveConflict = %v (code %q), want %s", err, code, tc.want)
			}
			if !errors.Is(err, tc.err) {
				t.Fatalf("ResolveConflict = %v, want the cause %v kept", err, tc.err)
			}
		})
	}
	t.Run("no conflict", func(t *testing.T) {
		h := newHarness(t)
		writeTree(t, h.saves(), baseFiles)
		h.pass()
		h.open()
		if err := h.svc.ResolveConflict(gameLocal, true); !errors.Is(err, errNoConflict) || uierr.Code(err) != codeNoConflict {
			t.Fatalf("ResolveConflict = %v, want %s", err, codeNoConflict)
		}
	})
}
