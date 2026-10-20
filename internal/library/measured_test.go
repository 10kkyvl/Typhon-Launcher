package library

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestUpdateMeasuredSize(t *testing.T) {
	setup := func(t *testing.T) (*Service, Game) {
		t.Helper()
		service := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
		dir := t.TempDir()
		exe := filepath.Join(dir, "game.exe")
		if err := os.WriteFile(exe, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		game, err := service.RegisterInstalled(t.Context(), InstalledGame{Title: "Game", InstallDir: dir, Executable: exe})
		if err != nil {
			t.Fatal(err)
		}
		return service, game
	}

	t.Run("changed size is stored and persisted", func(t *testing.T) {
		service, game := setup(t)
		changed, err := service.UpdateMeasuredSize(game.ID, game.InstallDir, 777, false)
		if err != nil || !changed {
			t.Fatalf("UpdateMeasuredSize = %v, %v", changed, err)
		}
		got, err := service.Find(game.ID)
		if err != nil || got.SizeBytes != 777 || got.SizeUnknown {
			t.Fatalf("game = %+v, %v", got, err)
		}
		reopened, err := NewServiceAt(service.path)
		if err != nil {
			t.Fatal(err)
		}
		if again, err := reopened.Find(game.ID); err != nil || again.SizeBytes != 777 {
			t.Fatalf("reopened = %+v, %v", again, err)
		}
	})
	t.Run("same value is not rewritten", func(t *testing.T) {
		service, game := setup(t)
		changed, err := service.UpdateMeasuredSize(game.ID, game.InstallDir, game.SizeBytes, game.SizeUnknown)
		if err != nil || changed {
			t.Fatalf("UpdateMeasuredSize = %v, %v; want no change", changed, err)
		}
	})
	t.Run("unknown size is recorded as unknown, not zero", func(t *testing.T) {
		service, game := setup(t)
		if _, err := service.UpdateMeasuredSize(game.ID, game.InstallDir, 0, true); err != nil {
			t.Fatal(err)
		}
		got, err := service.Find(game.ID)
		if err != nil || !got.SizeUnknown {
			t.Fatalf("game = %+v, %v", got, err)
		}
	})
	t.Run("game moved since the measurement", func(t *testing.T) {
		service, game := setup(t)
		changed, err := service.UpdateMeasuredSize(game.ID, filepath.Join(t.TempDir(), "elsewhere"), 5, false)
		if err != nil || changed {
			t.Fatalf("UpdateMeasuredSize = %v, %v; want the stale value dropped", changed, err)
		}
		got, err := service.Find(game.ID)
		if err != nil || got.SizeBytes != game.SizeBytes {
			t.Fatalf("size = %d (%v), want %d", got.SizeBytes, err, game.SizeBytes)
		}
	})
	t.Run("unknown game", func(t *testing.T) {
		service, _ := setup(t)
		if _, err := service.UpdateMeasuredSize("nope", "x", 1, false); !errors.Is(err, errNotFound) {
			t.Fatalf("error = %v, want errNotFound", err)
		}
	})
	t.Run("save failure rolls the record back", func(t *testing.T) {
		service, game := setup(t)
		service.mu.Lock()
		service.path = t.TempDir()
		service.mu.Unlock()
		changed, err := service.UpdateMeasuredSize(game.ID, game.InstallDir, 12345, false)
		if err == nil || changed {
			t.Fatalf("UpdateMeasuredSize = %v, %v; want the save error", changed, err)
		}
		got, findErr := service.Find(game.ID)
		if findErr != nil || got.SizeBytes != game.SizeBytes {
			t.Fatalf("size = %d (%v) after a failed save, want %d", got.SizeBytes, findErr, game.SizeBytes)
		}
	})
}
