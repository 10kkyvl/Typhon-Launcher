//go:build windows

package selfupdate

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"golang.org/x/sys/windows"

	"typhon/internal/uierr"
)

const (
	applyHelperEnv      = "TYPHON_SELFUPDATE_APPLY_HELPER"
	applyHelperLogEnv   = "TYPHON_SELFUPDATE_APPLY_LOG_DIR"
	applyHelperLogName  = "installer-ran.txt"
	applyHelperEventEnv = "TYPHON_SELFUPDATE_APPLY_EVENT"
	applyHelperFailCode = 7
)

// runApplyHelper is the installer stand-in: Apply starts a copy of this test
// binary with a fixed command line, so the only way to steer it is through the
// environment, and TestMain hands control here before any test runs.
func runApplyHelper(mode string) int {
	record := strconv.Itoa(os.Getpid()) + "\n" + windows.UTF16PtrToString(windows.GetCommandLine())
	root, err := os.OpenRoot(os.Getenv(applyHelperLogEnv))
	if err != nil {
		return 90
	}
	logFile, err := root.OpenFile(applyHelperLogName, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return 90
	}
	if _, err := logFile.WriteString(record); err != nil {
		return 90
	}
	if err := logFile.Close(); err != nil {
		return 90
	}
	if err := root.Close(); err != nil {
		return 90
	}
	if name := os.Getenv(applyHelperEventEnv); name != "" {
		ptr, err := windows.UTF16PtrFromString(name)
		if err != nil {
			return 91
		}
		event, err := windows.OpenEvent(windows.EVENT_MODIFY_STATE, false, ptr)
		if err != nil {
			return 92
		}
		if err := windows.SetEvent(event); err != nil {
			return 93
		}
	}
	switch mode {
	case "ok":
		return 0
	case "fail":
		return applyHelperFailCode
	case "hang":
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, os.Interrupt)
		<-stop
		return 0
	}
	return 99
}

func stageApplyInstaller(t *testing.T) (configDir, installerPath string) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	data, err := os.ReadFile(exe)
	if err != nil {
		t.Fatalf("read test binary: %v", err)
	}
	return stageApplyInstallerBytes(t, data)
}

func stageApplyInstallerBytes(t *testing.T, data []byte) (configDir, installerPath string) {
	t.Helper()
	configDir = testConfigDir(t)
	installerPath, err := ArtifactPath(configDir, "1.2.3", "typhon-setup.exe")
	if err != nil {
		t.Fatalf("ArtifactPath: %v", err)
	}
	writeTestFile(t, installerPath, data)

	art := Artifact{
		OS: "windows", Arch: "amd64", Kind: KindInstaller, Name: "typhon-setup.exe",
		URL: "https://example.com/typhon-setup.exe", Size: int64(len(data)), SHA256: sha256Hex(t, data),
	}
	if err := mustStore(t, configDir).Save(stored{AvailableVersion: "1.2.3", Artifact: &art, ReadyPath: installerPath}); err != nil {
		t.Fatalf("seed store: %v", err)
	}
	return configDir, installerPath
}

func useApplyHelper(t *testing.T, mode string) (logPath string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv(applyHelperEnv, mode)
	t.Setenv(applyHelperLogEnv, dir)
	return filepath.Join(dir, applyHelperLogName)
}

var applyEventSeq atomic.Int64

func armApplyEvent(t *testing.T) windows.Handle {
	t.Helper()
	name := fmt.Sprintf("Local\\typhon-selfupdate-apply-%d-%d", os.Getpid(), applyEventSeq.Add(1))
	ptr, err := windows.UTF16PtrFromString(name)
	if err != nil {
		t.Fatalf("event name: %v", err)
	}
	event, err := windows.CreateEvent(nil, 1, 0, ptr)
	if err != nil {
		t.Fatalf("CreateEvent: %v", err)
	}
	t.Cleanup(func() {
		if err := windows.CloseHandle(event); err != nil {
			t.Errorf("close event: %v", err)
		}
	})
	t.Setenv(applyHelperEventEnv, name)
	return event
}

func awaitApplyEvent(t *testing.T, event windows.Handle) bool {
	t.Helper()
	const timeoutMillis = 60000
	result, err := windows.WaitForSingleObject(event, timeoutMillis)
	if err != nil {
		t.Fatalf("WaitForSingleObject: %v", err)
	}
	return result == windows.WAIT_OBJECT_0
}

func readApplyLog(t *testing.T, path string) (pid int, cmdLine string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the installer never ran: %v", err)
	}
	pidText, cmdLine, ok := strings.Cut(string(data), "\n")
	if !ok {
		t.Fatalf("malformed installer record %q", data)
	}
	pid, err = strconv.Atoi(pidText)
	if err != nil {
		t.Fatalf("installer pid %q: %v", pidText, err)
	}
	return pid, cmdLine
}

