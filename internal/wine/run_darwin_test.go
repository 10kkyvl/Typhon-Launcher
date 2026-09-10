package wine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCxstartArgs(t *testing.T) {
	b := Bottle{Name: "Typhon-Demo-abcd1234", Drive: "t", Games: "/Users/x/Games"}
	c := Cmd{
		Path:         `T:\Demo\setup.exe`,
		Args:         []string{"/VERYSILENT", `/DIR=T:\Demo`},
		WorkDir:      `T:\Demo`,
		Log:          "/tmp/cx.log",
		WaitChildren: true,
	}

	got := strings.Join(cxstartArgs(b, c, true), " ")
	for _, want := range []string{
		"--bottle Typhon-Demo-abcd1234",
		"--no-gui",
		"--wait-children",
		`--workdir T:\Demo`,
		"--cx-log /tmp/cx.log",
		`-- T:\Demo\setup.exe /VERYSILENT /DIR=T:\Demo`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("args %q missing %q", got, want)
		}
	}
}

func TestCxstartArgsDetached(t *testing.T) {
	b := Bottle{Name: "Typhon-Demo-abcd1234", Drive: "t"}
	c := Cmd{Path: `T:\Demo\game.exe`}

	got := strings.Join(cxstartArgs(b, c, false), " ")
	if !strings.Contains(got, "--no-wait") {
		t.Fatalf("args %q missing --no-wait", got)
	}
	if strings.Contains(got, "--wait-children") {
		t.Fatalf("args %q must not wait", got)
	}
}

func TestCxstartArgsOptional(t *testing.T) {
	b := Bottle{Name: "B", Drive: "t"}
	got := strings.Join(cxstartArgs(b, Cmd{Path: `T:\a.exe`}, true), " ")
	for _, absent := range []string{"--workdir", "--cx-log", "--dll", "--winver"} {
		if strings.Contains(got, absent) {
			t.Fatalf("args %q must not contain %q when unset", got, absent)
		}
	}
}

