package support

import (
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"

	"typhon/internal/account"
)

// sessions stands in for the account service's token source: the signed-in
// account changes when a test switches it, the way a sign-out and a sign-in
// on another account would while a send is running.
type sessions struct {
	mu      sync.Mutex
	current string
	reads   int
}

func newSessions(token string) *sessions { return &sessions{current: token} }

func (s *sessions) token() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reads++
	return s.current, nil
}

func (s *sessions) switchTo(token string) {
	s.mu.Lock()
	s.current = token
	s.mu.Unlock()
}

func (s *sessions) readCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reads
}

// tokenFirstThen answers the current token to the first read and then to every
// later one.
func (s *sessions) tokenFirstThen(then string) func() (string, error) {
	return func() (string, error) {
		tok, err := s.token()
		s.switchTo(then)
		return tok, err
	}
}

func authorization(req seen) string {
	return req.header.Get("Authorization")
}

func sendWithLogs(r *rig, op string) error {
	if op == "create" {
		_, err := r.svc.Create(attachingInput())
		return err
	}
	_, err := r.svc.Reply(1042, "more", true)
	return err
}

func TestSendWithLogsIsRefusedWhenTheAccountChangedDuringTheUpload(t *testing.T) {
	for _, op := range []string{"create", "reply"} {
		t.Run(op, func(t *testing.T) {
			who := newSessions("token-A")
			up := &uploads{}
			r := newRig(t, rigOptions{
				token: who.token,
				uploadLogs: func() (string, error) {
					id, err := up.next()
					who.switchTo("token-B")
					return id, err
				},
			})
			r.start(t)

			err := sendWithLogs(r, op)
			if calls := r.api.requests(); len(calls) != 0 {
				t.Fatalf("account A's send was posted after account B signed in: %s %s with %q and bundle %q",
					calls[0].method, calls[0].path, authorization(calls[0]), sentBundle(t, r, 0))
			}
			if apiErr := apiCode(t, err); apiErr.Code != account.CodeUnauthenticated {
				t.Fatalf("code = %q, want %q", apiErr.Code, account.CodeUnauthenticated)
			}

			if err := sendWithLogs(r, op); err != nil {
				t.Fatalf("account B's own send: %v", err)
			}
			if got := up.n.Load(); got != 2 {
				t.Fatalf("uploads = %d, want account B's send to upload its own logs", got)
			}
			req := r.api.only()
			if got := authorization(req); got != "Bearer token-B" {
				t.Fatalf("account B's send carries %q", got)
			}
			if got := sentBundle(t, r, 0); got != "BUNDLE02" {
				t.Fatalf("account B's send carries bundle %q, want its own BUNDLE02", got)
			}
		})
	}
}

func TestRefusedSendDoesNotKeepItsBundleForItsOwnAccountEither(t *testing.T) {
	who := newSessions("token-A")
	up := &uploads{}
	first := true
	r := newRig(t, rigOptions{
		token: who.token,
		uploadLogs: func() (string, error) {
			id, err := up.next()
			if first {
				first = false
				who.switchTo("token-B")
			}
			return id, err
		},
	})
	r.start(t)

	err := sendWithLogs(r, "create")
	if calls := r.api.requests(); len(calls) != 0 {
		t.Fatalf("the send was posted with %q after the account changed", authorization(calls[0]))
	}
	if apiCode(t, err).Code != account.CodeUnauthenticated {
		t.Fatalf("first send error = %v", err)
	}
	who.switchTo("token-A")
	if err := sendWithLogs(r, "create"); err != nil {
		t.Fatalf("second send: %v", err)
	}
	if got := up.n.Load(); got != 2 {
		t.Fatalf("uploads = %d, want the refused send's bundle dropped", got)
	}
	if got := sentBundle(t, r, 0); got != "BUNDLE02" {
		t.Fatalf("send carries %q", got)
	}
}

