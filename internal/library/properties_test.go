package library

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"typhon/internal/uierr"
)

var errStopAfterCapture = errors.New("stop after capture")

func propertiesService(t *testing.T) (*Service, Game) {
	t.Helper()
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	game, err := s.AddGame(tempGameExe(t), "Props")
	if err != nil {
		t.Fatal(err)
	}
	return s, game
}

func blockPersist(t *testing.T, s *Service) {
	t.Helper()
	blocked := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(blocked, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	s.path = filepath.Join(blocked, "library.json")
}

func TestSetLaunchArgsTable(t *testing.T) {
	tooMany := make([]string, 33)
	for i := range tooMany {
		tooMany[i] = "a"
	}
	cases := []struct {
		name     string
		in       []string
		want     []string
		wantCode string
	}{
		{name: "keeps content as typed", in: []string{"-name", "  x ", "-w"}, want: []string{"-name", "  x ", "-w"}},
		{name: "keeps explicit empty argument", in: []string{"-flag", ""}, want: []string{"-flag", ""}},
		{name: "nil clears", in: nil, want: nil},
		{name: "empty slice clears", in: []string{}, want: nil},
		{name: "newline", in: []string{"-a\n-b"}, wantCode: "library.launch_args_invalid"},
		{name: "carriage return", in: []string{"-a\rb"}, wantCode: "library.launch_args_invalid"},
		{name: "nul", in: []string{"-a\x00"}, wantCode: "library.launch_args_invalid"},
		{name: "tab inside", in: []string{"a\tb"}, wantCode: "library.launch_args_invalid"},
		{name: "too many", in: tooMany, wantCode: "library.launch_args_too_long"},
		{name: "too long total", in: []string{strings.Repeat("a", 300), strings.Repeat("b", 213)}, wantCode: "library.launch_args_too_long"},
		{name: "exact total", in: []string{strings.Repeat("a", 300), strings.Repeat("b", 212)}, want: []string{strings.Repeat("a", 300), strings.Repeat("b", 212)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, game := propertiesService(t)
			if _, err := s.SetLaunchArgs(game.ID, []string{"-keep"}); err != nil {
				t.Fatal(err)
			}
			got, err := s.SetLaunchArgs(game.ID, tc.in)
			if tc.wantCode != "" {
				if code := uierr.Code(err); code != tc.wantCode {
					t.Fatalf("code = %q (%v), want %q", code, err, tc.wantCode)
				}
				stored, findErr := s.Find(game.ID)
				if findErr != nil {
					t.Fatal(findErr)
				}
				if !reflect.DeepEqual(stored.LaunchArgs, []string{"-keep"}) {
					t.Fatalf("rejected input changed args: %v", stored.LaunchArgs)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got.LaunchArgs, tc.want) {
				t.Fatalf("args = %#v, want %#v", got.LaunchArgs, tc.want)
			}
		})
	}
}

func TestSetLaunchArgsPersistsAndRollsBack(t *testing.T) {
	s, game := propertiesService(t)
	if _, err := s.SetLaunchArgs(game.ID, []string{"-a"}); err != nil {
		t.Fatal(err)
	}
	reloaded, err := NewServiceAt(s.path)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := reloaded.Find(game.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(stored.LaunchArgs, []string{"-a"}) {
		t.Fatalf("reloaded args = %v", stored.LaunchArgs)
	}

	blockPersist(t, s)
	if _, err := s.SetLaunchArgs(game.ID, []string{"-b"}); err == nil {
		t.Fatal("expected persist failure")
	}
	after, err := s.Find(game.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after.LaunchArgs, []string{"-a"}) {
		t.Fatalf("args after failed persist = %v, want rollback to [-a]", after.LaunchArgs)
	}
}

func TestSetLaunchArgsUnknownGame(t *testing.T) {
	s, _ := propertiesService(t)
	if _, err := s.SetLaunchArgs("missing", []string{"-a"}); err == nil {
		t.Fatal("expected not found")
	}
}

func TestLaunchArgsLineRoundTrip(t *testing.T) {
	cases := [][]string{
		{"-windowed"},
		{"-name", "John Smith", "-w"},
		{`C:\Games\My Game\save`},
		{`say "hi"`},
		{`path\`, `a b\`},
		{`\\server\share dir\`},
		{"-x=1", "--flag"},
	}
	for _, args := range cases {
		line := LaunchArgsLine(args)
		got, err := ParseLaunchArgs(line)
		if err != nil {
			t.Fatalf("%v: line %q: %v", args, line, err)
		}
		if !reflect.DeepEqual(got, args) {
			t.Fatalf("round trip of %q via %q = %q", args, line, got)
		}
	}
}

func TestParseLaunchArgsTable(t *testing.T) {
	cases := []struct {
		name     string
		in       string
		want     []string
		wantCode string
	}{
		{name: "empty", in: "", want: nil},
		{name: "blank", in: "  \t ", want: nil},
		{name: "plain", in: "-a  -b\t-c", want: []string{"-a", "-b", "-c"}},
		{name: "quoted space", in: `-n "A B" -x`, want: []string{"-n", "A B", "-x"}},
		{name: "escaped quote", in: `say \"hi\"`, want: []string{"say", `"hi"`}},
		{name: "backslashes before quote", in: `"a\\" b`, want: []string{`a\`, "b"}},
		{name: "backslashes without quote", in: `C:\dir\file`, want: []string{`C:\dir\file`}},
		{name: "unterminated quote", in: `-n "A B`, wantCode: "library.launch_args_invalid"},
		{name: "single quote", in: `"`, wantCode: "library.launch_args_invalid"},
		{name: "empty quoted argument", in: `""`, want: []string{""}},
		{name: "flag with empty argument", in: `-flag ""`, want: []string{"-flag", ""}},
		{name: "only backslashes", in: `\\\\`, want: []string{`\\\\`}},
		{name: "lone escaped quote", in: `\"`, want: []string{`"`}},
		{name: "trailing backslashes", in: `a\\ b\`, want: []string{`a\\`, `b\`}},
		{name: "trailing backslashes in quotes", in: `"a b\\"`, want: []string{`a b\`}},
		{name: "odd backslashes before closing quote", in: `"a\"`, wantCode: "library.launch_args_invalid"},
		{name: "newline", in: "-a\n-b", wantCode: "library.launch_args_invalid"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseLaunchArgs(tc.in)
			if tc.wantCode != "" {
				if code := uierr.Code(err); code != tc.wantCode {
					t.Fatalf("code = %q (%v), want %q", code, err, tc.wantCode)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestSetLaunchArgsLineAndGet(t *testing.T) {
	s, game := propertiesService(t)
	updated, err := s.SetLaunchArgsLine(game.ID, `-name "John Smith" -w`)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(updated.LaunchArgs, []string{"-name", "John Smith", "-w"}) {
		t.Fatalf("args = %q", updated.LaunchArgs)
	}
	line, err := s.GetLaunchArgsLine(game.ID)
	if err != nil {
		t.Fatal(err)
	}
	if line != `-name "John Smith" -w` {
		t.Fatalf("line = %q", line)
	}
	if _, err := s.SetLaunchArgsLine(game.ID, `"open`); err == nil {
		t.Fatal("unterminated quote accepted")
	}
	if _, err := s.GetLaunchArgsLine("missing"); err == nil {
		t.Fatal("expected not found")
	}
}

func TestSetRunAsAdmin(t *testing.T) {
	s, game := propertiesService(t)
	on, err := s.SetRunAsAdmin(game.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if !on.RunAsAdmin {
		t.Fatal("flag not set")
	}
	reloaded, err := NewServiceAt(s.path)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := reloaded.Find(game.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !stored.RunAsAdmin {
		t.Fatal("flag not persisted")
	}

	blockPersist(t, s)
	if _, err := s.SetRunAsAdmin(game.ID, false); err == nil {
		t.Fatal("expected persist failure")
	}
	after, err := s.Find(game.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !after.RunAsAdmin {
		t.Fatal("failed persist left the flag cleared in memory")
	}
	if _, err := s.SetRunAsAdmin("missing", true); err == nil {
		t.Fatal("expected not found")
	}
}

func TestPlayGamePassesRunAsAdmin(t *testing.T) {
	s, game := propertiesService(t)
	if _, err := s.SetRunAsAdmin(game.ID, true); err != nil {
		t.Fatal(err)
	}
	var got launch
	s.start = func(_ context.Context, req launch) (gameProcess, error) {
		got = req
		return nil, errStopAfterCapture
	}
	if err := s.PlayGame(game.ID); err == nil {
		t.Fatal("expected the capturing starter to fail the launch")
	}
	if !got.elevated {
		t.Fatalf("launch.elevated = false, want true: %+v", got)
	}
}

func TestClearSavesDir(t *testing.T) {
	s, game := propertiesService(t)
	dir := t.TempDir()
	if _, err := s.SetSavesDir(game.ID, dir); err != nil {
		t.Fatal(err)
	}
	cleared, err := s.ClearSavesDir(game.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cleared.SavesDir != "" {
		t.Fatalf("SavesDir = %q after clear", cleared.SavesDir)
	}
	reloaded, err := NewServiceAt(s.path)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := reloaded.Find(game.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.SavesDir != "" {
		t.Fatalf("persisted SavesDir = %q", stored.SavesDir)
	}
	if _, err := s.ClearSavesDir(game.ID); err != nil {
		t.Fatalf("clearing an already empty path: %v", err)
	}
	if _, err := s.ClearSavesDir("missing"); err == nil {
		t.Fatal("expected not found")
	}
}

func TestClearSavesDirRollsBackWhenPersistFails(t *testing.T) {
	s, game := propertiesService(t)
	dir := t.TempDir()
	if _, err := s.SetSavesDir(game.ID, dir); err != nil {
		t.Fatal(err)
	}
	blockPersist(t, s)
	if _, err := s.ClearSavesDir(game.ID); err == nil {
		t.Fatal("expected persist failure")
	}
	after, err := s.Find(game.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.SavesDir == "" {
		t.Fatal("failed persist left SavesDir cleared in memory")
	}
}

func TestLaunchArgsLineIsIdempotent(t *testing.T) {
	lines := []string{
		"", "-a  -b\t-c", `-n "A B" -x`, `say \"hi\"`, `"a\\" b`, `C:\dir\file`,
		`""`, `-flag ""`, `\\`, `\"`, `a\ b\`, `"a b\\"`, `"  x "`,
	}
	for _, line := range lines {
		first, err := ParseLaunchArgs(line)
		if err != nil {
			t.Fatalf("%q: %v", line, err)
		}
		once := LaunchArgsLine(first)
		second, err := ParseLaunchArgs(once)
		if err != nil {
			t.Fatalf("%q: reparse of %q: %v", line, once, err)
		}
		if !reflect.DeepEqual(first, second) {
			t.Fatalf("%q: args %q reparsed from %q as %q", line, first, once, second)
		}
		if twice := LaunchArgsLine(second); twice != once {
			t.Fatalf("%q: line %q then %q", line, once, twice)
		}
	}
}

func TestLaunchArgsEmptyArgumentRoundTrip(t *testing.T) {
	s, game := propertiesService(t)
	updated, err := s.SetLaunchArgsLine(game.ID, `-flag ""`)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(updated.LaunchArgs, []string{"-flag", ""}) {
		t.Fatalf("args = %q", updated.LaunchArgs)
	}
	line, err := s.GetLaunchArgsLine(game.ID)
	if err != nil {
		t.Fatal(err)
	}
	if line != `-flag ""` {
		t.Fatalf("line = %q", line)
	}
}

func TestUserPropertiesSurviveReinstallAndUpdate(t *testing.T) {
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	exe := tempGameExe(t)
	installed, err := s.RegisterInstalled(InstalledGame{Title: "Game", Executable: exe, InstallDir: filepath.Dir(exe), Version: "1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetLaunchArgs(installed.ID, []string{"-w", ""}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetRunAsAdmin(installed.ID, true); err != nil {
		t.Fatal(err)
	}
	check := func(step string) {
		t.Helper()
		got, err := s.Find(installed.ID)
		if err != nil {
			t.Fatalf("%s: %v", step, err)
		}
		if !got.RunAsAdmin || !reflect.DeepEqual(got.LaunchArgs, []string{"-w", ""}) {
			t.Fatalf("%s: RunAsAdmin=%v LaunchArgs=%q, user settings lost", step, got.RunAsAdmin, got.LaunchArgs)
		}
	}

	if _, err := s.RegisterInstalled(InstalledGame{Title: "Game", Executable: exe, InstallDir: filepath.Dir(exe), Version: "2"}); err != nil {
		t.Fatal(err)
	}
	check("reinstall")
	if _, err := s.ApplyInstalledUpdate(InstalledUpdate{ID: installed.ID, Version: "3"}); err != nil {
		t.Fatal(err)
	}
	check("update")
	if err := s.MarkUninstalled(installed.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddGame(exe, ""); err != nil {
		t.Fatal(err)
	}
	check("revive")
}

type changeWatcher struct {
	changed []Game
	at      []time.Time
}

func (w *changeWatcher) SessionStarted(Game)   {}
func (w *changeWatcher) SessionStopped(string) {}
func (w *changeWatcher) SessionChanged(game Game, startedAt time.Time) {
	w.changed = append(w.changed, game)
	w.at = append(w.at, startedAt)
}

func TestSetHidePresence(t *testing.T) {
	s, game := propertiesService(t)
	on, err := s.SetHidePresence(game.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if !on.HidePresence {
		t.Fatal("flag not set")
	}
	reloaded, err := NewServiceAt(s.path)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := reloaded.Find(game.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !stored.HidePresence {
		t.Fatal("flag not persisted")
	}

	blockPersist(t, s)
	if _, err := s.SetHidePresence(game.ID, false); err == nil {
		t.Fatal("expected persist failure")
	}
	after, err := s.Find(game.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !after.HidePresence {
		t.Fatal("failed persist left the flag cleared in memory")
	}
	if _, err := s.SetHidePresence("missing", true); !errors.Is(err, errNotFound) {
		t.Fatalf("missing game: %v", err)
	}
}

func TestSetHidePresenceNotifiesOnlyRunningSession(t *testing.T) {
	s, game := propertiesService(t)
	w := &changeWatcher{}
	s.AddSessionWatcher(w)

	if _, err := s.SetHidePresence(game.ID, true); err != nil {
		t.Fatal(err)
	}
	if len(w.changed) != 0 {
		t.Fatalf("idle game notified watchers: %d", len(w.changed))
	}

	startedAt := time.Unix(1700000000, 0)
	s.mu.Lock()
	s.running[game.ID] = &session{startedAt: startedAt, lastSeen: startedAt}
	s.mu.Unlock()
	if _, err := s.SetHidePresence(game.ID, false); err != nil {
		t.Fatal(err)
	}
	if len(w.changed) != 1 || w.changed[0].HidePresence || !w.at[0].Equal(startedAt) {
		t.Fatalf("running game change: %+v at %v", w.changed, w.at)
	}

	if _, err := s.SetHidePresence(game.ID, false); err != nil {
		t.Fatal(err)
	}
	if len(w.changed) != 1 {
		t.Fatalf("unchanged flag notified watchers: %d", len(w.changed))
	}

	blockPersist(t, s)
	if _, err := s.SetHidePresence(game.ID, true); err == nil {
		t.Fatal("expected persist failure")
	}
	if len(w.changed) != 1 {
		t.Fatalf("failed save notified watchers: %d", len(w.changed))
	}
}