func TestApplyHandsTheInstallerItsSilentCommandLine(t *testing.T) {
	_, installer := stageApplyInstaller(t)
	logPath := useApplyHelper(t, "ok")
	installDir := filepath.Join(t.TempDir(), "Program Files", "Typhon")
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	if err := Apply(context.Background(), installer, installDir, filepath.Join(installDir, "typhon.exe")); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	_, cmdLine := readApplyLog(t, logPath)
	want := `"` + installer + `" /S /D=` + installDir
	if cmdLine != want {
		t.Fatalf("installer command line = %q, want %q: NSIS reads /D= literally to the end of the line, so it must come last and unquoted", cmdLine, want)
	}
}

func TestApplyReportsAnInstallerThatFails(t *testing.T) {
	_, installer := stageApplyInstaller(t)
	useApplyHelper(t, "fail")
	installDir := t.TempDir()

	err := Apply(context.Background(), installer, installDir, filepath.Join(installDir, "typhon.exe"))
	if err == nil {
		t.Fatal("Apply() error = nil for an installer that exited with an error")
	}
	var coded *uierr.Error
	if !errors.As(err, &coded) || coded.Code() != "selfupdate.installer_failed" {
		t.Fatalf("Apply() error = %v, want a selfupdate.installer_failed error the interface can show", err)
	}
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != applyHelperFailCode {
		t.Fatalf("Apply() error = %v, want the installer's exit code %d preserved", err, applyHelperFailCode)
	}
}

func TestApplyNeverStartsAnInstallerItCouldNotVerify(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T, configDir, installer, installDir string) error
		want error
	}{
		{
			name: "bytes changed after the download",
			run: func(t *testing.T, _, installer, installDir string) error {
				data, err := os.ReadFile(installer)
				if err != nil {
					t.Fatalf("read installer: %v", err)
				}
				data[len(data)/2] ^= 0xff
				writeTestFile(t, installer, data)
				return Apply(context.Background(), installer, installDir, filepath.Join(installDir, "typhon.exe"))
			},
			want: ErrHashMismatch,
		},
		{
			name: "file shorter than recorded",
			run: func(t *testing.T, _, installer, installDir string) error {
				data, err := os.ReadFile(installer)
				if err != nil {
					t.Fatalf("read installer: %v", err)
				}
				writeTestFile(t, installer, data[:len(data)-1])
				return Apply(context.Background(), installer, installDir, filepath.Join(installDir, "typhon.exe"))
			},
			want: ErrSizeMismatch,
		},
		{
			name: "install directory is gone",
			run: func(_ *testing.T, _, installer, installDir string) error {
				return Apply(context.Background(), installer, filepath.Join(installDir, "gone"), filepath.Join(installDir, "typhon.exe"))
			},
			want: fs.ErrNotExist,
		},
		{
			name: "path is not the one recorded as ready",
			run: func(t *testing.T, configDir, installer, installDir string) error {
				data, err := os.ReadFile(installer)
				if err != nil {
					t.Fatalf("read installer: %v", err)
				}
				other, err := ArtifactPath(configDir, "1.2.4", "typhon-setup.exe")
				if err != nil {
					t.Fatalf("ArtifactPath: %v", err)
				}
				writeTestFile(t, other, data)
				return Apply(context.Background(), other, installDir, filepath.Join(installDir, "typhon.exe"))
			},
			want: ErrNotReady,
		},
		{
			name: "context already cancelled",
			run: func(_ *testing.T, _, installer, installDir string) error {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return Apply(ctx, installer, installDir, filepath.Join(installDir, "typhon.exe"))
			},
			want: context.Canceled,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configDir, installer := stageApplyInstallerBytes(t, bytes.Repeat([]byte("MZ installer stand-in "), 200))
			installDir := t.TempDir()

			err := tt.run(t, configDir, installer, installDir)
			if !errors.Is(err, tt.want) {
				t.Fatalf("Apply() error = %v, want %v (the stand-in is not a program: an installer_failed error means Apply tried to run it)", err, tt.want)
			}
		})
	}
}

func TestApplyStopsTheInstallerWhenTheContextEnds(t *testing.T) {
	_, installer := stageApplyInstaller(t)
	logPath := useApplyHelper(t, "hang")
	installDir := t.TempDir()

	started := armApplyEvent(t)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	done := make(chan error, 1)
	go func() {
		done <- Apply(ctx, installer, installDir, filepath.Join(installDir, "typhon.exe"))
	}()

	if !awaitApplyEvent(t, started) {
		cancel()
		t.Fatalf("the installer did not come up; Apply returned %v", <-done)
	}
	pid, _ := readApplyLog(t, logPath)

	cancel()
	if err := <-done; err == nil {
		t.Fatal("Apply() error = nil after the context ended and the installer was killed")
	}
	alive, err := workerProcessAlive(pid)
	if err != nil {
		t.Fatalf("workerProcessAlive: %v", err)
	}
	if alive {
		t.Fatalf("installer process %d is still running after Apply returned", pid)
	}
}
