//go:build windows

package jumplist

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func throwawayConfig(t *testing.T) Config {
	t.Helper()
	appID := "Typhon.JumpListTest." + strconv.FormatInt(time.Now().UnixNano(), 36)
	launcher, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := deleteList(appID); err != nil {
			t.Errorf("delete the throwaway list %s: %v", appID, err)
		}
	})
	return Config{
		AppID:    appID,
		Launcher: launcher,
		Labels:   Labels{Recent: "Recent games", Library: "Library", Downloads: "Downloads"},
	}
}

func TestApplyCommitsAList(t *testing.T) {
	cfg := throwawayConfig(t)
	gameExe := filepath.Join(os.Getenv("SystemRoot"), "System32", "notepad.exe")
	cases := []struct {
		name  string
		games []Game
	}{
		{"no games gives only the tasks", nil},
		{"games with and without an icon", []Game{
			{ID: "g1", Title: "Dungeons & Dragons", Executable: gameExe},
			{ID: "g2", Title: "Второй Эпизод"},
		}},
		{"the list is replaced on the next commit", []Game{{ID: "g3", Title: "Third"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := Apply(cfg, tc.games); err != nil {
				t.Fatalf("Apply: %v", err)
			}
		})
	}
}

func TestApplyRefusesAnInvalidConfigBeforeTouchingTheShell(t *testing.T) {
	cfg := throwawayConfig(t)
	cfg.Launcher = "typhon.exe"
	if err := Apply(cfg, nil); err == nil {
		t.Fatal("Apply accepted a relative launcher path")
	}
}

func TestApplySkipsAGameWithABadID(t *testing.T) {
	cfg := throwawayConfig(t)
	if err := Apply(cfg, []Game{{ID: "bad id", Title: "Bad"}, {ID: "ok", Title: "Ok"}}); err != nil {
		t.Fatalf("Apply with one unusable id: %v", err)
	}
}
