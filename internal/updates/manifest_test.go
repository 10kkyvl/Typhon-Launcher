package updates

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func sampleTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, root, "game.exe", "executable")
	writeFile(t, root, "data/pak0.pak", "first archive")
	writeFile(t, root, "data/pak1.pak", "second archive")
	return root
}

func TestBuildManifest(t *testing.T) {
	root := sampleTree(t)
	manifest, err := BuildManifest(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Entries) != 3 {
		t.Fatalf("entries = %d, want 3", len(manifest.Entries))
	}
	want := []string{"data/pak0.pak", "data/pak1.pak", "game.exe"}
	for i, entry := range manifest.Entries {
		if entry.Path != want[i] {
			t.Fatalf("entry %d = %q, want %q", i, entry.Path, want[i])
		}
		if entry.Hash == "" || entry.Size == 0 {
			t.Fatalf("entry %q not hashed: %+v", entry.Path, entry)
		}
	}
	if manifest.TotalSize == 0 {
		t.Fatal("manifest total size is zero")
	}
}

func TestVerifyManifestClean(t *testing.T) {
	root := sampleTree(t)
	manifest, err := BuildManifest(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := VerifyManifest(context.Background(), root, manifest, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Issues) != 0 || result.OkFiles != 3 {
		t.Fatalf("result = %+v", result)
	}
	if result.Ratio() != 1 {
		t.Fatalf("ratio = %f, want 1", result.Ratio())
	}
}

func TestVerifyManifestDetectsDamage(t *testing.T) {
	root := sampleTree(t)
	manifest, err := BuildManifest(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "game.exe")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, "data/pak0.pak", "first archivX")
	writeFile(t, root, "data/pak1.pak", "second archive but longer")
	writeFile(t, root, "user_settings.ini", "kept")

	result, err := VerifyManifest(context.Background(), root, manifest, nil)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]IssueKind{}
	for _, issue := range result.Issues {
		kinds[issue.Path] = issue.Kind
	}
	if kinds["game.exe"] != IssueMissing {
		t.Fatalf("game.exe = %q, want %q", kinds["game.exe"], IssueMissing)
	}
	if kinds["data/pak0.pak"] != IssueCorrupted {
		t.Fatalf("pak0 = %q, want %q", kinds["data/pak0.pak"], IssueCorrupted)
	}
	if kinds["data/pak1.pak"] != IssueSize {
		t.Fatalf("pak1 = %q, want %q", kinds["data/pak1.pak"], IssueSize)
	}
	if len(result.Extra) != 1 || result.Extra[0] != "user_settings.ini" {
		t.Fatalf("extra = %v, want the user file only", result.Extra)
	}
	if result.OkFiles != 0 {
		t.Fatalf("okFiles = %d, want 0", result.OkFiles)
	}
}

func TestVerifyManifestCancellation(t *testing.T) {
	root := sampleTree(t)
	manifest, err := BuildManifest(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := VerifyManifest(ctx, root, manifest, nil); err == nil {
		t.Fatal("expected verification to honour a cancelled context")
	}
}

func TestManifestManaged(t *testing.T) {
	manifest := FileManifest{Entries: []FileManifestEntry{{Path: "data/pak0.pak"}}}
	if !manifest.Managed("data/pak0.pak") {
		t.Fatal("managed file not recognised")
	}
	if manifest.Managed("mods/extra.pak") {
		t.Fatal("unknown file reported as managed")
	}
}