func TestRunReturnsExitCode(t *testing.T) {
	m, bottles, _ := newTestManager(t)
	// Подменяем cxstart скриптом, который возвращает заданный код.
	if err := os.WriteFile(m.rt.CxStart, []byte("#!/bin/sh\nexit 7\n"), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	b := Bottle{Name: "B", Path: filepath.Join(bottles, "B"), Drive: "t", Games: t.TempDir()}

	code, err := m.Run(context.Background(), b, Cmd{Path: `T:\a.exe`})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if code != 7 {
		t.Fatalf("code = %d, want 7", code)
	}
}

func TestRunCancelled(t *testing.T) {
	m, bottles, _ := newTestManager(t)
	if err := os.WriteFile(m.rt.CxStart, []byte("#!/bin/sh\nsleep 30\n"), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	b := Bottle{Name: "B", Path: filepath.Join(bottles, "B"), Drive: "t", Games: t.TempDir()}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := m.Run(ctx, b, Cmd{Path: `T:\a.exe`}); err == nil {
		t.Fatal("Run with a cancelled context: want error")
	}
}

// TestRunCancelledKillsBottle закрывает КРИТ-находку: cxstart подменяет себя
// winewrapper'ом, и убийство прямого потомка (единственное, что делает голый
// exec.CommandContext) не гасит установщик внутри бутыля. Run обязан сам
// свалить бутыль через Kill (wineserver -k), а не оставлять живого писателя.
func TestRunCancelledKillsBottle(t *testing.T) {
	m, bottles, log := newTestManager(t)
	if err := os.WriteFile(m.rt.CxStart, []byte("#!/bin/sh\nsleep 30\n"), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	wineServerScript := "#!/bin/sh\necho \"wineserver $@ prefix=$WINEPREFIX\" >> " + log + "\nexit 0\n"
	if err := os.WriteFile(m.rt.WineServer, []byte(wineServerScript), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	b := Bottle{Name: "B", Path: filepath.Join(bottles, "B"), Drive: "t", Games: t.TempDir()}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := m.Run(ctx, b, Cmd{Path: `T:\a.exe`}); err == nil {
		t.Fatal("Run with a cancelled context: want error")
	}

	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatalf("ReadFile log: %v", err)
	}
	if !contains(string(data), "wineserver -k") {
		t.Fatalf("cancelled Run did not kill the bottle, log = %q", string(data))
	}
	if !contains(string(data), "prefix="+b.Path) {
		t.Fatalf("log %q missing WINEPREFIX=%s", string(data), b.Path)
	}
}

// TestRunCancelledReportsUnstoppedTree закрывает вторую половину той же
// находки: если Kill не смог подтвердить остановку бутыля, Run обязан
// вернуть ошибку, которую вызывающий отличит от обычной отмены — иначе
// discardSilent не узнает, что писатель мог остаться жив.
func TestRunCancelledReportsUnstoppedTree(t *testing.T) {
	m, bottles, _ := newTestManager(t)
	if err := os.WriteFile(m.rt.CxStart, []byte("#!/bin/sh\nsleep 30\n"), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.WriteFile(m.rt.WineServer, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	b := Bottle{Name: "B", Path: filepath.Join(bottles, "B"), Drive: "t", Games: t.TempDir()}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := m.Run(ctx, b, Cmd{Path: `T:\a.exe`})
	if err == nil {
		t.Fatal("Run with a cancelled context and a failing Kill: want error")
	}
	if !errors.Is(err, ErrTreeNotStopped) {
		t.Fatalf("err = %v, want it to wrap ErrTreeNotStopped", err)
	}
}

// StartDetached намеренно не ждёт: cxstart подменяет себя winewrapper'ом,
// который живёт всё время игры. Значит и код возврата ему не виден — ошибкой
// остаётся только невозможность запустить сам процесс.
func TestStartDetachedDoesNotWaitForExitCode(t *testing.T) {
	m, bottles, _ := newTestManager(t)
	if err := os.WriteFile(m.rt.CxStart, []byte("#!/bin/sh\nsleep 30\n"), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	b := Bottle{Name: "B", Path: filepath.Join(bottles, "B"), Drive: "t", Games: t.TempDir()}

	done := make(chan error, 1)
	go func() { done <- m.StartDetached(context.Background(), b, Cmd{Path: `T:\a.exe`}) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("StartDetached: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("StartDetached ждёт завершения запущенного процесса")
	}
}

func TestStartDetachedReportsUnstartable(t *testing.T) {
	m, bottles, _ := newTestManager(t)
	m.rt.CxStart = filepath.Join(bottles, "no-such-cxstart")
	b := Bottle{Name: "B", Path: filepath.Join(bottles, "B"), Drive: "t", Games: t.TempDir()}

	if err := m.StartDetached(context.Background(), b, Cmd{Path: `T:\a.exe`}); err == nil {
		t.Fatal("StartDetached without cxstart: want error")
	}
}

func TestRunCancelledStopsWrappedInstaller(t *testing.T) {
	m, bottles, _ := newTestManager(t)
	home := t.TempDir()
	fakeSharedBottle(t, bottles, "Steam", home)
	b, err := m.SharedBottle(filepath.Join(home, "Games", "Demo"))
	if err != nil {
		t.Fatal(err)
	}
	victim := exec.Command("/bin/sleep", "30")
	if err = victim.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- victim.Wait() }()
	//nolint:errcheck // test cleanup; process may already have exited.
	defer victim.Process.Kill()
	original := `Z:\Downloads\setup.exe`
	m.psOutput = func() (string, error) {
		return fmt.Sprintf("%d Sun Sep  6 07:31:35 2026 %s\n", victim.Process.Pid, original), nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = m.Run(ctx, b, Cmd{Path: `C:\bridge\installguard.exe`, StopPaths: []string{original}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run: %v", err)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("installer was not killed")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("wrapped installer survived cancellation")
	}
}

func TestRunRequiresBridgeCompletionAcknowledgement(t *testing.T) {
	manager := NewManager(Runtime{CxStart: "/usr/bin/true"})
	_, err := manager.Run(context.Background(), Bottle{Name: "fixture"}, Cmd{Path: `C:\fixture.exe`, CancelFile: filepath.Join(t.TempDir(), "cancel")})
	if !errors.Is(err, ErrTreeNotStopped) {
		t.Fatalf("unconfirmed bridge exit: %v", err)
	}
}
