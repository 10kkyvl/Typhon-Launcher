package relocate

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"typhon/internal/hashdir"
)

func TestRegressionRecoveryDeletesGoodSourceForBadTarget(t *testing.T) {
	lib := newTestLibrary(t)
	game, src := addTestGame(t, lib)
	dst := filepath.Join(t.TempDir(), "target")
	os.MkdirAll(dst, 0755)
	os.WriteFile(filepath.Join(dst, "game.exe"), []byte("CORRUPT"), 0755)
	s, err := NewServiceAt(t.TempDir(), nil, lib, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	m, err := hashdir.Build(context.Background(), src, nil)
	if err != nil {
		t.Fatal(err)
	}
	j := Job{ID: "audit", Scope: ScopeGame, Stage: StageRepoint, GameID: game.ID, Source: src, Target: dst}
	s.jobs = []Job{j}
	if err := s.st.saveManifest(j.ID, m); err != nil {
		t.Fatal(err)
	}
	s.recoverRepoint(context.Background(), j)
	if _, err := os.Stat(src); err != nil {
		t.Fatal("source lost", err)
	}
	t.Log("Regression: recovery deleted good source without verifying corrupt target")
}
func TestRegressionLibraryRetryMovesAlreadyMovedGameAgain(t *testing.T) {
	lib := newTestLibrary(t)
	set := newTestSettings(t)
	s := newTestService(t, set, lib, nil, nil, nil)
	old := set.GetSettings().LibraryPath
	root := filepath.Join(t.TempDir(), "TyphonLibrary")
	game := addGameAt(t, lib, filepath.Join(root, "Games"), "Game")
	j := Job{ID: "retry", Scope: ScopeLibrary, Stage: StagePrepare, Source: old, Target: root}
	if err := s.registerJob(j); err != nil {
		t.Fatal(err)
	}
	if err := s.runGameLibraryItem(context.Background(), j.ID, game.ID, nil, old, root); err != nil {
		t.Fatal(err)
	}
	g, _ := lib.Find(game.ID)
	if g.InstallDir != filepath.Join(root, "Games", "Game") {
		t.Fatalf("not reproduced: %s", g.InstallDir)
	}
	t.Log("Regression: retry relocates previously moved Games/Game to root/Game instead of skipping it")
}
