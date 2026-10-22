package library

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

type fakeProc struct{}

func (*fakeProc) pid() int    { return 1 }
func (*fakeProc) wait() error { return nil }
func (*fakeProc) kill() error { return nil }

func TestRemoveExternal(t *testing.T) {
	tests := []struct {
		name    string
		refs    []ExternalRef
		running bool
		want    int
		left    int
	}{
		{"imported non-game removed", []ExternalRef{{Provider: ProviderSteam, AppID: "42"}}, false, 1, 0},
		{"unknown ref ignored", []ExternalRef{{Provider: ProviderSteam, AppID: "7"}}, false, 0, 1},
		{"other provider ignored", []ExternalRef{{Provider: ProviderGOG, AppID: "42"}}, false, 0, 1},
		{"running game kept", []ExternalRef{{Provider: ProviderSteam, AppID: "42"}}, true, 0, 1},
		{"no refs", nil, false, 0, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := syncFixture(t)
			id := s.GetGames()[0].ID
			if tc.running {
				s.mu.Lock()
				s.running[id] = &session{}
				s.mu.Unlock()
			}
			n, err := s.RemoveExternal(tc.refs)
			if err != nil {
				t.Fatal(err)
			}
			if n != tc.want {
				t.Fatalf("removed = %d, want %d", n, tc.want)
			}
			if got := len(s.GetGames()); got != tc.left {
				t.Fatalf("games left = %d, want %d", got, tc.left)
			}
		})
	}
}

func TestRemoveExternalSurvivesReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)
	item, _ := externalFixture(t)
	if _, err := s.ApplyExternal(context.Background(), []ExternalGame{item}); err != nil {
		t.Fatal(err)
	}
	if n, err := s.RemoveExternal([]ExternalRef{{Provider: ProviderSteam, AppID: "42"}}); err != nil || n != 1 {
		t.Fatalf("remove = %d, %v", n, err)
	}
	if len(s.ExternalRefs()) != 0 {
		t.Fatalf("external refs left: %v", s.ExternalRefs())
	}
	reloaded := mustServiceAt(t, path)
	if len(reloaded.GetGames()) != 0 || len(reloaded.ExternalRefs()) != 0 {
		t.Fatalf("removed game came back after reload: %+v", reloaded.GetGames())
	}
}

func TestRemoveExternalClosesDetectedSessionWithoutPlaytime(t *testing.T) {
	tests := []struct {
		name        string
		sess        *session
		wantRemoved int
		wantStopped bool
	}{
		{"detected session", &session{external: true}, 1, true},
		{"launched by us", &session{process: &fakeProc{}}, 0, false},
		{"not detected, not launched", &session{}, 0, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := syncFixture(t)
			id := s.GetGames()[0].ID
			recorded := 0
			s.SetPlayRecorder(func(string, time.Time, time.Time) { recorded++ })
			watcher := recordingWatcher{started: make(chan Game, 1), stopped: make(chan string, 1)}
			s.AddSessionWatcher(watcher)
			s.mu.Lock()
			s.running[id] = tc.sess
			s.mu.Unlock()

			n, err := s.RemoveExternal([]ExternalRef{{Provider: ProviderSteam, AppID: "42"}})
			if err != nil {
				t.Fatal(err)
			}
			if n != tc.wantRemoved {
				t.Fatalf("removed = %d, want %d", n, tc.wantRemoved)
			}
			select {
			case got := <-watcher.stopped:
				if !tc.wantStopped || got != id {
					t.Fatalf("watcher stopped %q, wantStopped=%v", got, tc.wantStopped)
				}
			default:
				if tc.wantStopped {
					t.Fatal("watcher did not get SessionStopped")
				}
			}
			s.wg.Wait()
			if recorded != 0 {
				t.Fatalf("play recorder called %d times for a non-game", recorded)
			}
			if tc.wantStopped && s.IsRunning(id) {
				t.Fatal("session still running after removal")
			}
		})
	}
}

func TestRemoveExternalKeepsSessionWhenSaveFails(t *testing.T) {
	s, _ := syncFixture(t)
	id := s.GetGames()[0].ID
	watcher := recordingWatcher{started: make(chan Game, 1), stopped: make(chan string, 1)}
	s.AddSessionWatcher(watcher)
	s.mu.Lock()
	s.running[id] = &session{external: true}
	s.mu.Unlock()
	s.path = t.TempDir()

	n, err := s.RemoveExternal([]ExternalRef{{Provider: ProviderSteam, AppID: "42"}})
	if err == nil || n != 0 {
		t.Fatalf("remove = %d, %v; want a save error", n, err)
	}
	if len(s.GetGames()) != 1 || !s.IsRunning(id) {
		t.Fatalf("game or session lost after failed save: games=%d running=%v", len(s.GetGames()), s.IsRunning(id))
	}
	select {
	case got := <-watcher.stopped:
		t.Fatalf("watcher got SessionStopped(%q) although nothing was removed", got)
	default:
	}
}
