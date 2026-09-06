package wine

import (
	"context"
	"os"
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
