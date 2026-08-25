package install

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mkInstaller(t *testing.T, path, marker string) {
	t.Helper()
	mkText(t, path, strings.Repeat("\x00", 512)+marker+strings.Repeat("\x00", 4096))
}

func innoSource(t *testing.T, root string) string {
	t.Helper()
	dir := filepath.Join(root, "Game")
	mkInstaller(t, filepath.Join(dir, "setup.exe"), "Inno Setup Setup Data (5.6.2) (u)")
	return dir
}

func TestInspectMarksInnoInstallerSilent(t *testing.T) {
	root := t.TempDir()
	innoSource(t, root)

	plan, err := Inspect(context.Background(), root)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if plan.Type != TypeExeInstaller {
		t.Fatalf("type = %s, want %s", plan.Type, TypeExeInstaller)
	}
	if plan.Engine != EngineInno {
		t.Fatalf("engine = %s, want %s", plan.Engine, EngineInno)
	}
	if !plan.Silent || plan.RequiresUserInteraction || !plan.CanAutoInstall {
		t.Fatalf("silent=%v interaction=%v auto=%v", plan.Silent, plan.RequiresUserInteraction, plan.CanAutoInstall)
	}
}

func TestInspectKeepsUnknownInstallerInteractive(t *testing.T) {
	root := t.TempDir()
	mkFile(t, filepath.Join(root, "Game", "setup.exe"), 4096)

	plan, err := Inspect(context.Background(), root)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if plan.Engine != EngineUnknown || plan.Silent {
		t.Fatalf("engine = %s, silent = %v", plan.Engine, plan.Silent)
	}
	if !plan.RequiresUserInteraction || plan.CanAutoInstall {
		t.Fatalf("interaction=%v auto=%v", plan.RequiresUserInteraction, plan.CanAutoInstall)
	}
}

func TestSilentInstallUsesChosenDirectory(t *testing.T) {
	s, downloads, registrar := newTestService(t)
	root := t.TempDir()
	innoSource(t, root)
	downloads.add("d1", "Game", root)

	games := t.TempDir()
	dest := filepath.Join(games, "Game")
	s.roots = []string{games}
	s.runner = &fakeRunner{act: func(spec runSpec) {
		mkFile(t, filepath.Join(spec.Dir, "ignored.txt"), 1)
		mkFile(t, filepath.Join(dest, "Game.exe"), 8192)
	}}

	item, err := s.Start("d1", StartOptions{Destination: dest})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	done := s.waitStatus(t, item.ID, StatusCompleted)
	if done.Destination != dest {
		t.Fatalf("destination = %s, want %s", done.Destination, dest)
	}
	if len(registrar.games) != 1 || registrar.games[0].InstallDir != dest {
		t.Fatalf("registered = %+v", registrar.games)
	}

	fake, ok := s.runner.(*fakeRunner)
	if !ok {
		t.Fatalf("runner = %T", s.runner)
	}
	calls := fake.calls()
	if len(calls) != 1 {
		t.Fatalf("runner calls = %d", len(calls))
	}
	args := strings.Join(calls[0].Args, " ")
	if !strings.Contains(args, "/VERYSILENT") || !strings.Contains(args, "/DIR="+dest) {
		t.Fatalf("args = %q", args)
	}
	if !calls[0].Background || !calls[0].Hidden {
		t.Fatalf("silent installer must run hidden in background: %+v", calls[0])
	}
}

func TestSilentInstallRequiresDestination(t *testing.T) {
	s, downloads, _ := newTestService(t)
	root := t.TempDir()
	innoSource(t, root)
	downloads.add("d1", "Game", root)
	s.roots = []string{t.TempDir()}

	if _, err := s.Start("d1", StartOptions{}); !errors.Is(err, errNoDestination) {
		t.Fatalf("err = %v, want %v", err, errNoDestination)
	}
}

