package main

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"typhon/internal/emulators"
	"typhon/internal/library"
	"typhon/internal/uierr"
)

type fakeGames struct {
	game library.Game
	err  error
}

func (f fakeGames) Find(string) (library.Game, error) { return f.game, f.err }

var ps2Game = library.Game{ID: "g1", Emulation: &library.Emulation{Platform: emulators.PS2}}

type fakeLauncher struct{ err error }

func (f fakeLauncher) PlayGame(string) error                { return f.err }
func (f fakeLauncher) PlayGameProfile(string, string) error { return f.err }

func promptsFor(games gameFinder) (*EmulatorPromptService, *[]EmulatorNeeded) {
	var events []EmulatorNeeded
	return newEmulatorPromptService(games, func(needed EmulatorNeeded) { events = append(events, needed) }), &events
}

func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return &buf
}

func TestEveryPromptCodeAsksForAnEmulator(t *testing.T) {
	sentinels := []error{
		emulators.ErrNotConfigured, emulators.ErrFileMissing, emulators.ErrWindowsProgram, emulators.ErrNotExe,
		emulators.ErrNotRegularFile, emulators.ErrFileUnreadable, emulators.ErrPathRelative,
	}
	for _, sentinel := range sentinels {
		code := uierr.Code(sentinel)
		t.Run(code, func(t *testing.T) {
			prompts, events := promptsFor(fakeGames{game: ps2Game})
			prompts.offer("g1", fmt.Errorf("launch: %w", sentinel))
			want := EmulatorNeeded{GameID: "g1", Platform: emulators.PS2, Code: code}
			if !slices.Equal(*events, []EmulatorNeeded{want}) {
				t.Fatalf("events = %+v, want one %+v", *events, want)
			}
			if got := prompts.TakePending(); got != want {
				t.Fatalf("pending = %+v, want %+v", got, want)
			}
			if got := prompts.TakePending(); got != (EmulatorNeeded{}) {
				t.Fatalf("second take = %+v, want nothing", got)
			}
		})
	}
}

func TestWrappedCodeOfAnUnreadableEmulatorAsks(t *testing.T) {
	prompts, events := promptsFor(fakeGames{game: ps2Game})
	prompts.offer("g1", uierr.Wrap(uierr.Code(emulators.ErrFileUnreadable), errors.New("denied")))
	if len(*events) != 1 {
		t.Fatalf("events = %+v", *events)
	}
}

func TestOtherOutcomesDoNotAsk(t *testing.T) {
	cases := []struct {
		name  string
		games fakeGames
		err   error
	}{
		{"launched", fakeGames{game: ps2Game}, nil},
		{"plain error", fakeGames{game: ps2Game}, errors.New("boom")},
		{"foreign code", fakeGames{game: ps2Game}, uierr.New("library.already_running", "x")},
		{"unknown platform", fakeGames{game: ps2Game}, emulators.ErrUnknownPlatform},
		{"bad bundle", fakeGames{game: ps2Game}, emulators.ErrBadAppBundle},
		{"invalid path", fakeGames{game: ps2Game}, emulators.ErrPathInvalid},
		{"game is gone", fakeGames{err: errors.New("not found")}, emulators.ErrNotConfigured},
		{"native game", fakeGames{game: library.Game{ID: "g1"}}, emulators.ErrNotConfigured},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			prompts, events := promptsFor(c.games)
			prompts.offer("g1", c.err)
			if len(*events) != 0 {
				t.Fatalf("events = %+v", *events)
			}
			if got := prompts.TakePending(); got != (EmulatorNeeded{}) {
				t.Fatalf("pending = %+v", got)
			}
		})
	}
}

func TestPromptWaitsForAPageThatIsNotReady(t *testing.T) {
	prompts := newEmulatorPromptService(fakeGames{game: ps2Game}, func(EmulatorNeeded) {})
	prompts.offer("g1", emulators.ErrFileMissing)
	prompts.offer("g1", emulators.ErrNotConfigured)
	want := EmulatorNeeded{GameID: "g1", Platform: emulators.PS2, Code: uierr.Code(emulators.ErrNotConfigured)}
	if got := prompts.TakePending(); got != want {
		t.Fatalf("pending = %+v, want the newer %+v", got, want)
	}
}

func TestLaunchSurfaces(t *testing.T) {
	surfaces := []struct {
		name string
		play func(player promptingPlayer, reveal func())
	}{
		{"tray", func(p promptingPlayer, reveal func()) { playFromTray(p, "g1", reveal) }},
		{"shortcut", func(p promptingPlayer, reveal func()) { playFromShortcut(p, playRequest{id: "g1"}, reveal) }},
		{"second launch", func(p promptingPlayer, reveal func()) {
			handleSecondLaunch([]string{"typhon.exe", "--play", "g1"}, p, openerFunc(reveal), &NavigationService{}, func(string) {})
		}},
	}
	outcomes := []struct {
		name       string
		err        error
		wantEvents int
		wantReveal int
		wantLevel  string
		wantNot    string
	}{
		{"emulator missing", emulators.ErrNotConfigured, 1, 1, "level=WARN", "level=ERROR"},
		{"other failure", errors.New("boom"), 0, 1, "level=ERROR", "level=WARN"},
		{"launched", nil, 0, 0, "", "level="},
	}
	for _, surface := range surfaces {
		for _, outcome := range outcomes {
			t.Run(surface.name+"/"+outcome.name, func(t *testing.T) {
				log := captureLog(t)
				prompts, events := promptsFor(fakeGames{game: ps2Game})
				revealed := 0
				surface.play(promptingPlayer{player: fakeLauncher{err: outcome.err}, prompts: prompts}, func() { revealed++ })
				if len(*events) != outcome.wantEvents || revealed != outcome.wantReveal {
					t.Fatalf("events=%d revealed=%d, want %d and %d", len(*events), revealed, outcome.wantEvents, outcome.wantReveal)
				}
				logged := log.String()
				if !strings.Contains(logged, outcome.wantLevel) || strings.Contains(logged, outcome.wantNot) {
					t.Fatalf("log = %q, want %q without %q", logged, outcome.wantLevel, outcome.wantNot)
				}
			})
		}
	}
}

type openerFunc func()

func (f openerFunc) Open() { f() }

func TestPromptCodesMatchTheFrontend(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("frontend", "src", "lib", "library", "emulation.ts"))
	if err != nil {
		t.Fatal(err)
	}
	list := regexp.MustCompile(`(?s)EMULATOR_PROMPT_CODES[^=]*=\s*\[(.*?)\]`).FindSubmatch(body)
	if list == nil {
		t.Fatal("EMULATOR_PROMPT_CODES not found")
	}
	var frontend []string
	for _, match := range regexp.MustCompile(`'([^']+)'`).FindAllSubmatch(list[1], -1) {
		frontend = append(frontend, string(match[1]))
	}
	want := slices.Sorted(slices.Values(emulatorPromptCodes))
	slices.Sort(frontend)
	if !slices.Equal(frontend, want) {
		t.Fatalf("frontend %v, go %v", frontend, want)
	}
}
