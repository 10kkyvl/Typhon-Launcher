package metadata

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func sampleAsset(gameID, id string, kind AssetType) MediaAsset {
	return MediaAsset{
		ID:        id,
		GameID:    gameID,
		Type:      kind,
		SourceURL: "https://images.example/" + id + ".png",
		Path:      "games/" + gameID + "/" + id + ".png",
		Width:     1920,
		Height:    1080,
		CreatedAt: time.Unix(1700000000, 0).UTC(),
	}
}

func writeAssetFile(t *testing.T, root, rel string) string {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("create dir: %v", err)
	}
	if err := os.WriteFile(full, []byte("payload"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	return full
}

func TestNewAssetStoreRejectsBrokenIndex(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, assetsFileName), []byte("{broken"), 0o600); err != nil {
		t.Fatalf("write index: %v", err)
	}
	if _, err := newAssetStore(dir); err == nil {
		t.Fatal("broken index was accepted as an empty store")
	}
}

func TestNewAssetStoreAcceptsMissingIndex(t *testing.T) {
	store, err := newAssetStore(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	if got := store.list("any"); got != nil {
		t.Fatalf("assets = %v, want none", got)
	}
}

func TestNewAssetStoreRejectsEmptyDir(t *testing.T) {
	if _, err := newAssetStore("  "); !errors.Is(err, errStorePath) {
		t.Fatalf("err = %v, want errStorePath", err)
	}
}

func TestReplaceReturnsPreviousAndPersists(t *testing.T) {
	dir := t.TempDir()
	store, err := newAssetStore(dir)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	first := []MediaAsset{sampleAsset("game1", "a1", AssetCover), sampleAsset("game1", "a2", AssetScreenshot)}
	if _, err := store.replace("game1", first); err != nil {
		t.Fatalf("replace: %v", err)
	}
	if _, err := store.replace("game2", []MediaAsset{sampleAsset("game2", "b1", AssetCover)}); err != nil {
		t.Fatalf("replace other game: %v", err)
	}

	previous, err := store.replace("game1", []MediaAsset{sampleAsset("game1", "a3", AssetCover)})
	if err != nil {
		t.Fatalf("replace again: %v", err)
	}
	if len(previous) != 2 {
		t.Fatalf("previous = %d, want 2", len(previous))
	}
	if got := store.list("game1"); len(got) != 1 || got[0].ID != "a3" {
		t.Fatalf("current = %+v", got)
	}
	if got := store.list("game2"); len(got) != 1 {
		t.Fatalf("other game touched: %+v", got)
	}

	reopened, err := newAssetStore(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if got := reopened.list("game1"); len(got) != 1 || got[0].ID != "a3" {
		t.Fatalf("persisted = %+v", got)
	}
	if got := reopened.list("game1"); got[0].URL != "/media/games/game1/a3.png" {
		t.Fatalf("url = %q", got[0].URL)
	}
}

func TestReplaceRollsBackOnWriteFailure(t *testing.T) {
	dir := t.TempDir()
	store, err := newAssetStore(dir)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	if _, err := store.replace("game1", []MediaAsset{sampleAsset("game1", "a1", AssetCover)}); err != nil {
		t.Fatalf("replace: %v", err)
	}

	indexPath := filepath.Join(dir, assetsFileName)
	if err := os.Remove(indexPath); err != nil {
		t.Fatalf("remove index: %v", err)
	}
	if err := os.Mkdir(indexPath, 0o755); err != nil {
		t.Fatalf("block index path: %v", err)
	}

	if _, err := store.replace("game1", []MediaAsset{sampleAsset("game1", "a2", AssetCover)}); err == nil {
		t.Fatal("replace succeeded despite an unwritable index")
	}
	got := store.list("game1")
	if len(got) != 1 || got[0].ID != "a1" {
		t.Fatalf("in-memory state = %+v, want the previous asset", got)
	}
}

func TestSweepRemovesOnlyOrphans(t *testing.T) {
	dir := t.TempDir()
	store, err := newAssetStore(dir)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	known := sampleAsset("game1", "a1", AssetCover)
	if _, err := store.replace("game1", []MediaAsset{known}); err != nil {
		t.Fatalf("replace: %v", err)
	}

	keep := writeAssetFile(t, store.mediaRoot(), known.Path)
	orphan := writeAssetFile(t, store.mediaRoot(), "games/game1/stale.png")
	candidate := writeAssetFile(t, store.mediaRoot(), candidatesDirName+"/thumb.jpg")

	if err := store.sweep(context.Background()); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatalf("known asset removed: %v", err)
	}
	if _, err := os.Stat(orphan); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("orphan kept: %v", err)
	}
	if _, err := os.Stat(candidate); err != nil {
		t.Fatalf("candidate cache swept: %v", err)
	}
}

func TestSweepHonoursCancelledContext(t *testing.T) {
	dir := t.TempDir()
	store, err := newAssetStore(dir)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	orphan := writeAssetFile(t, store.mediaRoot(), "games/game1/stale.png")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := store.sweep(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if _, err := os.Stat(orphan); err != nil {
		t.Fatalf("file removed under a cancelled context: %v", err)
	}
}

func TestSweepWithoutMediaDir(t *testing.T) {
	store, err := newAssetStore(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	if err := store.sweep(context.Background()); err != nil {
		t.Fatalf("sweep: %v", err)
	}
}

func TestClearCandidates(t *testing.T) {
	dir := t.TempDir()
	store, err := newAssetStore(dir)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	thumb := writeAssetFile(t, store.mediaRoot(), candidatesDirName+"/thumb.jpg")
	if err := store.clearCandidates(); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if _, err := os.Stat(thumb); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("candidate cache kept: %v", err)
	}
}

func TestRemoveFilesIgnoresMissing(t *testing.T) {
	dir := t.TempDir()
	store, err := newAssetStore(dir)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	asset := sampleAsset("game1", "a1", AssetCover)
	full := writeAssetFile(t, store.mediaRoot(), asset.Path)

	store.removeFiles([]MediaAsset{asset, sampleAsset("game1", "gone", AssetCover)})
	if _, err := os.Stat(full); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("file kept: %v", err)
	}
}

func TestNewAssetIDIsFilesystemSafe(t *testing.T) {
	seen := map[string]bool{}
	for range 32 {
		id, err := newAssetID()
		if err != nil {
			t.Fatalf("new id: %v", err)
		}
		if seen[id] {
			t.Fatalf("duplicate id %q", id)
		}
		seen[id] = true
		if _, err := writeAsset(t.TempDir(), "game1", id, fetchedImage{Data: []byte("x"), Format: "png"}); err != nil {
			t.Fatalf("id %q rejected as a file name: %v", id, err)
		}
	}
}
