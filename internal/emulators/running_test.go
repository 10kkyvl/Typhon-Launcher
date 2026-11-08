package emulators

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeCopies is a process table the tests drive: a copy leaves when it is
// asked, when it is forced, or never.
type fakeCopies struct {
	mu       sync.Mutex
	alive    []int
	leaveAsk bool
	leaveEnd bool
	asked    []int
	forced   []int
	listErr  error
}

func (f *fakeCopies) finder() copyFinder {
	return copyFinder{
		list: func(context.Context, string) ([]int, error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			return slices.Clone(f.alive), f.listErr
		},
		ask: func(pid int) error {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.asked = append(f.asked, pid)
			if f.leaveAsk {
				f.alive = slices.DeleteFunc(f.alive, func(p int) bool { return p == pid })
			}
			return nil
		},
		force: func(pid int) error {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.forced = append(f.forced, pid)
			if f.leaveEnd {
				f.alive = slices.DeleteFunc(f.alive, func(p int) bool { return p == pid })
			}
			return nil
		},
	}
}

func TestCloseRunning(t *testing.T) {
	t.Run("nothing open asks nobody", func(t *testing.T) {
		f := &fakeCopies{}
		if err := f.finder().close(t.Context(), "rpcs3", time.Second); err != nil || len(f.asked) != 0 {
			t.Fatalf("err %v asked %v", err, f.asked)
		}
	})
	t.Run("a copy that quits when asked is never forced", func(t *testing.T) {
		f := &fakeCopies{alive: []int{10, 11}, leaveAsk: true}
		if err := f.finder().close(t.Context(), "rpcs3", time.Second); err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(f.asked, []int{10, 11}) || len(f.forced) != 0 {
			t.Fatalf("asked %v forced %v", f.asked, f.forced)
		}
	})
	t.Run("a copy that ignores the request is ended after the wait", func(t *testing.T) {
		f := &fakeCopies{alive: []int{10}, leaveEnd: true}
		if err := f.finder().close(t.Context(), "rpcs3", 150*time.Millisecond); err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(f.forced, []int{10}) {
			t.Fatalf("forced %v", f.forced)
		}
	})
	t.Run("a copy that survives is reported, not assumed closed", func(t *testing.T) {
		f := &fakeCopies{alive: []int{10}}
		err := f.finder().close(t.Context(), "/x/rpcs3", 100*time.Millisecond)
		if err == nil || !strings.Contains(err.Error(), "rpcs3") {
			t.Fatalf("err %v", err)
		}
	})
	t.Run("a table that cannot be read is an error", func(t *testing.T) {
		f := &fakeCopies{listErr: errors.New("denied")}
		if err := f.finder().close(t.Context(), "rpcs3", time.Second); err == nil {
			t.Fatal("no error")
		}
	})
	t.Run("cancelling stops the wait", func(t *testing.T) {
		f := &fakeCopies{alive: []int{10}}
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if err := f.finder().close(ctx, "rpcs3", time.Minute); !errors.Is(err, context.Canceled) {
			t.Fatalf("err %v", err)
		}
	})
}

func TestOnlyRPCS3IsSingleInstance(t *testing.T) {
	for _, p := range Platforms() {
		if p.SingleInstance() != (p.ID == PS3) {
			t.Errorf("%s: SingleInstance = %v", p.ID, p.SingleInstance())
		}
	}
}
