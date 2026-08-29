package hashdir

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

func TestBuild(t *testing.T) {
	root := sampleTree(t)
	m, err := Build(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Entries) != 3 {
		t.Fatalf("entries = %d, want 3", len(m.Entries))
	}
	want := []string{"data/pak0.pak", "data/pak1.pak", "game.exe"}
	for i, entry := range m.Entries {
		if entry.Path != want[i] {
			t.Fatalf("entry %d = %q, want %q", i, entry.Path, want[i])
		}
		if entry.Hash == "" || entry.Size == 0 {
			t.Fatalf("entry %q not hashed: %+v", entry.Path, entry)
		}
	}
	if m.TotalSize == 0 {
		t.Fatal("total size is zero")
	}
}

func TestBuildEmptyDir(t *testing.T) {
	root := t.TempDir()
	m, err := Build(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Entries) != 0 {
		t.Fatalf("entries = %d, want 0", len(m.Entries))
	}
	if m.TotalSize != 0 {
		t.Fatalf("total size = %d, want 0", m.TotalSize)
	}
}

func TestBuildMissingRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "does-not-exist")
	if _, err := Build(context.Background(), root, nil); err == nil {
		t.Fatal("expected error for a missing root")
	}
}

func TestBuildCancelledContext(t *testing.T) {
	root := sampleTree(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Build(ctx, root, nil); err == nil {
		t.Fatal("expected build to honour a cancelled context")
	}
}

func TestVerifyClean(t *testing.T) {
	root := sampleTree(t)
	m, err := Build(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := Verify(context.Background(), root, m, nil)
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

func TestVerifyDetectsCorruptedMissingAndExtra(t *testing.T) {
	root := sampleTree(t)
	m, err := Build(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "game.exe")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, "data/pak0.pak", "first archivX")
	writeFile(t, root, "data/pak1.pak", "second archive but longer")
	writeFile(t, root, "user_settings.ini", "kept")

	result, err := Verify(context.Background(), root, m, nil)
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
	if got := result.Count(IssueMissing); got != 1 {
		t.Fatalf("Count(IssueMissing) = %d, want 1", got)
	}
}

func TestVerifyCancelledContext(t *testing.T) {
	root := sampleTree(t)
	m, err := Build(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Verify(ctx, root, m, nil); err == nil {
		t.Fatal("expected verify to honour a cancelled context")
	}
}

func TestVerifyMissingRoot(t *testing.T) {
	root := sampleTree(t)
	m, err := Build(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	if _, err := Verify(context.Background(), missing, m, nil); err == nil {
		t.Fatal("expected error walking a missing root")
	}
}

func TestManifestManaged(t *testing.T) {
	m := Manifest{Entries: []Entry{{Path: "data/pak0.pak"}}}
	if !m.Managed("data/pak0.pak") {
		t.Fatal("managed file not recognised")
	}
	if m.Managed("mods/extra.pak") {
		t.Fatal("unknown file reported as managed")
	}
}
