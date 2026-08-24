//go:build windows

package install

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestNeedsElevation(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "прямая ошибка", err: windows.ERROR_ELEVATION_REQUIRED, want: true},
		{name: "как её отдаёт exec", err: &fs.PathError{Op: "fork/exec", Path: `C:\setup.exe`, Err: windows.ERROR_ELEVATION_REQUIRED}, want: true},
		{name: "обёрнутая", err: fmt.Errorf("установщик: %w", windows.ERROR_ELEVATION_REQUIRED), want: true},
		{name: "нет доступа", err: &fs.PathError{Op: "fork/exec", Path: `C:\setup.exe`, Err: windows.ERROR_ACCESS_DENIED}, want: false},
		{name: "нет файла", err: fs.ErrNotExist, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := needsElevation(tc.err); got != tc.want {
				t.Fatalf("needsElevation(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestElevationError(t *testing.T) {
	t.Run("отказ в UAC", func(t *testing.T) {
		err := elevationError(`C:\setup.exe`, windows.ERROR_CANCELLED)
		if !errors.Is(err, errElevationDeclined) {
			t.Fatalf("elevationError = %v, want errElevationDeclined", err)
		}
	})
	t.Run("прочая ошибка сохраняет причину", func(t *testing.T) {
		err := elevationError(`C:\setup.exe`, windows.ERROR_FILE_NOT_FOUND)
		if errors.Is(err, errElevationDeclined) {
			t.Fatalf("elevationError = %v, must not be errElevationDeclined", err)
		}
		if !errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
			t.Fatalf("elevationError = %v, cause lost", err)
		}
	})
}

func TestElevationParams(t *testing.T) {
	cases := []struct {
		name string
		spec runSpec
		want string
	}{
		{name: "пусто", spec: runSpec{Path: `C:\setup.exe`}, want: ""},
		{
			name: "nsis отдаётся дословно",
			spec: runSpec{Path: `C:\setup.exe`, CmdLine: `"C:\setup.exe" /S /D=C:\Games\GTA SA`, Tail: `/S /D=C:\Games\GTA SA`},
			want: `/S /D=C:\Games\GTA SA`,
		},
		{
			name: "аргументы экранируются только при необходимости",
			spec: runSpec{Path: `C:\setup.exe`, Args: []string{"/VERYSILENT", `/DIR=C:\Games\GTA SA`}},
			want: `/VERYSILENT "/DIR=C:\Games\GTA SA"`,
		},
		{
			name: "хвост важнее аргументов",
			spec: runSpec{Path: `C:\setup.exe`, Args: []string{"/S"}, Tail: `/S /D=C:\Games`},
			want: `/S /D=C:\Games`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := elevationParams(tc.spec); got != tc.want {
				t.Fatalf("elevationParams = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSilentSpecKeepsNsisTail(t *testing.T) {
	item := Installation{Engine: EngineNsis, InstallerPath: `C:\downloads\setup.exe`, Destination: `C:\Games\GTA SA`}
	spec, err := silentSpec(item, item.InstallerPath, "", installOptions{})
	if err != nil {
		t.Fatalf("silentSpec error = %v", err)
	}
	want := `/S /D=C:\Games\GTA SA`
	if spec.Tail != want {
		t.Fatalf("Tail = %q, want %q", spec.Tail, want)
	}
	if spec.CmdLine != quoteArg(item.InstallerPath)+" "+want {
		t.Fatalf("CmdLine = %q, does not match Tail %q", spec.CmdLine, want)
	}
}

func TestStartElevatedRejectsBadPath(t *testing.T) {
	if _, err := startElevated(runSpec{Path: "C:" + string(rune(0)) + `\setup.exe`}); err == nil {
		t.Fatal("startElevated с нулевым байтом в пути вернул успех")
	}
}

func TestStartElevatedMissingFile(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "setup.exe")
	done := make(chan error, 1)
	go func() {
		proc, err := startElevated(runSpec{Path: missing, Hidden: true})
		if proc != nil {
			if termErr := proc.terminate(); termErr != nil {
				t.Errorf("terminate error = %v", termErr)
			}
			proc.close()
		}
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("startElevated несуществующего файла вернул успех")
		}
		if errors.Is(err, errNoElevatedProcess) {
			t.Fatalf("ShellExecuteEx отчитался об успехе без процесса: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("startElevated завис")
	}
}

func TestCreateProcessReportsElevation(t *testing.T) {
	if windows.GetCurrentProcessToken().IsElevated() {
		t.Skip("тест идёт с правами администратора: ERROR_ELEVATION_REQUIRED не воспроизводится")
	}
	const path = `C:\Windows\regedit.exe`
	if _, statErr := os.Stat(path); statErr != nil {
		t.Skipf("нет %s: %v", path, statErr)
	}
	cmd := exec.Command(path)
	if startErr := cmd.Start(); startErr != nil {
		if !needsElevation(startErr) {
			t.Fatalf("start %s error = %v, want ERROR_ELEVATION_REQUIRED", path, startErr)
		}
		return
	}
	if killErr := cmd.Process.Kill(); killErr != nil {
		t.Fatalf("kill %s error = %v", path, killErr)
	}
	if waitErr := cmd.Wait(); waitErr != nil {
		var exit *exec.ExitError
		if !errors.As(waitErr, &exit) {
			t.Fatalf("wait %s error = %v", path, waitErr)
		}
	}
	t.Skipf("%s запустился без повышения прав", path)
}
