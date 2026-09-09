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
	os.MkdirAll(src, 0755)
	os.WriteFile(filepath.Join(src, "game.exe"), []byte("only copy"), 0755)
	dst := filepath.Join(t.TempDir(), "destination")
	os.MkdirAll(dst, 0755)
	os.WriteFile(filepath.Join(dst, "concurrent-file"), []byte("occupied"), 0644)
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
