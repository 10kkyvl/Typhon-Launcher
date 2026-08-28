package accountsync

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestStoreLoad(t *testing.T) {
	t.Run("missing file returns empty state", func(t *testing.T) {
		dir := t.TempDir()
		st, err := newStore(dir).load()
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		if st.DeviceID != "" || st.SettingsRevision != 0 || len(st.Games) != 0 {
			t.Fatalf("expected empty state, got %+v", st)
		}
	})

	t.Run("corrupt json fails, does not fall back to empty", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "sync.json")
		if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
			t.Fatalf("seed corrupt file: %v", err)
		}
		if _, err := newStore(dir).load(); err == nil {
			t.Fatal("expected error loading corrupt state, got nil")
		}
	})

	t.Run("empty dir path is rejected", func(t *testing.T) {
		if _, err := newStore("").load(); err == nil {
			t.Fatal("expected error for empty dir")
		}
	})

	t.Run("save then load round trips", func(t *testing.T) {
		dir := t.TempDir()
		st := syncState{
			DeviceID:         "11111111-1111-1111-1111-111111111111",
			SettingsRevision: 4,
			Games: map[string]gameState{
				"123": {DeviceSeconds: 60, Baseline: 30},
			},
		}
		s := newStore(dir)
		if err := s.save(st); err != nil {
			t.Fatalf("save: %v", err)
		}
		got, err := s.load()
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		if got.DeviceID != st.DeviceID || got.SettingsRevision != st.SettingsRevision {
			t.Fatalf("round trip mismatch: got %+v want %+v", got, st)
		}
		if got.Games["123"] != (gameState{DeviceSeconds: 60, Baseline: 30}) {
			t.Fatalf("round trip game state mismatch: got %+v", got.Games["123"])
		}
	})

	t.Run("save with empty dir path is rejected", func(t *testing.T) {
		if err := newStore("").save(emptyState()); err == nil {
			t.Fatal("expected error for empty dir")
		}
	})

	t.Run("no permission to read directory surfaces an error", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "sync.json")
		if err := os.WriteFile(path, []byte(`{"version":1,"data":{}}`), 0o000); err != nil {
			t.Fatalf("seed unreadable file: %v", err)
		}
		t.Cleanup(func() {
			if err := os.Chmod(path, 0o600); err != nil {
				t.Logf("restore permissions on %s: %v", path, err)
			}
		})
		_, err := newStore(dir).load()
		if err == nil {
			t.Skip("platform allows reading despite permission bits (likely running as admin)")
		}
		if errors.Is(err, os.ErrNotExist) {
			t.Fatalf("expected a permission-style error, got ErrNotExist: %v", err)
		}
	})
}
