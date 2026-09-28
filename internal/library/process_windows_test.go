//go:build windows && !devmock

package library

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"golang.org/x/sys/windows"

	"typhon/internal/uierr"
)

const elevatedExe = `C:\Windows\regedit.exe`

// requireElevationPrompt skips unless CreateProcess of elevatedExe fails with
// ERROR_ELEVATION_REQUIRED here, the same way it does for a game with
// requireAdministrator in its manifest.
func requireElevationPrompt(t *testing.T) {
	t.Helper()
	if windows.GetCurrentProcessToken().IsElevated() {
		t.Skip("тест идёт с правами администратора: ERROR_ELEVATION_REQUIRED не воспроизводится")
	}
	if _, err := os.Stat(elevatedExe); err != nil {
		t.Skipf("нет %s: %v", elevatedExe, err)
	}
	probe := exec.Command(elevatedExe)
	err := probe.Start()
	if err == nil {
		if killErr := probe.Process.Kill(); killErr != nil {
			t.Fatalf("kill probe: %v", killErr)
		}
		var exit *exec.ExitError
		if waitErr := probe.Wait(); waitErr != nil && !errors.As(waitErr, &exit) {
			t.Fatalf("wait probe: %v", waitErr)
		}
		t.Skipf("%s запустился без повышения: пользователь не администратор", elevatedExe)
	}
	if !errors.Is(err, windows.ERROR_ELEVATION_REQUIRED) {
		t.Fatalf("probe start error = %v, want ERROR_ELEVATION_REQUIRED", err)
	}
}

// longProcess starts a real child and wraps a handle to it the way
// startElevated wraps the one ShellExecuteEx returns.
func longProcess(t *testing.T) *handleProcess {
	t.Helper()
	cmd := exec.Command(`C:\Windows\System32\cmd.exe`, "/C", "ping -n 30 127.0.0.1 >nul")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		// Тест обычно уже убил процесс через handleProcess, а TerminateProcess
		// на завершённом процессе отвечает ERROR_ACCESS_DENIED.
		if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) && !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
			t.Errorf("kill child: %v", err)
		}
		var exit *exec.ExitError
		if err := cmd.Wait(); err != nil && !errors.As(err, &exit) {
			t.Errorf("wait child: %v", err)
		}
	})
	//nolint:gosec // G115: PID из os/exec укладывается в uint32 на Windows
	handle, err := windows.OpenProcess(
		windows.PROCESS_TERMINATE|windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION,
		false, uint32(cmd.Process.Pid))
	if err != nil {
		t.Fatal(err)
	}
	return &handleProcess{handle: handle, id: cmd.Process.Pid}
}

func TestElevatingStarterFallsBackOnElevationRequired(t *testing.T) {
	requireElevationPrompt(t)
	want := &handleProcess{id: 4242}
	var got launch
	start := elevatingStarter(execStarter, func(req launch) (gameProcess, error) {
		got = req
		return want, nil
	})
	req := launch{executable: elevatedExe, args: []string{"/s", `C:\Program Files\x.reg`}, workDir: `C:\Windows`}

	proc, err := start(t.Context(), req)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if proc != want {
		t.Fatalf("start returned %v, want the elevated process", proc)
	}
	if got.executable != req.executable || got.workDir != req.workDir || len(got.args) != 2 || got.args[1] != req.args[1] {
		t.Fatalf("elevate got %+v, want %+v", got, req)
	}
}

func TestElevatingStarterKeepsOtherErrors(t *testing.T) {
	var calls atomic.Int32
	start := elevatingStarter(execStarter, func(launch) (gameProcess, error) {
		calls.Add(1)
		return nil, errors.New("elevate must not be called")
	})
	missing := filepath.Join(t.TempDir(), "game.exe")

	_, err := start(t.Context(), launch{executable: missing, workDir: filepath.Dir(missing)})
	if err == nil {
		t.Fatal("start of a missing executable succeeded")
	}
	if calls.Load() != 0 {
		t.Fatalf("a plain start failure went to UAC (%d calls): %v", calls.Load(), err)
	}
}

