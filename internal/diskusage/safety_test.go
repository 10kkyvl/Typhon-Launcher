package diskusage

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"typhon/internal/diskreason"
	"typhon/internal/library"
	"typhon/internal/relocate"
	"typhon/internal/uierr"
)

func byPath(rep Report) map[string]Leftover {
	out := map[string]Leftover{}
	for _, l := range rep.Leftovers {
		out[l.Path] = l
	}
	return out
}

func requireKept(t *testing.T, r *rig, path string) {
	t.Helper()
	if err := r.svc.DeleteLeftover(context.Background(), path); !errors.Is(err, errNotDeletable) {
		t.Fatalf("DeleteLeftover(%s) = %v, want errNotDeletable", path, err)
	}
	if !exists(path) {
		t.Fatalf("%s was deleted", path)
	}
}

func TestCopiesAMoveLeftAreListedByKindAndNeverDeletable(t *testing.T) {
	r := newRig(t)
	at := func(name string) string {
		p := filepath.Join(t.TempDir(), name)
		writeBytes(t, filepath.Join(p, "game.exe"), 10)
		return p
	}
	only := at("Game")
	holding := at("Game.staging")
	movedOn := at("Old")
	unknownSource := at("Older")
	r.rel.copies = []relocate.OrphanedCopy{
		{JobID: "j1", GameID: "g1", Title: "Game One", Path: only, Kind: relocate.CopyTarget, OnlyCopy: true},
		{JobID: "j2", GameID: "g2", Title: "Game Two", Path: holding, Kind: relocate.CopyStaging, HoldsGame: true},
		{JobID: "j3", GameID: "g3", Title: "Game Three", Path: movedOn, Kind: relocate.CopySource, MovedOn: true},
		{JobID: "j4", GameID: "g4", Title: "Game Four", Path: unknownSource, Kind: relocate.CopySource},
	}

	rep, err := r.svc.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got := byPath(rep)
	want := []struct {
		path, kind, code, game, title string
	}{
		{only, KindRelocateCopyTarget, diskreason.OnlyCopy, "g1", "Game One"},
		{holding, KindRelocateCopyStaging, diskreason.HoldsGame, "g2", "Game Two"},
		{movedOn, KindRelocateCopySource, reasonRelocateCopy, "g3", "Game Three"},
		{unknownSource, KindRelocateCopySource, diskreason.OnlyCopy, "g4", "Game Four"},
	}
	for _, w := range want {
		l, ok := got[w.path]
		if !ok || l.Kind != w.kind || l.Deletable || l.ReasonCode != w.code || l.GameID != w.game || l.Title != w.title || l.SizeBytes != 10 {
			t.Errorf("copy %s = %+v (listed %v), want kind %s, reason %s, game %s %q, not deletable", filepath.Base(w.path), l, ok, w.kind, w.code, w.game, w.title)
		}
	}
	for _, w := range want {
		requireKept(t, r, w.path)
	}
}

func TestCopiesAMoveLeftThatCannotBeCheckedAreAWarning(t *testing.T) {
	r := newRig(t)
	r.rel.copiesErr = errors.New("library unavailable")
	rep, err := r.svc.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Warnings) != 1 || rep.Warnings[0].Code != WarnMovesUnchecked {
		t.Fatalf("warnings = %+v", rep.Warnings)
	}
}

func TestRemovalFoldersAreListedAndNeverDeletable(t *testing.T) {
	r := newRig(t)
	pending := filepath.Join(r.games, "Game.removing")
	stray := filepath.Join(r.games, "Other.removing-2")
	writeBytes(t, filepath.Join(pending, "a.bin"), 5)
	writeBytes(t, filepath.Join(stray, "b.bin"), 7)
	writeBytes(t, filepath.Join(r.games, "notes.removing"), 1)
	writeBytes(t, filepath.Join(r.games, "Game.removing-x", "c.bin"), 1)
	r.inst.pending = []string{pending}

	rep, err := r.svc.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got := byPath(rep)
	if l := got[pending]; l.Kind != KindRemoval || l.Deletable || l.ReasonCode != reasonRemovalPending || l.SizeBytes != 5 {
		t.Errorf("pending removal = %+v", l)
	}
	if l := got[stray]; l.Kind != KindRemoval || l.Deletable || l.ReasonCode != reasonRemovalUnjournaled || l.SizeBytes != 7 {
		t.Errorf("removal no journal names = %+v", l)
	}
	if len(rep.Leftovers) != 2 {
		t.Errorf("leftovers = %+v, want only the two folders a removal names", rep.Leftovers)
	}
	requireKept(t, r, pending)
	requireKept(t, r, stray)
}

