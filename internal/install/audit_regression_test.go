package install

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRegressionPortableMoveCommitFailureLosesSource(t *testing.T) {
	s, _, _ := newTestService(t)
	src := filepath.Join(t.TempDir(), "source")
	if err := os.MkdirAll(src, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "game.exe"), []byte("only copy"), 0755); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), "destination")
	if err := os.MkdirAll(dst, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dst, "concurrent-file"), []byte("occupied"), 0644); err != nil {
		t.Fatal(err)
	}
	item := Installation{ID: "audit-move", Type: TypePortable, Mode: ModeMove, ContentRoot: src, Destination: dst}
	s.mu.Lock()
	s.items = append(s.items, &item)
	s.mu.Unlock()
	err := s.runPortable(context.Background(), item.ID, item)
	if err == nil {
		t.Fatal("expected error")
	}
	if b, e := os.ReadFile(filepath.Join(src, "game.exe")); e != nil || string(b) != "only copy" {
		t.Fatalf("source lost: %s %v", b, e)
	}
}
