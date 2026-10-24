package catalog

import (
	"context"
	"errors"
	"testing"
	"time"
)

type relatedRemote struct {
	remoteFixture
	items []RelatedGame
	err   error
	asked []string
}

func (r *relatedRemote) Related(_ context.Context, id string) ([]RelatedGame, error) {
	r.asked = append(r.asked, id)
	return r.items, r.err
}

func TestOnPC(t *testing.T) {
	cases := []struct {
		name      string
		platforms []string
		want      bool
	}{
		{"unknown", nil, true},
		{"windows", []string{"PlayStation 4", "PC (Microsoft Windows)"}, true},
		{"dos", []string{"DOS"}, true},
		{"console only", []string{"PlayStation 5", "Xbox Series X|S"}, false},
	}
	for _, tc := range cases {
		if got := OnPC(tc.platforms); got != tc.want {
			t.Errorf("%s: OnPC = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestRelated(t *testing.T) {
	ctx := context.Background()
	t.Run("no remote is an error", func(t *testing.T) {
		s := mustServiceAt(t, t.TempDir())
		seed(t, s, Game{ID: "g", ServerID: "srv", Title: "G"})
		if _, err := s.Related(ctx, "g"); !errors.Is(err, errNoRemote) {
			t.Fatalf("err = %v, want errNoRemote", err)
		}
	})
	t.Run("remote without related support is an error", func(t *testing.T) {
		s := mustServiceAt(t, t.TempDir())
		seed(t, s, Game{ID: "g", ServerID: "srv", Title: "G"})
		s.SetRemoteCatalog(&remoteFixture{})
		if _, err := s.Related(ctx, "g"); !errors.Is(err, errNoRemote) {
			t.Fatalf("err = %v, want errNoRemote", err)
		}
	})
	t.Run("no server id is an empty list", func(t *testing.T) {
		s := mustServiceAt(t, t.TempDir())
		seed(t, s, Game{ID: "local", Title: "Local"})
		remote := &relatedRemote{}
		s.SetRemoteCatalog(remote)
		got, err := s.Related(ctx, "local")
		if err != nil || got == nil || len(got) != 0 || len(remote.asked) != 0 {
			t.Fatalf("got %v err %v asked %v", got, err, remote.asked)
		}
	})
	t.Run("unknown id is an empty list", func(t *testing.T) {
		s := mustServiceAt(t, t.TempDir())
		s.SetRemoteCatalog(&relatedRemote{})
		got, err := s.Related(ctx, "nope")
		if err != nil || len(got) != 0 {
			t.Fatalf("got %v err %v", got, err)
		}
	})
	t.Run("server 404 is an empty list", func(t *testing.T) {
		s := mustServiceAt(t, t.TempDir())
		seed(t, s, Game{ID: "g", ServerID: "srv", Title: "G"})
		s.SetRemoteCatalog(&relatedRemote{err: ErrRelatedNotFound})
		got, err := s.Related(ctx, "g")
		if err != nil || got == nil || len(got) != 0 {
			t.Fatalf("got %v err %v", got, err)
		}
	})
	t.Run("errors are not cached, results are", func(t *testing.T) {
		now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		s := mustServiceAt(t, t.TempDir())
		s.clock = func() time.Time { return now }
		seed(t, s, Game{ID: "g", ServerID: "srv", Title: "G"})
		remote := &relatedRemote{err: errors.New("boom")}
		s.SetRemoteCatalog(remote)
		if _, err := s.Related(ctx, "g"); err == nil {
			t.Fatal("remote error swallowed")
		}
		remote.err = nil
		remote.items = []RelatedGame{{ID: "r", Title: "Remake", Relation: "remake"}}
		got, err := s.Related(ctx, "g")
		if err != nil || len(got) != 1 || len(remote.asked) != 2 {
			t.Fatalf("after recovery: %v %v asked %v", got, err, remote.asked)
		}
		if _, err = s.Related(ctx, "g"); err != nil || len(remote.asked) != 2 {
			t.Fatalf("cache miss within TTL: %v asked %v", err, remote.asked)
		}
		now = now.Add(relatedCacheTTL + time.Second)
		if _, err = s.Related(ctx, "g"); err != nil || len(remote.asked) != 3 {
			t.Fatalf("cache not expired: %v asked %v", err, remote.asked)
		}
	})
}

func TestLocalIDByServerID(t *testing.T) {
	s := mustServiceAt(t, t.TempDir())
	seed(t, s, Game{ID: "g", ServerID: "srv", Title: "G"})
	if id, ok := s.LocalIDByServerID("srv"); !ok || id != "g" {
		t.Fatalf("got %q %v", id, ok)
	}
	if _, ok := s.LocalIDByServerID("missing"); ok {
		t.Fatal("found a missing id")
	}
	if _, ok := s.LocalIDByServerID(""); ok {
		t.Fatal("empty id matched")
	}
}
