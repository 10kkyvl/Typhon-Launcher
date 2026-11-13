package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"typhon/internal/cloudsaves"
	"typhon/internal/library"
	"typhon/internal/uierr"
)

type namedHook string

func (namedHook) SessionStarting(context.Context, library.Game) error { return nil }

type hookRecorder []library.LaunchHook

func (r *hookRecorder) AddLaunchHook(h library.LaunchHook) { *r = append(*r, h) }

// The library runs hooks in the order they were added and stops at the first
// block (internal/library TestBlockingHookKeepsLaterHooksFromRunning), so the
// order here is what keeps a blocked launch from costing a pre-launch backup.
func TestCloudSavesCheckRunsBeforeThePreLaunchBackup(t *testing.T) {
	cloud, backup := namedHook("cloud saves"), namedHook("save backup")
	var added hookRecorder

	addLaunchHooks(&added, cloud, backup)

	if len(added) != 2 || added[0] != cloud || added[1] != backup {
		t.Fatalf("launch hooks = %v, want [cloud saves, save backup]", added)
	}
}

func TestMainAddsLaunchHooksOnlyThroughAddLaunchHooks(t *testing.T) {
	body, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(body)
	if !strings.Contains(src, "addLaunchHooks(libraryService, cloudSavesService, saveBackupService)") {
		t.Fatal("main.go does not add the cloud saves and save backup hooks through addLaunchHooks")
	}
	if n := strings.Count(src, "libraryService.AddLaunchHook("); n != 0 {
		t.Fatalf("main.go adds %d launch hooks directly, outside the order addLaunchHooks keeps", n)
	}
}

// What the library hands back for a launch the cloud check stopped:
// LaunchBlockedError's cause, wrapped as the hook wrapped it.
func TestOutsideLaunchStoppedByCloudSavesOpensTheWindowAndLogsOnce(t *testing.T) {
	surfaces := []struct {
		name string
		play func(player promptingPlayer, reveal func())
	}{
		{"tray", func(p promptingPlayer, reveal func()) { playFromTray(p, "g1", reveal) }},
		{"shortcut at cold start", func(p promptingPlayer, reveal func()) { playFromShortcut(p, playRequest{id: "g1"}, reveal) }},
		{"second instance", func(p promptingPlayer, reveal func()) {
			handleSecondLaunch([]string{"typhon.exe", "--play", "g1"}, p, openerFunc(reveal), &NavigationService{}, func(string) {})
		}},
	}
	for _, blockedBy := range []error{cloudsaves.ErrConflictUnresolved, cloudsaves.ErrSyncing} {
		code := uierr.Code(blockedBy)
		for _, surface := range surfaces {
			t.Run(surface.name+"/"+code, func(t *testing.T) {
				log := captureLog(t)
				prompts, events := promptsFor(fakeGames{game: library.Game{ID: "g1"}})
				revealed := 0
				err := fmt.Errorf("launch check: %w", blockedBy)

				surface.play(promptingPlayer{player: fakeLauncher{err: err}, prompts: prompts}, func() { revealed++ })

				if revealed != 1 {
					t.Fatalf("window opened %d times, want once", revealed)
				}
				if len(*events) != 0 {
					t.Fatalf("emulator prompts %+v for a launch the cloud check stopped", *events)
				}
				logged := log.String()
				if n := strings.Count(logged, "level=ERROR"); n != 1 || !strings.Contains(logged, code) {
					t.Fatalf("log = %q, want one error line with %s", logged, code)
				}
				if strings.Contains(logged, "level=WARN") {
					t.Fatalf("log = %q, want the failure logged once, as an error", logged)
				}
			})
		}
	}
}
