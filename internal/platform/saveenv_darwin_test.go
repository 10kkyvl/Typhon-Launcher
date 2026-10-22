//go:build darwin

package platform

import (
	"path/filepath"
	"testing"

	"typhon/internal/wine"
)

func TestSaveEnvsFromHasOneHostDocuments(t *testing.T) {
	own := wine.Bottle{Path: "/bottles/B"}
	shared := wine.Bottle{Path: "/bottles/S", Shared: true}

	envs := saveEnvsFrom("/Users/x", []wine.Bottle{own, shared})

	if len(envs) != 3 {
		t.Fatalf("got %d envs, want host + 2 bottles", len(envs))
	}
	if got := envs[0][TokenDocuments]; got != filepath.Join("/Users/x", "Documents") {
		t.Fatalf("host documents = %q", got)
	}
	for i, env := range envs[1:] {
		if _, ok := env[TokenDocuments]; ok {
			t.Fatalf("bottle env %d carries its own documents", i)
		}
	}
	user := filepath.Join("/bottles/B", "drive_c", "users", "crossover")
	if got := envs[1][TokenAppData]; got != filepath.Join(user, "AppData", "Roaming") {
		t.Fatalf("appdata = %q", got)
	}
	if _, ok := envs[1][TokenSteam]; ok {
		t.Fatal("own bottle must not carry a steam root")
	}
	if got := envs[2][TokenSteam]; got != filepath.Join("/bottles/S", "drive_c", "Program Files (x86)", "Steam") {
		t.Fatalf("shared steam = %q", got)
	}
}

func TestSaveEnvsFromWithoutBottlesStillHasHostEnv(t *testing.T) {
	envs := saveEnvsFrom("/Users/x", nil)
	if len(envs) != 1 || envs[0][TokenDocuments] == "" {
		t.Fatalf("envs = %+v, want the host env so that {game} hints still resolve", envs)
	}
}