func TestSessionUnreadableAtPostTimeSendsNothingAndKeepsTheBundle(t *testing.T) {
	broken := errors.New("keychain locked")
	var reads atomic.Int32
	up := &uploads{}
	r := newRig(t, rigOptions{
		uploadLogs: up.next,
		token: func() (string, error) {
			if reads.Add(1) == 2 {
				return "", broken
			}
			return "token-A", nil
		},
	})
	r.start(t)

	err := sendWithLogs(r, "create")
	if !errors.Is(err, broken) {
		t.Fatalf("error = %v, want the keychain failure", err)
	}
	if calls := r.api.requests(); len(calls) != 0 {
		t.Fatalf("%d requests went out without a confirmed session", len(calls))
	}
	if err := sendWithLogs(r, "create"); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if got := up.n.Load(); got != 1 {
		t.Fatalf("uploads = %d, want the same account's retry to reuse its bundle", got)
	}
	if got := authorization(r.api.only()); got != "Bearer token-A" {
		t.Fatalf("retry carries %q", got)
	}
}

func TestSendWithLogsUnderOneSessionIsPostedOnceWithItsToken(t *testing.T) {
	for _, op := range []string{"create", "reply"} {
		t.Run(op, func(t *testing.T) {
			who := newSessions("token-A")
			r := newRig(t, rigOptions{token: who.token})
			r.start(t)

			if err := sendWithLogs(r, op); err != nil {
				t.Fatalf("send: %v", err)
			}
			req := r.api.only()
			if req.method != http.MethodPost {
				t.Fatalf("request = %s %s", req.method, req.path)
			}
			if got := authorization(req); got != "Bearer token-A" {
				t.Fatalf("Authorization = %q", got)
			}
			if got := sentBundle(t, r, 0); got != "BUNDLE01" {
				t.Fatalf("logBundleId = %q", got)
			}
		})
	}
}

func TestSendWithoutLogsReadsTheSessionOnce(t *testing.T) {
	for _, op := range []string{"create", "reply"} {
		t.Run(op, func(t *testing.T) {
			who := newSessions("token-A")
			r := newRig(t, rigOptions{token: who.tokenFirstThen("token-B")})
			r.start(t)

			var err error
			if op == "create" {
				_, err = r.svc.Create(validInput())
			} else {
				_, err = r.svc.Reply(1042, "more", false)
			}
			if err != nil {
				t.Fatalf("send: %v", err)
			}
			req := r.api.only()
			if got := authorization(req); got != "Bearer token-A" {
				t.Fatalf("Authorization = %q, want the session the send started under", got)
			}
			if got := who.readCount(); got != 1 {
				t.Fatalf("session read %d times, want once", got)
			}
			if events := r.journal.list(); len(events) != 1 {
				t.Fatalf("events = %v, want no upload", events)
			}
		})
	}
}

func TestKeptBundleIsReusedByItsAccountOnlyAfterAccountsSwitch(t *testing.T) {
	who := newSessions("token-A")
	up := &uploads{}
	r := newRig(t, rigOptions{token: who.token, uploadLogs: up.next, status: http.StatusConflict, body: refusedLimit})
	r.start(t)

	if err := sendWithLogs(r, "create"); apiCode(t, err).Code != CodeTicketLimit {
		t.Fatalf("A's first send error = %v", err)
	}
	if err := sendWithLogs(r, "create"); apiCode(t, err).Code != CodeTicketLimit {
		t.Fatalf("A's retry error = %v", err)
	}
	if got := up.n.Load(); got != 1 {
		t.Fatalf("uploads after A's retry = %d, want A's bundle reused", got)
	}
	if got := sentBundle(t, r, 1); got != "BUNDLE01" {
		t.Fatalf("A's retry carries %q", got)
	}

	who.switchTo("token-B")
	r.resp.set(http.StatusCreated, acceptedBody)
	if err := sendWithLogs(r, "create"); err != nil {
		t.Fatalf("B's first send: %v", err)
	}
	if got := up.n.Load(); got != 2 {
		t.Fatalf("uploads = %d, want B's send to upload its own logs", got)
	}
	calls := r.api.requests()
	if got := authorization(calls[2]); got != "Bearer token-B" {
		t.Fatalf("B's send carries %q", got)
	}
	if got := sentBundle(t, r, 2); got != "BUNDLE02" {
		t.Fatalf("B's first send carries A's bundle: %q", got)
	}
}
