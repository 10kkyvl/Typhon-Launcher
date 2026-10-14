package lan

import (
	"os"
	"path/filepath"
	"testing"
)

// Test the synchronous destination reservation independently of sockets.
// TestLoopbackEndToEnd covers delivery and registration through real peers.
func TestReceivePreservesOccupiedDestinations(t *testing.T) {
	const hash = "0123456789abcdef0123456789abcdef01234567"
	primary := filepath.Join(t.TempDir(), "Game")
	fallback := primary + "-" + hash[:8]
	occupied := []string{primary, fallback, fallback + "-1"}
	for _, dir := range occupied {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "game.exe"), []byte("MY EXISTING GAME"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	dest, err := reserveReceiveDirectory(primary, hash)
	if err != nil {
		t.Fatal(err)
	}
	if dest != fallback+"-2" {
		t.Fatalf("destination = %s, want unused numbered directory", dest)
	}
	if info, err := os.Stat(dest); err != nil || !info.IsDir() {
		t.Fatalf("destination not reserved: %v", err)
	}
	for _, dir := range occupied {
		got, err := os.ReadFile(filepath.Join(dir, "game.exe"))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != "MY EXISTING GAME" {
			t.Fatalf("existing game changed in %s", dir)
		}
	}
}
