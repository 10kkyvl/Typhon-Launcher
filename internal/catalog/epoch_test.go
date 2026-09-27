package catalog

import (
	"testing"
	"time"

	"typhon/internal/titles"
)

func TestEpochMovesWhenMatchingCouldChange(t *testing.T) {
	s := newTestService(t)
	game := seed(t, s, Game{Title: "Epoch Base", GameType: "Main Game"})[0]

	cases := []struct {
		name string
		act  func(t *testing.T)
	}{
		{"a game is added", func(t *testing.T) {
			seed(t, s, Game{Title: "Epoch Second"})
		}},
		{"an override is learnt", func(t *testing.T) {
			if err := s.LearnMatch("epoch alias", game.ID); err != nil {
				t.Fatalf("LearnMatch() error = %v", err)
			}
		}},
		{"an override is forgotten", func(t *testing.T) {
			if err := s.ForgetMatch("epoch alias"); err != nil {
				t.Fatalf("ForgetMatch() error = %v", err)
			}
		}},
		{"metadata is applied", func(t *testing.T) {
			if _, err := s.ApplyMetadata(game.ID, MetadataPatch{IGDBID: "42", Title: "Epoch Base", UpdatedAt: time.Now()}); err != nil {
				t.Fatalf("ApplyMetadata() error = %v", err)
			}
		}},
		{"a game is provisioned", func(t *testing.T) {
			if _, err := s.Provision([]Query{{Title: "Epoch Provisioned"}}); err != nil {
				t.Fatalf("Provision() error = %v", err)
			}
		}},
		{"the dictionary is replaced", func(t *testing.T) {
			dict, err := titles.NewDict(titles.Spec{Version: titles.DictVersion})
			if err != nil {
				t.Fatalf("NewDict() error = %v", err)
			}
			previous := titles.Active()
			t.Cleanup(func() { titles.SetActive(previous) })
			titles.SetActive(dict)
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := s.Epoch()
			tc.act(t)
			if after := s.Epoch(); after == before {
				t.Fatalf("Epoch stayed at %d", after)
			}
		})
	}
}

func TestEpochIsStableWithoutChanges(t *testing.T) {
	s := newTestService(t)
	seed(t, s, Game{Title: "Epoch Still"})

	first := s.Epoch()
	if first == 0 {
		t.Fatal("Epoch() = 0, which a release reads as never matched")
	}
	for range 3 {
		s.ListGames()
		s.SearchGames("Epoch", 5)
		s.Resolve(Query{Title: "Epoch Still"})
		if got := s.Epoch(); got != first {
			t.Fatalf("Epoch() = %d after a read-only call, want %d", got, first)
		}
	}
}
