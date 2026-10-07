//go:build windows

package install

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestMainRunSpecInteractiveMsiRunsMsiexecInstall(t *testing.T) {
	root := t.TempDir()
	installer := filepath.Join(root, "game.msi")
	rs, err := mainRunSpec(workerSpec{
		Interactive: true, Engine: EngineMsi, InstallerPath: installer, WorkingDir: root,
		Hidden: true, Background: true,
	}, nil)
	if err != nil {
		t.Fatalf("mainRunSpec: %v", err)
	}
	if !strings.EqualFold(filepath.Base(rs.Path), "msiexec.exe") {
		t.Fatalf("path = %q, want msiexec.exe", rs.Path)
	}
	if len(rs.Args) != 2 || rs.Args[0] != "/i" || rs.Args[1] != installer {
		t.Fatalf("args = %q, want /i %s with no silent switches", rs.Args, installer)
	}
	if rs.Hidden || rs.Background || rs.CmdLine != "" {
		t.Fatalf("interactive msi run is hidden, backgrounded or carries a command line: %+v", rs)
	}
}
