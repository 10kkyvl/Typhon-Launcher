package install

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
)

const (
	cupheadBase  = "setup_cuphead_1.3.9_(85531)"
	cupheadAddon = "setup_cuphead_-_the_delicious_last_course_1.3.9_(85531)"
)

type chainRunner struct {
	mu    sync.Mutex
	codes []int
	specs []runSpec
	act   func(spec runSpec)
}

func (r *chainRunner) run(_ context.Context, spec runSpec) (int, error) {
	r.mu.Lock()
	n := len(r.specs)
	r.specs = append(r.specs, spec)
	r.mu.Unlock()
	if r.act != nil {
		r.act(spec)
	}
	if n < len(r.codes) {
		return r.codes[n], nil
	}
	return 0, nil
}

func (r *chainRunner) calls() []runSpec {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]runSpec(nil), r.specs...)
}

func gogInstaller(t *testing.T, dir, stem string, data int) {
	t.Helper()
	mkInstaller(t, filepath.Join(dir, stem+".exe"), "Inno Setup Setup Data (5.6.2) (u)")
	mkFile(t, filepath.Join(dir, stem+"-1.bin"), data)
}

func cupheadSource(t *testing.T, root string) string {
	t.Helper()
	dir := filepath.Join(root, "Cuphead")
	gogInstaller(t, dir, cupheadAddon, 4096)
	gogInstaller(t, dir, cupheadBase, 16384)
	return dir
}

func TestInspectPutsBaseGameBeforeAddon(t *testing.T) {
	root := t.TempDir()
	dir := cupheadSource(t, root)

	plan, err := Inspect(context.Background(), root)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if plan.InstallerPath != filepath.Join(dir, cupheadBase+".exe") {
		t.Fatalf("installer = %s, want base game", plan.InstallerPath)
	}
	if len(plan.ExtraInstallers) != 1 || plan.ExtraInstallers[0] != filepath.Join(dir, cupheadAddon+".exe") {
		t.Fatalf("extras = %v, want addon", plan.ExtraInstallers)
	}
}

func TestInspectKeepsUnrelatedInstallerOutOfChain(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "Game")
	gogInstaller(t, dir, "setup", 16384)
	mkInstaller(t, filepath.Join(dir, "install_dependencies.exe"), "Inno Setup Setup Data (5.6.2) (u)")

	plan, err := Inspect(context.Background(), root)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if plan.InstallerPath != filepath.Join(dir, "setup.exe") {
		t.Fatalf("installer = %s", plan.InstallerPath)
	}
	if len(plan.ExtraInstallers) != 0 {
		t.Fatalf("extras = %v, want none", plan.ExtraInstallers)
	}
}

func TestInspectDropsChainWhenPlanIsOverriddenByHand(t *testing.T) {
	root := t.TempDir()
	dir := cupheadSource(t, root)

	plan, err := Inspect(context.Background(), root)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if err := fillInstallerPlan(&plan, TypeExeInstaller, filepath.Join(dir, cupheadAddon+".exe")); err != nil {
		t.Fatalf("fillInstallerPlan: %v", err)
	}
	if len(plan.ExtraInstallers) != 0 {
		t.Fatalf("extras = %v, want none", plan.ExtraInstallers)
	}
}

func TestSilentInstallRunsAddonAfterBaseGame(t *testing.T) {
	s, downloads, _ := newTestService(t)
	root := t.TempDir()
	dir := cupheadSource(t, root)
	downloads.add("d1", "Cuphead", root)

	games := t.TempDir()
	dest := filepath.Join(games, "Cuphead")
	s.roots = []string{games}
	runner := &chainRunner{act: func(spec runSpec) {
		mkFile(t, filepath.Join(dest, "Cuphead.exe"), 8192)
	}}
	s.runner = runner

	item, err := s.Start("d1", StartOptions{Destination: dest})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	s.waitStatus(t, item.ID, StatusCompleted)

	calls := runner.calls()
	if len(calls) != 2 {
		t.Fatalf("runner calls = %d, want 2", len(calls))
	}
	if calls[0].Path != filepath.Join(dir, cupheadBase+".exe") {
		t.Fatalf("first call = %s, want base game", calls[0].Path)
	}
	if calls[1].Path != filepath.Join(dir, cupheadAddon+".exe") {
		t.Fatalf("second call = %s, want addon", calls[1].Path)
	}
	if got := argValue(calls[1], "/DIR="); got != dest {
		t.Fatalf("addon directory = %q, want %q", got, dest)
	}
}