func TestPlayGameElevationDeclinedKeepsItsCode(t *testing.T) {
	requireElevationPrompt(t)
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	game, err := s.AddGame(tempGameExe(t), "Elevated")
	if err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	s.findLocked(game.ID).Executable = elevatedExe
	s.mu.Unlock()
	var calls atomic.Int32
	s.start = elevatingStarter(execStarter, func(launch) (gameProcess, error) {
		calls.Add(1)
		return nil, errElevationDeclined
	})

	for attempt := 1; attempt <= 2; attempt++ {
		err := s.PlayGame(game.ID)
		if code := uierr.Code(err); code != "library.elevation_declined" {
			t.Fatalf("attempt %d: PlayGame error code = %q (%v), want library.elevation_declined", attempt, code, err)
		}
		if int(calls.Load()) != attempt {
			t.Fatalf("attempt %d: elevate called %d times: a declined prompt left the game stuck as starting", attempt, calls.Load())
		}
	}
	if s.IsRunning(game.ID) {
		t.Fatal("declined launch opened a session")
	}
}

func TestPlayGameTracksElevatedProcess(t *testing.T) {
	requireElevationPrompt(t)
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	game, err := s.AddGame(tempGameExe(t), "Elevated")
	if err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	s.findLocked(game.ID).Executable = elevatedExe
	s.mu.Unlock()
	proc := longProcess(t)
	s.start = elevatingStarter(execStarter, func(launch) (gameProcess, error) { return proc, nil })

	if err := s.PlayGame(game.ID); err != nil {
		t.Fatalf("PlayGame: %v", err)
	}
	if !s.IsRunning(game.ID) {
		t.Fatal("elevated game has no session")
	}
	if err := s.StopGame(game.ID); err != nil {
		t.Fatalf("StopGame: %v", err)
	}
	s.sessionWG.Wait()
	if s.IsRunning(game.ID) {
		t.Fatal("session survived the elevated process exit")
	}
}

func TestHandleProcessKillAfterExit(t *testing.T) {
	proc := longProcess(t)
	waited := make(chan error, 1)
	go func() { waited <- proc.wait() }()

	if err := proc.kill(); err != nil {
		t.Fatalf("kill: %v", err)
	}
	err := <-waited
	if code, known := exitCode(err); !known || code != 1 {
		t.Fatalf("wait after kill = %v (code=%d known=%v), want exit code 1", err, code, known)
	}
	if err := proc.kill(); !errors.Is(err, errSessionProcessGone) {
		t.Fatalf("kill after exit = %v, want errSessionProcessGone", err)
	}
}

func TestStartElevatedRejectsBadPath(t *testing.T) {
	_, err := startElevated(launch{executable: "C:" + string(rune(0)) + `\game.exe`})
	if err == nil {
		t.Fatal("startElevated с нулевым байтом в пути вернул успех")
	}
	if errors.Is(err, errElevationDeclined) {
		t.Fatalf("bad path reported as a declined prompt: %v", err)
	}
}

// Пробник обязан быть GUI-приложением: консольный дочерний процесс цепляется
// к консоли теста при любом стартере, и проверка хэндлов ничего не доказала бы.
const launchProbeSource = `package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
)

func main() {
	if len(os.Args) < 3 {
		os.Exit(9)
	}
	mode, out, rest := os.Args[1], os.Args[2], os.Args[3:]
	switch mode {
	case "env":
		write(out, strings.Join(os.Environ(), "\n"))
	case "stdio":
		write(out, stdioReport())
	case "cwd":
		wd, err := os.Getwd()
		if err != nil {
			os.Exit(9)
		}
		write(out, wd)
	case "args":
		write(out, strings.Join(rest, "\x1f"))
	case "exit":
		if len(rest) != 1 {
			os.Exit(9)
		}
		code, err := strconv.Atoi(rest[0])
		if err != nil {
			os.Exit(9)
		}
		os.Exit(code)
	default:
		os.Exit(9)
	}
}

func write(path, content string) {
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		os.Exit(9)
	}
}

func stdioReport() string {
	handles := []struct {
		label string
		id    int
	}{
		{"in", syscall.STD_INPUT_HANDLE},
		{"out", syscall.STD_OUTPUT_HANDLE},
		{"err", syscall.STD_ERROR_HANDLE},
	}
	var b strings.Builder
	for _, h := range handles {
		handle, err := syscall.GetStdHandle(h.id)
		status := "valid"
		if err != nil || handle == 0 || handle == syscall.InvalidHandle {
			status = "empty"
		}
		fmt.Fprintf(&b, "%s=%s\n", h.label, status)
	}
	return b.String()
}
`