func TestSilentInstallFindsDirectoryInstallerChose(t *testing.T) {
	s, downloads, registrar := newTestService(t)
	root := t.TempDir()
	innoSource(t, root)
	downloads.add("d1", "Game", root)

	games := t.TempDir()
	dest := filepath.Join(games, "Game")
	elsewhere := filepath.Join(games, "GOG Games", "Game")
	s.roots = []string{games}
	s.runner = &fakeRunner{act: func(runSpec) {
		mkFile(t, filepath.Join(elsewhere, "Game.exe"), 8192)
	}}

	item, err := s.Start("d1", StartOptions{Destination: dest})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	done := s.waitStatus(t, item.ID, StatusCompleted)
	if done.Destination != elsewhere {
		t.Fatalf("destination = %s, want %s", done.Destination, elsewhere)
	}
	if len(registrar.games) != 1 || registrar.games[0].InstallDir != elsewhere {
		t.Fatalf("registered = %+v", registrar.games)
	}
	if exists(dest) {
		t.Fatalf("empty %s left behind", dest)
	}
}

func TestSilentInstallWithoutOutputFails(t *testing.T) {
	s, downloads, _ := newTestService(t)
	root := t.TempDir()
	innoSource(t, root)
	downloads.add("d1", "Game", root)

	games := t.TempDir()
	dest := filepath.Join(games, "Game")
	s.roots = []string{games}
	s.runner = &fakeRunner{}

	item, err := s.Start("d1", StartOptions{Destination: dest})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	failed := s.waitStatus(t, item.ID, StatusFailed)
	if failed.Error != errInstallerNoOutput.Error() {
		t.Fatalf("error = %q", failed.Error)
	}
}

func TestSilentInstallCancelledByInstallerCleansDirectory(t *testing.T) {
	s, downloads, _ := newTestService(t)
	root := t.TempDir()
	innoSource(t, root)
	downloads.add("d1", "Game", root)

	games := t.TempDir()
	dest := filepath.Join(games, "Game")
	s.roots = []string{games}
	s.runner = &fakeRunner{code: 2, act: func(runSpec) {
		mkFile(t, filepath.Join(dest, "half.bin"), 1024)
	}}

	item, err := s.Start("d1", StartOptions{Destination: dest})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	failed := s.waitStatus(t, item.ID, StatusFailed)
	if !strings.Contains(failed.Error, errInstallerCancelled.Error()) {
		t.Fatalf("error = %q", failed.Error)
	}
	if exists(dest) {
		t.Fatalf("failed install left %s behind", dest)
	}
}

func TestSilentInstallRejectsRelativeDestination(t *testing.T) {
	s, downloads, _ := newTestService(t)
	root := t.TempDir()
	innoSource(t, root)
	downloads.add("d1", "Game", root)
	s.roots = []string{t.TempDir()}

	if _, err := s.Start("d1", StartOptions{Destination: filepath.Join("relative", "Game")}); !errors.Is(err, errRelativeDestination) {
		t.Fatalf("err = %v, want %v", err, errRelativeDestination)
	}
}

func TestRetryUpgradesLegacyInstallerRecord(t *testing.T) {
	s, downloads, registrar := newTestService(t)
	root := t.TempDir()
	dir := innoSource(t, root)
	downloads.add("d1", "Game", root)

	games := s.config().GamesPath
	s.roots = []string{games}
	s.runner = &fakeRunner{act: func(spec runSpec) {
		for _, arg := range spec.Args {
			if target, ok := strings.CutPrefix(arg, "/DIR="); ok {
				mkFile(t, filepath.Join(target, "Game.exe"), 8192)
			}
		}
	}}

	s.mu.Lock()
	s.items = append(s.items, &Installation{
		ID:            "old",
		DownloadID:    "d1",
		Name:          "Game",
		Type:          TypeExeInstaller,
		Status:        StatusFailed,
		InstallerPath: filepath.Join(dir, "setup.exe"),
		WorkingDir:    dir,
		Error:         errInstallerFail.Error(),
	})
	s.mu.Unlock()

	if err := s.Retry("old"); err != nil {
		t.Fatalf("retry: %v", err)
	}
	done := s.waitStatus(t, "old", StatusCompleted)
	if done.Engine != EngineInno || !done.Silent {
		t.Fatalf("engine = %s, silent = %v", done.Engine, done.Silent)
	}
	if filepath.Dir(done.Destination) != games {
		t.Fatalf("destination = %s, want under %s", done.Destination, games)
	}
	if len(registrar.games) != 1 || registrar.games[0].InstallDir != done.Destination {
		t.Fatalf("registered = %+v", registrar.games)
	}
}