func TestSilentInstallFailsWhenAddonFails(t *testing.T) {
	s, downloads, registrar := newTestService(t)
	root := t.TempDir()
	cupheadSource(t, root)
	downloads.add("d1", "Cuphead", root)

	games := t.TempDir()
	dest := filepath.Join(games, "Cuphead")
	s.roots = []string{games}
	runner := &chainRunner{codes: []int{0, 1}, act: func(spec runSpec) {
		mkFile(t, filepath.Join(dest, "Cuphead.exe"), 8192)
	}}
	s.runner = runner

	item, err := s.Start("d1", StartOptions{Destination: dest})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	done := s.waitStatus(t, item.ID, StatusFailed)
	if done.Error == "" {
		t.Fatal("failed install must carry an error")
	}
	if len(runner.calls()) != 2 {
		t.Fatalf("runner calls = %d, want 2", len(runner.calls()))
	}
	if exists(dest) {
		t.Fatalf("half-installed %s left behind", dest)
	}
	if len(registrar.games) != 0 {
		t.Fatalf("registered = %+v", registrar.games)
	}
}

func TestRetryRepicksAutomaticInstaller(t *testing.T) {
	s, downloads, _ := newTestService(t)
	root := t.TempDir()
	dir := cupheadSource(t, root)
	downloads.add("d1", "Cuphead", root)

	games := s.config().GamesPath
	s.roots = []string{games}
	runner := &chainRunner{act: func(spec runSpec) {
		if target := argValue(spec, "/DIR="); target != "" {
			mkFile(t, filepath.Join(target, "Cuphead.exe"), 8192)
		}
	}}
	s.runner = runner

	s.mu.Lock()
	s.items = append(s.items, &Installation{
		ID:            "old",
		DownloadID:    "d1",
		Name:          "Cuphead",
		Type:          TypeExeInstaller,
		Status:        StatusFailed,
		InstallerPath: filepath.Join(dir, cupheadAddon+".exe"),
		WorkingDir:    dir,
		Error:         errInstallerFail.Error(),
	})
	s.mu.Unlock()

	if err := s.Retry("old"); err != nil {
		t.Fatalf("retry: %v", err)
	}
	done := s.waitStatus(t, "old", StatusCompleted)
	if done.InstallerPath != filepath.Join(dir, cupheadBase+".exe") {
		t.Fatalf("installer = %s, want base game", done.InstallerPath)
	}
	calls := runner.calls()
	if len(calls) != 2 {
		t.Fatalf("runner calls = %d, want 2", len(calls))
	}
	if calls[0].Path != filepath.Join(dir, cupheadBase+".exe") || calls[1].Path != filepath.Join(dir, cupheadAddon+".exe") {
		t.Fatalf("calls = %s, %s", calls[0].Path, calls[1].Path)
	}
}

func TestRetryKeepsInstallerChosenByHand(t *testing.T) {
	s, downloads, _ := newTestService(t)
	root := t.TempDir()
	dir := cupheadSource(t, root)
	downloads.add("d1", "Cuphead", root)

	games := s.config().GamesPath
	s.roots = []string{games}
	runner := &chainRunner{act: func(spec runSpec) {
		if target := argValue(spec, "/DIR="); target != "" {
			mkFile(t, filepath.Join(target, "Cuphead.exe"), 8192)
		}
	}}
	s.runner = runner

	s.mu.Lock()
	s.items = append(s.items, &Installation{
		ID:              "old",
		DownloadID:      "d1",
		Name:            "Cuphead",
		Type:            TypeExeInstaller,
		Status:          StatusFailed,
		InstallerPath:   filepath.Join(dir, cupheadAddon+".exe"),
		ManualInstaller: true,
		WorkingDir:      dir,
		Error:           errInstallerFail.Error(),
	})
	s.mu.Unlock()

	if err := s.Retry("old"); err != nil {
		t.Fatalf("retry: %v", err)
	}
	done := s.waitStatus(t, "old", StatusCompleted)
	if done.InstallerPath != filepath.Join(dir, cupheadAddon+".exe") {
		t.Fatalf("installer = %s, want the file chosen by hand", done.InstallerPath)
	}
	if calls := runner.calls(); len(calls) != 1 {
		t.Fatalf("runner calls = %d, want 1", len(calls))
	}
}

func TestInstallerSlug(t *testing.T) {
	cases := []struct{ stem, want string }{
		{cupheadBase, "setup_cuphead"},
		{cupheadAddon, "setup_cuphead_-_the_delicious_last_course"},
		{"setup_the_witcher_3_wild_hunt_goty_4.04_(a)_(17987)", "setup_the_witcher_3_wild_hunt_goty"},
		{"setup", "setup"},
		{"install_dependencies", "install_dependencies"},
	}
	for _, tc := range cases {
		if got := installerSlug(tc.stem); got != tc.want {
			t.Fatalf("installerSlug(%q) = %q, want %q", tc.stem, got, tc.want)
		}
	}
}