func buildLaunchProbe(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(dir, "probe.go")
	if err := os.WriteFile(src, []byte(launchProbeSource), 0o600); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(dir, "probe.exe")
	//nolint:gosec // G204: exe и src — пути внутри t.TempDir(), построенные самим тестом; внешнего ввода, который требует валидации по инварианту 32, здесь нет
	cmd := exec.Command("go", "build", "-ldflags=-H=windowsgui", "-o", exe, src)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build launch probe: %v\n%s", err, out)
	}
	return exe
}

func startProbe(t *testing.T, start gameStarter, probe, workDir string, args ...string) gameProcess {
	t.Helper()
	proc, err := start(t.Context(), launch{executable: probe, args: args, workDir: workDir})
	if err != nil {
		t.Fatalf("start probe: %v", err)
	}
	return proc
}

func readProbeFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read probe output %s: %v", path, err)
	}
	return string(data)
}

func hasEnvVar(env []string, key string) bool {
	for _, line := range env {
		if strings.HasPrefix(line, key+"=") {
			return true
		}
	}
	return false
}

func TestCreateProcessStarterUsesInteractiveUserEnvironment(t *testing.T) {
	probe := buildLaunchProbe(t)
	t.Setenv("TYPHON_LAUNCH_LEAK_PROBE", "1")
	t.Setenv("WEBVIEW2_USER_DATA_FOLDER", "leak")

	outNew := filepath.Join(t.TempDir(), "env_new.txt")
	proc := startProbe(t, createProcessStarter, probe, filepath.Dir(probe), "env", outNew)
	if err := proc.wait(); err != nil {
		t.Fatalf("probe: %v", err)
	}
	newEnv := strings.Split(readProbeFile(t, outNew), "\n")
	for _, key := range []string{"TYPHON_LAUNCH_LEAK_PROBE", "WEBVIEW2_USER_DATA_FOLDER"} {
		if hasEnvVar(newEnv, key) {
			t.Fatalf("launcher-only variable %s reached the game through createProcessStarter", key)
		}
	}
	for _, key := range []string{"USERPROFILE", "SystemRoot"} {
		if !hasEnvVar(newEnv, key) {
			t.Fatalf("expected user environment variable %s missing from the game", key)
		}
	}

	outOld := filepath.Join(t.TempDir(), "env_old.txt")
	oldProc := startProbe(t, execStarter, probe, filepath.Dir(probe), "env", outOld)
	if err := oldProc.wait(); err != nil {
		t.Fatalf("probe (old starter): %v", err)
	}
	oldEnv := strings.Split(readProbeFile(t, outOld), "\n")
	if !hasEnvVar(oldEnv, "TYPHON_LAUNCH_LEAK_PROBE") {
		t.Fatal("sanity check failed: execStarter no longer inherits the launcher's own environment")
	}
}

