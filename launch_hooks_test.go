package main

import (
	"context"
	"os"
	"strings"
	"testing"

	"typhon/internal/library"
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

// The cloud saves launch check finds a recorded conflict offline only through
// the stored credential's account; passes keep the confirmed one.
func TestMainGivesCloudSavesTheConfirmedAndTheStoredAccount(t *testing.T) {
	body, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(body)
	start := strings.Index(src, "cloudsaves.NewService(")
	if start < 0 {
		t.Fatal("main.go does not build the cloud saves service")
	}
	end := strings.Index(src[start:], "\n\t)\n")
	if end < 0 {
		t.Fatal("end of the cloudsaves.NewService call not found")
	}
	call := src[start : start+end]
	confirmed := strings.Index(call, "accountService.SyncAccountID(token)")
	stored := strings.Index(call, "\n\t\taccountService.DurableAccountID,\n")
	if confirmed < 0 || stored < confirmed {
		t.Fatalf("cloudsaves.NewService call = %s\nwant the confirmed account (SyncAccountID), then the stored one (DurableAccountID)", call)
	}
}
