package wine

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// fakeBottle собирает минимальный бутыль: dosdevices с занятыми буквами,
// как их раздаёт CrossOver смонтированным томам, и пустой drive_c.
func fakeBottle(t *testing.T, taken ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "bottle")
	dos := filepath.Join(path, "dosdevices")
	if err := os.MkdirAll(dos, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(path, "drive_c"), 0o755); err != nil {
		t.Fatalf("MkdirAll drive_c: %v", err)
	}
	if err := os.Symlink("../drive_c", filepath.Join(dos, "c:")); err != nil {
		t.Fatalf("Symlink c: %v", err)
	}
	for _, letter := range taken {
		if err := os.Symlink("/Volumes/Some", filepath.Join(dos, letter+":")); err != nil {
			t.Fatalf("Symlink %s: %v", letter, err)
		}
	}
	return path
}

func TestFreeDrivePrefersT(t *testing.T) {
	bottle := fakeBottle(t, "d", "e")

	letter, err := freeDrive(filepath.Join(bottle, "dosdevices"))
	if err != nil {
		t.Fatalf("freeDrive: %v", err)
	}
	if letter != "t" {
		t.Fatalf("letter = %q, want %q", letter, "t")
	}
}

func TestFreeDriveSkipsTaken(t *testing.T) {
	bottle := fakeBottle(t, "t", "u", "v", "w", "x")

	letter, err := freeDrive(filepath.Join(bottle, "dosdevices"))
	if err != nil {
		t.Fatalf("freeDrive: %v", err)
	}
	if letter != "l" {
		t.Fatalf("letter = %q, want %q", letter, "l")
	}
}

func TestFreeDriveExhausted(t *testing.T) {
	taken := []string{}
	for _, r := range "lmnopqrstuvwx" {
		taken = append(taken, string(r))
	}
	bottle := fakeBottle(t, taken...)

	_, err := freeDrive(filepath.Join(bottle, "dosdevices"))
	if !errors.Is(err, ErrNoFreeDrive) {
		t.Fatalf("err = %v, want ErrNoFreeDrive", err)
	}
}

func TestEnsureDriveCreatesAndRepoints(t *testing.T) {
	bottle := fakeBottle(t)
	games := t.TempDir()
	link := filepath.Join(bottle, "dosdevices", "t:")

	if err := ensureDrive(bottle, "t", games); err != nil {
		t.Fatalf("ensureDrive: %v", err)
	}
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("Readlink: %v", err)
	}
	if target != games {
		t.Fatalf("target = %q, want %q", target, games)
	}

	// CrossOver при обновлении бутыля может отдать нашу букву тому: буква
	// внутри нашего бутыля наша, поэтому ensureDrive обязан её вернуть.
	if err := os.Remove(link); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if err := os.Symlink("/Volumes/Intruder", link); err != nil {
		t.Fatalf("Symlink intruder: %v", err)
	}
	if err := ensureDrive(bottle, "t", games); err != nil {
		t.Fatalf("ensureDrive after intruder: %v", err)
	}
	target, err = os.Readlink(link)
	if err != nil {
		t.Fatalf("Readlink after repoint: %v", err)
	}
	if target != games {
		t.Fatalf("target = %q, want %q", target, games)
	}
}

func TestEnsureDriveIdempotent(t *testing.T) {
	bottle := fakeBottle(t)
	games := t.TempDir()

	for range 3 {
		if err := ensureDrive(bottle, "t", games); err != nil {
			t.Fatalf("ensureDrive: %v", err)
		}
	}
	target, err := os.Readlink(filepath.Join(bottle, "dosdevices", "t:"))
	if err != nil {
		t.Fatalf("Readlink: %v", err)
	}
	if target != games {
		t.Fatalf("target = %q, want %q", target, games)
	}
}