func TestCreateProcessStarterDoesNotRedirectStdioToNUL(t *testing.T) {
	probe := buildLaunchProbe(t)

	outNew := filepath.Join(t.TempDir(), "stdio_new.txt")
	proc := startProbe(t, createProcessStarter, probe, filepath.Dir(probe), "stdio", outNew)
	if err := proc.wait(); err != nil {
		t.Fatalf("probe: %v", err)
	}
	newReport := strings.TrimSpace(readProbeFile(t, outNew))
	for _, line := range strings.Split(newReport, "\n") {
		if !strings.HasSuffix(line, "=empty") {
			t.Fatalf("createProcessStarter left a real standard handle open: %s\nfull report:\n%s", line, newReport)
		}
	}

	outOld := filepath.Join(t.TempDir(), "stdio_old.txt")
	oldProc := startProbe(t, execStarter, probe, filepath.Dir(probe), "stdio", outOld)
	if err := oldProc.wait(); err != nil {
		t.Fatalf("probe (old starter): %v", err)
	}
	oldReport := strings.TrimSpace(readProbeFile(t, outOld))
	for _, line := range strings.Split(oldReport, "\n") {
		if !strings.HasSuffix(line, "=valid") {
			t.Fatalf("sanity check failed: execStarter no longer redirects standard handles: %s", line)
		}
	}
}

func TestCreateProcessStarterUsesRequestedWorkDir(t *testing.T) {
	probe := buildLaunchProbe(t)
	workDir := t.TempDir()
	out := filepath.Join(t.TempDir(), "cwd.txt")

	proc := startProbe(t, createProcessStarter, probe, workDir, "cwd", out)
	if err := proc.wait(); err != nil {
		t.Fatalf("probe: %v", err)
	}
	got := strings.TrimSpace(readProbeFile(t, out))
	if !strings.EqualFold(filepath.Clean(got), filepath.Clean(workDir)) {
		t.Fatalf("game working directory = %q, want %q", got, workDir)
	}
}

func TestCreateProcessStarterPreservesArgsWithSpacesAndQuotes(t *testing.T) {
	probe := buildLaunchProbe(t)
	out := filepath.Join(t.TempDir(), "args.txt")
	args := []string{"value with spaces", `quo"ted value`, `trailing\backslash\`, ""}

	proc := startProbe(t, createProcessStarter, probe, filepath.Dir(probe), append([]string{"args", out}, args...)...)
	if err := proc.wait(); err != nil {
		t.Fatalf("probe: %v", err)
	}
	got := strings.Split(readProbeFile(t, out), "\x1f")
	if len(got) != len(args) {
		t.Fatalf("args = %q, want %q", got, args)
	}
	for i := range args {
		if got[i] != args[i] {
			t.Fatalf("arg %d = %q, want %q (full: got=%q want=%q)", i, got[i], args[i], got, args)
		}
	}
}

func TestCreateProcessStarterExitCode(t *testing.T) {
	probe := buildLaunchProbe(t)
	tests := []struct {
		name string
		code int
	}{
		{"nonzero", 3},
		{"zero", 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "unused.txt")
			proc := startProbe(t, createProcessStarter, probe, filepath.Dir(probe), "exit", out, strconv.Itoa(tc.code))
			err := proc.wait()
			code, known := exitCode(err)
			if !known || code != tc.code {
				t.Fatalf("exitCode(wait()) = (%d, %v), want (%d, true); err=%v", code, known, tc.code, err)
			}
		})
	}
}

func TestCreateProcessStarterMissingExecutable(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.exe")
	proc, err := createProcessStarter(t.Context(), launch{executable: missing, workDir: filepath.Dir(missing)})
	if err == nil {
		t.Fatal("start of a missing executable succeeded")
	}
	if proc != nil {
		t.Fatalf("start returned a process alongside an error: %v", proc)
	}
}

func TestCreateProcessStarterKillAfterExit(t *testing.T) {
	probe := buildLaunchProbe(t)
	out := filepath.Join(t.TempDir(), "exit.txt")
	proc := startProbe(t, createProcessStarter, probe, filepath.Dir(probe), "exit", out, "0")
	if err := proc.wait(); err != nil {
		t.Fatalf("probe: %v", err)
	}
	hp, ok := proc.(*handleProcess)
	if !ok {
		t.Fatalf("createProcessStarter returned %T, want *handleProcess", proc)
	}
	if err := hp.kill(); !errors.Is(err, errSessionProcessGone) {
		t.Fatalf("kill after exit = %v, want errSessionProcessGone", err)
	}
}
