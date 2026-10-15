package library

import (
	"errors"
	"os"
	"slices"
	"strconv"
	"testing"
)

func TestKillTree(t *testing.T) {
	errDenied := errors.New("access denied")
	errSnapshot := errors.New("snapshot failed")
	errRoot := errors.New("root kill failed")
	tree := map[uint32]uint32{20: 10, 30: 20, 31: 20, 99: 7}

	cases := []struct {
		name      string
		parents   func() (map[uint32]uint32, error)
		failPID   uint32
		rootErr   error
		wantOrder []string
		wantErrs  []error
	}{
		{
			name:      "descendants deepest first, then the root",
			parents:   func() (map[uint32]uint32, error) { return tree, nil },
			wantOrder: []string{"30", "31", "20", "root"},
		},
		{
			name:      "a failed descendant is returned and the rest are still ended",
			parents:   func() (map[uint32]uint32, error) { return tree, nil },
			failPID:   30,
			wantOrder: []string{"30", "31", "20", "root"},
			wantErrs:  []error{errDenied},
		},
		{
			name:      "an unreadable process table still ends the root and reports it",
			parents:   func() (map[uint32]uint32, error) { return nil, errSnapshot },
			wantOrder: []string{"root"},
			wantErrs:  []error{errSnapshot},
		},
		{
			name:      "root failure is reported next to a descendant failure",
			parents:   func() (map[uint32]uint32, error) { return tree, nil },
			failPID:   31,
			rootErr:   errRoot,
			wantOrder: []string{"30", "31", "20", "root"},
			wantErrs:  []error{errDenied, errRoot},
		},
		{
			name:      "no children",
			parents:   func() (map[uint32]uint32, error) { return map[uint32]uint32{99: 7}, nil },
			wantOrder: []string{"root"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var order []string
			kill := func(pid uint32) error {
				order = append(order, strconv.FormatUint(uint64(pid), 10))
				if pid == c.failPID {
					return errDenied
				}
				return nil
			}
			err := killTree(10, c.parents, kill, func() error {
				order = append(order, "root")
				return c.rootErr
			})
			if !slices.Equal(order, c.wantOrder) {
				t.Fatalf("kill order = %v, want %v", order, c.wantOrder)
			}
			if len(c.wantErrs) == 0 && err != nil {
				t.Fatalf("killTree = %v, want nil", err)
			}
			for _, want := range c.wantErrs {
				if !errors.Is(err, want) {
					t.Fatalf("killTree = %v, want it to contain %v", err, want)
				}
			}
		})
	}

	t.Run("root 0 is refused before anything is touched", func(t *testing.T) {
		called := false
		table := map[uint32]uint32{4: 0, 20: 4, 30: 20}
		err := killTree(0, func() (map[uint32]uint32, error) { return table, nil },
			func(uint32) error { called = true; return nil },
			func() error { called = true; return nil })
		if !errors.Is(err, errNoRootPID) || called {
			t.Fatalf("killTree(0) = %v, called=%v; want errNoRootPID and no kills", err, called)
		}
	})

	t.Run("the launcher itself is never ended as a descendant", func(t *testing.T) {
		self := uint32(os.Getpid()) //nolint:gosec // G115: test pid fits uint32
		table := map[uint32]uint32{self: 10, 30: self, 31: 10}
		var killed []uint32
		err := killTree(10, func() (map[uint32]uint32, error) { return table, nil },
			func(pid uint32) error { killed = append(killed, pid); return nil },
			func() error { return nil })
		if err != nil {
			t.Fatal(err)
		}
		if slices.Contains(killed, self) || !slices.Contains(killed, 30) || !slices.Contains(killed, 31) {
			t.Fatalf("killed = %v, want 30 and 31 without own pid %d", killed, self)
		}
	})
}
