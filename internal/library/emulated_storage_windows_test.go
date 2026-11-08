package library

import (
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// A junction hides the storage from any look at the path.
func TestJunctionToTheStorageGoesByID(t *testing.T) {
	s := newLibrary(t)
	root := t.TempDir()
	dest := filepath.Join(root, "ux0", "app", "PCSE00001")
	vitaFolder(t, dest, "PCSE00001")
	link := filepath.Join(t.TempDir(), "games")
	//nolint:gosec // G204: аргументы — пути из t.TempDir(); внешнего ввода, который требует валидации по инварианту 32, здесь нет
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", link, dest).CombinedOutput(); err != nil {
		t.Fatalf("mklink: %v: %s", err, out)
	}
	game := addVita(t, s, filepath.Join(link, "eboot.bin"))
	req, _ := playVita(t, s, game.ID, vitaRun{lived: time.Minute, program: vitaProgram(t, prefConfig(root))})
	if !slices.Equal(req.args, vitaByID) {
		t.Fatalf("args = %q: the only copy went as content", req.args)
	}
}

func TestDottedPathToTheStorageGoesByID(t *testing.T) {
	s := newLibrary(t)
	root := t.TempDir()
	vitaFolder(t, filepath.Join(root, "ux0", "app", "PCSE00001"), "PCSE00001")
	game := addVita(t, s, root+`\ux0.\app\PCSE00001\eboot.bin`)
	req, _ := playVita(t, s, game.ID, vitaRun{lived: time.Minute, program: vitaProgram(t, prefConfig(root))})
	if !slices.Equal(req.args, vitaByID) {
		t.Fatalf("args = %q: the only copy went as content", req.args)
	}
}