func TestUnreadableRemovalJournalIsAWarningAndItsFoldersAreKept(t *testing.T) {
	r := newRig(t)
	folder := filepath.Join(r.games, "Game.removing")
	writeBytes(t, filepath.Join(folder, "a.bin"), 5)
	r.inst.pendingErr = uierr.New("install.removals_unreadable", "журнал не читается")

	rep, err := r.svc.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Warnings) != 1 || rep.Warnings[0].Code != WarnRemovalsUnreadable || rep.Warnings[0].Area != "leftovers" {
		t.Fatalf("warnings = %+v", rep.Warnings)
	}
	if l := byPath(rep)[folder]; l.Deletable || l.ReasonCode != reasonRemovalsUnreadable {
		t.Fatalf("removal folder = %+v", l)
	}
	requireKept(t, r, folder)
}

func TestStagingFolderNoJournalNamesIsListedAndNeverDeletable(t *testing.T) {
	r := newRig(t)
	lost := filepath.Join(r.games, "Lost.staging")
	listed := filepath.Join(r.games, "Listed.staging")
	journaled := filepath.Join(r.games, "Journaled.staging")
	updates := filepath.Join(r.games, ".staging")
	for _, p := range []string{lost, listed, journaled, filepath.Join(updates, "unknown-game")} {
		writeBytes(t, filepath.Join(p, "x.bin"), 3)
	}
	r.rel.list = []relocate.Staging{{Path: listed, Deletable: true}}
	r.rel.jobs = []relocate.Job{{ID: "j", Staging: journaled, Stage: relocate.StageCopy}}

	rep, err := r.svc.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got := byPath(rep)
	if l := got[lost]; l.Kind != KindRelocateStaging || l.Deletable || l.ReasonCode != reasonRelocateUnjournaled {
		t.Errorf("staging no journal names = %+v", l)
	}
	if l := got[listed]; !l.Deletable {
		t.Errorf("the journaled staging the move service lists = %+v, want its own verdict", l)
	}
	for _, p := range []string{journaled, updates} {
		if l, ok := got[p]; ok {
			t.Errorf("%s listed as %+v, want it left to its owner", filepath.Base(p), l)
		}
	}
	requireKept(t, r, lost)
}

func TestStrayFolderAGameLiesInHoldsTheGame(t *testing.T) {
	r := newRig(t)
	stray := filepath.Join(r.games, "Lost.staging")
	writeBytes(t, filepath.Join(stray, "Game", "game.exe"), 3)
	r.lib.games = []library.Game{{ID: "g", Title: "G", InstallDir: filepath.Join(stray, "Game")}}
	rep, err := r.svc.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if l := byPath(rep)[stray]; l.Deletable || l.ReasonCode != diskreason.HoldsGame {
		t.Fatalf("stray staging around a game = %+v", l)
	}
}

func TestLeftoverIsNotDeletedWhileALibraryHoldOverlapsIt(t *testing.T) {
	r := newRig(t)
	path := filepath.Join(r.games, "Done.partial")
	writeBytes(t, filepath.Join(path, "a.bin"), 4)
	r.inst.partial[path] = true
	if _, err := r.svc.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	r.lib.mu.Lock()
	r.lib.held = map[string]bool{r.games: true}
	r.lib.mu.Unlock()
	requireKept(t, r, path)
	if len(r.inst.released) != len(r.inst.reserved) {
		t.Fatalf("reserved %v, released %v", r.inst.reserved, r.inst.released)
	}

	rep, err := r.svc.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if l := byPath(rep)[path]; l.Deletable || l.ReasonCode != reasonFolderHeld {
		t.Fatalf("leftover under a hold = %+v", l)
	}
	r.lib.mu.Lock()
	r.lib.held = nil
	r.lib.mu.Unlock()
	if _, err := r.svc.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := r.svc.DeleteLeftover(context.Background(), path); err != nil {
		t.Fatalf("DeleteLeftover once the hold is gone = %v", err)
	}
}
