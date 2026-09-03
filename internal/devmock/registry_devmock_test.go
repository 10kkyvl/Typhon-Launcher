//go:build devmock && !windows

package devmock

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func fixedClock(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

func staticPathFn(path string) func() (string, error) {
	return func() (string, error) { return path, nil }
}

func TestRegistryStartListWaitAfterKill(t *testing.T) {
	path := filepath.Join(t.TempDir(), "devmock-processes.json")
	reg := newRegistry(staticPathFn(path), fixedClock(time.Now()))

	p, err := reg.start("/games/demo/demo.exe", []string{"-windowed"}, "/games/demo", time.Hour)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if p.PID() < startPID {
		t.Fatalf("pid = %d, want >= %d", p.PID(), startPID)
	}
	if p.Path() != "/games/demo/demo.exe" {
		t.Fatalf("path = %q", p.Path())
	}

	entries, err := reg.list()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 1 || entries[0].PID != p.PID() {
		t.Fatalf("list = %+v, want one entry with pid %d", entries, p.PID())
	}

	if err := p.Kill(); err != nil {
		t.Fatalf("kill: %v", err)
	}
	if err := p.Wait(); err != nil {
		t.Fatalf("wait after kill: %v", err)
	}

	entries, err = reg.list()
	if err != nil {
		t.Fatalf("list after kill: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("list after kill = %+v, want empty", entries)
	}
}

func TestRegistryKillTwice(t *testing.T) {
	path := filepath.Join(t.TempDir(), "devmock-processes.json")
	reg := newRegistry(staticPathFn(path), fixedClock(time.Now()))

	p, err := reg.start("/games/demo/demo.exe", nil, "/games/demo", time.Hour)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := p.Kill(); err != nil {
		t.Fatalf("first kill: %v", err)
	}
	if err := p.Kill(); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("second kill = %v, want ErrNotRunning", err)
	}
}

func TestRegistryExpiryPrunesAndUnblocksWait(t *testing.T) {
	path := filepath.Join(t.TempDir(), "devmock-processes.json")
	reg := newRegistry(staticPathFn(path), time.Now)

	p, err := reg.start("/games/demo/demo.exe", nil, "/games/demo", 20*time.Millisecond)
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	if err := p.Wait(); err != nil {
		t.Fatalf("wait for expiry: %v", err)
	}

	entries, err := reg.list()
	if err != nil {
		t.Fatalf("list after expiry: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("list after expiry = %+v, want empty", entries)
	}

	if err := p.Kill(); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("kill after expiry = %v, want ErrNotRunning", err)
	}
}

func TestRegistryPersistsAcrossInstances(t *testing.T) {
	path := filepath.Join(t.TempDir(), "devmock-processes.json")
	now := fixedClock(time.Now())

	regA := newRegistry(staticPathFn(path), now)
	p, err := regA.start("/games/demo/demo.exe", []string{"-a"}, "/games/demo", time.Hour)
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	regB := newRegistry(staticPathFn(path), now)
	entries, err := regB.list()
	if err != nil {
		t.Fatalf("list on fresh registry: %v", err)
	}
	if len(entries) != 1 || entries[0].PID != p.PID() || entries[0].Path != p.Path() {
		t.Fatalf("list on fresh registry = %+v, want one entry with pid %d", entries, p.PID())
	}
}

func TestRegistryNextPIDPersistsAcrossInstances(t *testing.T) {
	path := filepath.Join(t.TempDir(), "devmock-processes.json")
	now := fixedClock(time.Now())

	regA := newRegistry(staticPathFn(path), now)
	first, err := regA.start("/games/a/a.exe", nil, "/games/a", time.Hour)
	if err != nil {
		t.Fatalf("start first: %v", err)
	}

	regB := newRegistry(staticPathFn(path), now)
	second, err := regB.start("/games/b/b.exe", nil, "/games/b", time.Hour)
	if err != nil {
		t.Fatalf("start second: %v", err)
	}
	if second.PID() <= first.PID() {
		t.Fatalf("second pid %d, want > first pid %d", second.PID(), first.PID())
	}
}

func TestRegistryMergesAcrossInstances(t *testing.T) {
	path := filepath.Join(t.TempDir(), "devmock-processes.json")
	now := fixedClock(time.Now())

	regA := newRegistry(staticPathFn(path), now)
	regB := newRegistry(staticPathFn(path), now)

	p1, err := regA.start("/games/a/a.exe", nil, "/games/a", time.Hour)
	if err != nil {
		t.Fatalf("start p1: %v", err)
	}
	p2, err := regB.start("/games/b/b.exe", nil, "/games/b", time.Hour)
	if err != nil {
		t.Fatalf("start p2: %v", err)
	}

	wantPIDs := map[uint32]bool{p1.PID(): true, p2.PID(): true}

	fromA, err := regA.list()
	if err != nil {
		t.Fatalf("list on A: %v", err)
	}
	if !samePIDs(fromA, wantPIDs) {
		t.Fatalf("list on A = %+v, want pids %v", fromA, wantPIDs)
	}

	fromB, err := regB.list()
	if err != nil {
		t.Fatalf("list on B: %v", err)
	}
	if !samePIDs(fromB, wantPIDs) {
		t.Fatalf("list on B = %+v, want pids %v", fromB, wantPIDs)
	}

	if err := p1.Kill(); err != nil {
		t.Fatalf("kill p1: %v", err)
	}

	fromB, err = regB.list()
	if err != nil {
		t.Fatalf("list on B after kill: %v", err)
	}
	if !samePIDs(fromB, map[uint32]bool{p2.PID(): true}) {
		t.Fatalf("list on B after kill = %+v, want only pid %d", fromB, p2.PID())
	}
}

func samePIDs(entries []Entry, want map[uint32]bool) bool {
	if len(entries) != len(want) {
		return false
	}
	for _, e := range entries {
		if !want[e.PID] {
			return false
		}
	}
	return true
}

func TestRegistryStartRejectsEmptyPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "devmock-processes.json")
	reg := newRegistry(staticPathFn(path), fixedClock(time.Now()))

	if _, err := reg.start("", nil, "/games/demo", time.Hour); err == nil {
		t.Fatal("start with empty path: expected error")
	}
}

func TestRegistryListRejectsInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "devmock-processes.json")
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	reg := newRegistry(staticPathFn(path), fixedClock(time.Now()))

	if _, err := reg.list(); err == nil {
		t.Fatal("list over invalid JSON: expected error")
	}
}

func TestRegistryStartRejectsInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "devmock-processes.json")
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	reg := newRegistry(staticPathFn(path), fixedClock(time.Now()))

	if _, err := reg.start("/games/demo/demo.exe", nil, "/games/demo", time.Hour); err == nil {
		t.Fatal("start over invalid JSON: expected error")
	}
}

func TestRegistryUnreadableDir(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permissions")
	}
	dir := t.TempDir()
	locked := filepath.Join(dir, "locked")
	if err := os.Mkdir(locked, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(locked, "devmock-processes.json")
	reg := newRegistry(staticPathFn(path), fixedClock(time.Now()))

	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		//nolint:gosec // G302: каталогу возвращается исходный режим (инвариант 8), иначе t.TempDir() не сможет его удалить
		if err := os.Chmod(locked, 0o755); err != nil {
			t.Fatal(err)
		}
	})

	if _, err := reg.start("/games/demo/demo.exe", nil, "/games/demo", time.Hour); err == nil {
		t.Fatal("start under unreadable dir: expected error")
	}
}

func TestRegistryPathResolutionError(t *testing.T) {
	wantErr := errors.New("path unavailable")
	reg := newRegistry(func() (string, error) { return "", wantErr }, fixedClock(time.Now()))

	if _, err := reg.start("/games/demo/demo.exe", nil, "/games/demo", time.Hour); !errors.Is(err, wantErr) {
		t.Fatalf("start = %v, want %v", err, wantErr)
	}
	if _, err := reg.list(); !errors.Is(err, wantErr) {
		t.Fatalf("list = %v, want %v", err, wantErr)
	}
}

func TestGameLifetime(t *testing.T) {
	cases := []struct {
		name    string
		set     bool
		value   string
		want    time.Duration
		wantErr bool
	}{
		{name: "unset defaults to 60s", set: false, want: defaultGameSeconds * time.Second},
		{name: "positive integer", set: true, value: "5", want: 5 * time.Second},
		{name: "empty is invalid", set: true, value: "", wantErr: true},
		{name: "non-numeric is invalid", set: true, value: "abc", wantErr: true},
		{name: "zero is invalid", set: true, value: "0", wantErr: true},
		{name: "negative is invalid", set: true, value: "-1", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.set {
				t.Setenv(gameSecondsEnv, tc.value)
			} else {
				if err := os.Unsetenv(gameSecondsEnv); err != nil {
					t.Fatal(err)
				}
			}
			got, err := gameLifetime()
			if tc.wantErr {
				if err == nil {
					t.Fatalf("gameLifetime() = %v, nil, want error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("gameLifetime(): %v", err)
			}
			if got != tc.want {
				t.Fatalf("gameLifetime() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestStateDirFromEnv(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sub", "state")
	t.Setenv(StateDirEnv, dir)

	got, err := StateDir()
	if err != nil {
		t.Fatalf("StateDir(): %v", err)
	}
	if got != dir {
		t.Fatalf("StateDir() = %q, want %q", got, dir)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat created dir: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("%s is not a directory", dir)
	}
}

func TestStateDirRejectsRelativeEnv(t *testing.T) {
	t.Setenv(StateDirEnv, "relative/path")

	if _, err := StateDir(); err == nil {
		t.Fatal("StateDir() with relative env value: expected error")
	}
}