func argValue(spec runSpec, prefix string) string {
	for _, arg := range spec.Args {
		if value, ok := strings.CutPrefix(arg, prefix); ok {
			return value
		}
	}
	return ""
}

func writeUTF16(t *testing.T, path, text string) {
	t.Helper()
	data := make([]byte, 0, len(text)*2)
	for _, b := range []byte(text) {
		data = append(data, b, 0)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestSilentInstallTrustsInstallerLogOnCrashExit(t *testing.T) {
	s, downloads, registrar := newTestService(t)
	root := t.TempDir()
	innoSource(t, root)
	downloads.add("d1", "Game", root)

	games := t.TempDir()
	dest := filepath.Join(games, "Game")
	s.roots = []string{games}
	s.runner = &fakeRunner{code: 3221226525, act: func(spec runSpec) {
		mkFile(t, filepath.Join(dest, "Game.exe"), 8192)
		writeUTF16(t, argValue(spec, "/LOG="), "Registering files\r\n"+innoSuccessMarker+"\r\n")
	}}

	item, err := s.Start("d1", StartOptions{Destination: dest})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	done := s.waitStatus(t, item.ID, StatusCompleted)
	if done.Destination != dest {
		t.Fatalf("destination = %s", done.Destination)
	}
	if len(registrar.games) != 1 {
		t.Fatalf("registered = %+v", registrar.games)
	}
}

func TestSilentInstallFailsWhenLogHasNoSuccess(t *testing.T) {
	s, downloads, _ := newTestService(t)
	root := t.TempDir()
	innoSource(t, root)
	downloads.add("d1", "Game", root)

	games := t.TempDir()
	dest := filepath.Join(games, "Game")
	s.roots = []string{games}
	s.runner = &fakeRunner{code: 3221226525, act: func(spec runSpec) {
		mkFile(t, filepath.Join(dest, "Game.exe"), 8192)
		writeUTF16(t, argValue(spec, "/LOG="), "Rolling back changes\r\n")
	}}

	item, err := s.Start("d1", StartOptions{Destination: dest})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	failed := s.waitStatus(t, item.ID, StatusFailed)
	if !strings.Contains(failed.Error, errInstallerFail.Error()) {
		t.Fatalf("error = %q", failed.Error)
	}
	if exists(dest) {
		t.Fatalf("failed install left %s behind", dest)
	}
}

func TestInteractiveInstallerKeepsForegroundRun(t *testing.T) {
	s, downloads, _ := newTestService(t)
	root := t.TempDir()
	dir := filepath.Join(root, "Game")
	mkFile(t, filepath.Join(dir, "setup.exe"), 4096)
	downloads.add("d1", "Game", root)

	games := t.TempDir()
	s.roots = []string{games}
	s.runner = &fakeRunner{act: func(runSpec) {
		mkFile(t, filepath.Join(games, "Game", "Game.exe"), 8192)
	}}

	item, err := s.Start("d1", StartOptions{})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	s.waitStatus(t, item.ID, StatusWaitingForUser)

	fake, ok := s.runner.(*fakeRunner)
	if !ok {
		t.Fatalf("runner = %T", s.runner)
	}
	calls := fake.calls()
	if len(calls) != 1 {
		t.Fatalf("runner calls = %d", len(calls))
	}
	if calls[0].Background || calls[0].Hidden || len(calls[0].Args) != 0 {
		t.Fatalf("interactive spec = %+v", calls[0])
	}
}
