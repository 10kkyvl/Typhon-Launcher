package instance

import (
	"context"
	"errors"
	"testing"
)

type stubListener struct{ closed bool }

func (l *stubListener) Close() error { l.closed = true; return nil }

func TestTakeoverClaimsTheEarlyMutexThenListens(t *testing.T) {
	sys := &fakeAPI{first: true}
	want := &stubListener{}
	var listenedID string
	listen := func(_ context.Context, id string, _ func([]string)) (Listener, error) {
		listenedID = id
		return want, nil
	}
	got, err := takeover(t.Context(), sys, listen, "abc", func([]string) {})
	if err != nil {
		t.Fatalf("takeover() error = %v", err)
	}
	if got != want || listenedID != "abc" {
		t.Fatalf("takeover() listener = %v id %q, want the listener for abc", got, listenedID)
	}
	if len(sys.asked) != 1 || sys.asked[0] != "typhon-abc-early" {
		t.Fatalf("claimed %v, want only typhon-abc-early", sys.asked)
	}
}

func TestTakeoverRefusesWhenAnotherLauncherIsFirst(t *testing.T) {
	sys := &fakeAPI{first: false}
	listen := func(context.Context, string, func([]string)) (Listener, error) {
		t.Fatal("listened although the role was taken")
		return nil, nil
	}
	if _, err := takeover(t.Context(), sys, listen, "abc", func([]string) {}); !errors.Is(err, ErrNotFirst) {
		t.Fatalf("takeover() error = %v, want ErrNotFirst", err)
	}
}

func TestTakeoverReportsClaimAndListenFailures(t *testing.T) {
	claimErr := errors.New("claim failed")
	if _, err := takeover(t.Context(), &fakeAPI{claimErr: claimErr}, nil, "abc", nil); !errors.Is(err, claimErr) {
		t.Fatalf("takeover() error = %v, want the claim error", err)
	}
	listenErr := errors.New("listen failed")
	listen := func(context.Context, string, func([]string)) (Listener, error) { return nil, listenErr }
	if _, err := takeover(t.Context(), &fakeAPI{first: true}, listen, "abc", nil); !errors.Is(err, listenErr) {
		t.Fatalf("takeover() error = %v, want the listen error", err)
	}
	if _, err := takeover(t.Context(), &fakeAPI{first: true}, listen, "", nil); !errors.Is(err, ErrNoID) {
		t.Fatalf("takeover() with no id error = %v, want ErrNoID", err)
	}
}
