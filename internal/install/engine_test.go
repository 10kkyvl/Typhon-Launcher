package install

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeStreamedInstaller(t *testing.T, path string, size int64, marker string, markerOffset int64) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil {
			t.Fatalf("close %s: %v", path, cerr)
		}
	}()

	const chunkSize = 64 * 1024
	junk := bytes.Repeat([]byte{'a'}, chunkSize)
	markerBytes := []byte(marker)

	var written int64
	for written < size {
		if len(markerBytes) > 0 && written <= markerOffset && markerOffset < written+chunkSize {
			pre := markerOffset - written
			if pre > 0 {
				if _, err := f.Write(junk[:pre]); err != nil {
					t.Fatalf("write %s: %v", path, err)
				}
			}
			if _, err := f.Write(markerBytes); err != nil {
				t.Fatalf("write marker %s: %v", path, err)
			}
			written += pre + int64(len(markerBytes))
			markerBytes = nil
			continue
		}
		remain := size - written
		n := int64(chunkSize)
		if remain < n {
			n = remain
		}
		if n <= 0 {
			break
		}
		if _, err := f.Write(junk[:n]); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
		written += n
	}
}

func TestDetectEngine(t *testing.T) {
	dir := t.TempDir()

	cases := []struct {
		name    string
		path    string
		setup   func(path string)
		want    Engine
		wantErr bool
	}{
		{
			name: "inno marker",
			path: filepath.Join(dir, "inno.exe"),
			setup: func(path string) {
				writeStreamedInstaller(t, path, int64(len("Inno Setup Setup Data")), "Inno Setup Setup Data", 0)
			},
			want: EngineInno,
		},
		{
			name: "nsis marker",
			path: filepath.Join(dir, "nsis.exe"),
			setup: func(path string) {
				writeStreamedInstaller(t, path, int64(len("NullsoftInst")), "NullsoftInst", 0)
			},
			want: EngineNsis,
		},
		{
			name: "installshield marker",
			path: filepath.Join(dir, "is.exe"),
			setup: func(path string) {
				writeStreamedInstaller(t, path, int64(len("InstallShield")), "InstallShield", 0)
			},
			want: EngineInstallShield,
		},
		{
			name: "no markers",
			path: filepath.Join(dir, "plain.exe"),
			setup: func(path string) {
				writeStreamedInstaller(t, path, 4096, "", 0)
			},
			want: EngineUnknown,
		},
		{
			name: "msi extension",
			path: filepath.Join(dir, "setup.msi"),
			setup: func(path string) {
				if err := os.WriteFile(path, nil, 0o644); err != nil {
					t.Fatalf("write %s: %v", path, err)
				}
			},
			want: EngineMsi,
		},
		{
			name:    "missing path",
			path:    filepath.Join(dir, "missing.exe"),
			wantErr: true,
		},
		{
			name:    "empty path",
			path:    "",
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.setup != nil {
				tc.setup(tc.path)
			}
			got, err := DetectEngine(tc.path)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("DetectEngine(%q) = %v, nil; want error", tc.path, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("DetectEngine(%q) error = %v", tc.path, err)
			}
			if got != tc.want {
				t.Fatalf("DetectEngine(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

func TestDetectEngineMarkerAtChunkBoundary(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "boundary.exe")

	marker := "Inno Setup Setup Data"
	offset := int64(engineScanChunk) - 5
	size := int64(engineScanChunk) + 4096

	writeStreamedInstaller(t, path, size, marker, offset)

	got, err := DetectEngine(path)
	if err != nil {
		t.Fatalf("DetectEngine error = %v", err)
	}
	if got != EngineInno {
		t.Fatalf("DetectEngine at chunk boundary = %v, want %v", got, EngineInno)
	}
}

func TestDetectEngineMarkerBeyondScanLimit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "toolarge.exe")

	marker := "InstallShield"
	offset := int64(engineScanMax) + 512*1024
	size := int64(engineScanMax) + 1024*1024

	writeStreamedInstaller(t, path, size, marker, offset)

	got, err := DetectEngine(path)
	if err != nil {
		t.Fatalf("DetectEngine error = %v", err)
	}
	if got != EngineUnknown {
		t.Fatalf("DetectEngine beyond scan limit = %v, want %v", got, EngineUnknown)
	}
}

func TestDetectEngineInnoTakesPriorityOverInstallShield(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mixed.exe")

	content := []byte("InstallShield")
	content = append(content, bytes.Repeat([]byte{'a'}, 100)...)
	content = append(content, []byte("Inno Setup Setup Data")...)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}

	got, err := DetectEngine(path)
	if err != nil {
		t.Fatalf("DetectEngine error = %v", err)
	}
	if got != EngineInno {
		t.Fatalf("DetectEngine with both markers = %v, want %v", got, EngineInno)
	}
}

func TestSilentArgs(t *testing.T) {
	dir := t.TempDir()
	installerPath := filepath.Join(dir, "Program Setup.exe")

	t.Run("inno strips trailing separator", func(t *testing.T) {
		dest := filepath.Join(dir, "My Game") + string(filepath.Separator)
		wantDest := filepath.Clean(dest)

		plan, err := silentArgs(EngineInno, installerPath, dest, "", installOptions{})
		if err != nil {
			t.Fatalf("silentArgs error = %v", err)
		}
		found := false
		for _, a := range plan.Args {
			if a == "/DIR="+wantDest {
				found = true
			}
			if strings.HasSuffix(a, string(filepath.Separator)) && strings.HasPrefix(a, "/DIR=") {
				t.Fatalf("/DIR= arg has trailing separator: %q", a)
			}
		}
		if !found {
			t.Fatalf("args %v missing /DIR=%s", plan.Args, wantDest)
		}
	})

	t.Run("inno with log path", func(t *testing.T) {
		logPath := filepath.Join(dir, "install.log")
		dest := filepath.Join(dir, "Dest1")
		plan, err := silentArgs(EngineInno, installerPath, dest, logPath, installOptions{})
		if err != nil {
			t.Fatalf("silentArgs error = %v", err)
		}
		found := false
		for _, a := range plan.Args {
			if a == "/LOG="+logPath {
				found = true
			}
		}
		if !found {
			t.Fatalf("args %v missing /LOG=%s", plan.Args, logPath)
		}
	})

	t.Run("nsis builds raw cmdline", func(t *testing.T) {
		dest := filepath.Join(dir, "Dest2") + string(filepath.Separator)
		wantDest := filepath.Clean(dest)

		plan, err := silentArgs(EngineNsis, installerPath, dest, "", installOptions{})
		if err != nil {
			t.Fatalf("silentArgs error = %v", err)
		}
		if len(plan.Args) != 0 {
			t.Fatalf("nsis plan.Args = %v, want empty", plan.Args)
		}
		wantSuffix := "/S /D=" + wantDest
		if !strings.HasSuffix(plan.CmdLine, wantSuffix) {
			t.Fatalf("CmdLine %q does not end with %q", plan.CmdLine, wantSuffix)
		}
		if strings.Contains(plan.CmdLine, `"`+wantDest+`"`) {
			t.Fatalf("CmdLine %q must not quote the destination", plan.CmdLine)
		}
		if !strings.HasPrefix(plan.CmdLine, quoteArg(installerPath)) {
			t.Fatalf("CmdLine %q does not start with quoted installer path", plan.CmdLine)
		}
	})

	t.Run("msi has qn and targetdir", func(t *testing.T) {
		dest := filepath.Join(dir, "Dest3")
		plan, err := silentArgs(EngineMsi, installerPath, dest, "", installOptions{})
		if err != nil {
			t.Fatalf("silentArgs error = %v", err)
		}
		hasQn, hasTarget := false, false
		for _, a := range plan.Args {
			if a == "/qn" {
				hasQn = true
			}
			if a == "TARGETDIR="+dest {
				hasTarget = true
			}
		}
		if !hasQn {
			t.Fatalf("msi args %v missing /qn", plan.Args)
		}
		if !hasTarget {
			t.Fatalf("msi args %v missing TARGETDIR=%s", plan.Args, dest)
		}
	})

	t.Run("installshield unsupported", func(t *testing.T) {
		dest := filepath.Join(dir, "Dest4")
		_, err := silentArgs(EngineInstallShield, installerPath, dest, "", installOptions{})
		if !errors.Is(err, errNoSilent) {
			t.Fatalf("silentArgs error = %v, want errNoSilent", err)
		}
	})

	t.Run("unknown unsupported", func(t *testing.T) {
		dest := filepath.Join(dir, "Dest5")
		_, err := silentArgs(EngineUnknown, installerPath, dest, "", installOptions{})
		if !errors.Is(err, errNoSilent) {
			t.Fatalf("silentArgs error = %v, want errNoSilent", err)
		}
	})

	t.Run("empty destination", func(t *testing.T) {
		_, err := silentArgs(EngineInno, installerPath, "", "", installOptions{})
		if !errors.Is(err, errEmptyDestination) {
			t.Fatalf("silentArgs error = %v, want errEmptyDestination", err)
		}
	})

	t.Run("relative destination", func(t *testing.T) {
		_, err := silentArgs(EngineInno, installerPath, "relative/dest", "", installOptions{})
		if !errors.Is(err, errRelativeDestination) {
			t.Fatalf("silentArgs error = %v, want errRelativeDestination", err)
		}
	})

	t.Run("empty installer path", func(t *testing.T) {
		dest := filepath.Join(dir, "Dest6")
		_, err := silentArgs(EngineInno, "", dest, "", installOptions{})
		if !errors.Is(err, errNoExecutable) {
			t.Fatalf("silentArgs error = %v, want errNoExecutable", err)
		}
	})
}

func TestExitError(t *testing.T) {
	cases := []struct {
		name    string
		engine  Engine
		code    int
		wantNil bool
		wantErr error
	}{
		{name: "success", engine: EngineInno, code: 0, wantNil: true},
		{name: "reboot required", engine: EngineNsis, code: 3010, wantNil: true},
		{name: "msi reboot initiated", engine: EngineMsi, code: 1641, wantNil: true},
		{name: "inno cancelled 2", engine: EngineInno, code: 2, wantErr: errInstallerCancelled},
		{name: "inno cancelled 5", engine: EngineInno, code: 5, wantErr: errInstallerCancelled},
		{name: "msi cancelled", engine: EngineMsi, code: 1602, wantErr: errInstallerCancelled},
		{name: "msi busy", engine: EngineMsi, code: 1618, wantErr: errInstallerBusy},
		{name: "msi generic failure", engine: EngineMsi, code: 1603, wantErr: errInstallerFail},
		{name: "nsis generic failure", engine: EngineNsis, code: 1, wantErr: errInstallerFail},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := exitError(tc.engine, tc.code)
			if tc.wantNil {
				if err != nil {
					t.Fatalf("exitError(%v, %d) = %v, want nil", tc.engine, tc.code, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("exitError(%v, %d) = nil, want error wrapping %v", tc.engine, tc.code, tc.wantErr)
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("exitError(%v, %d) = %v, want wrapping %v", tc.engine, tc.code, err, tc.wantErr)
			}
			if !strings.Contains(err.Error(), "1603") && tc.code == 1603 {
				t.Fatalf("exitError message %q does not contain code", err.Error())
			}
		})
	}
}

func TestQuoteArg(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "path with space",
			in:   `C:\Program Files\App\setup.exe`,
			want: `"C:\Program Files\App\setup.exe"`,
		},
		{
			name: "trailing backslash",
			in:   `C:\Program Files\App\`,
			want: `"C:\Program Files\App\\"`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := quoteArg(tc.in)
			if got != tc.want {
				t.Fatalf("quoteArg(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestSilentArgsDeclinesExtras(t *testing.T) {
	dir := t.TempDir()
	installerPath := filepath.Join(dir, "setup.exe")
	dest := filepath.Join(dir, "Game")

	cases := []struct {
		name        string
		opts        installOptions
		wantNoIcons bool
		wantTasks   []string
		denyTasks   []string
	}{
		{
			name:        "по умолчанию отклоняем всё",
			opts:        installOptions{SkipShortcuts: true, SkipExtras: true},
			wantNoIcons: true,
			wantTasks:   []string{"!desktopicon", "!quicklaunchicon", "!directx", "!vcredist", "!dotnet", "!associate"},
		},
		{
			name:        "только ярлыки",
			opts:        installOptions{SkipShortcuts: true},
			wantNoIcons: true,
			wantTasks:   []string{"!desktopicon"},
			denyTasks:   []string{"!directx", "!vcredist"},
		},
		{
			name:        "только допы",
			opts:        installOptions{SkipExtras: true},
			wantNoIcons: false,
			wantTasks:   []string{"!directx"},
			denyTasks:   []string{"!desktopicon"},
		},
		{
			name:        "ничего не отклоняем",
			opts:        installOptions{},
			wantNoIcons: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := silentArgs(EngineInno, installerPath, dest, "", tc.opts)
			if err != nil {
				t.Fatalf("silentArgs error = %v", err)
			}
			joined := strings.Join(plan.Args, " ")
			if got := strings.Contains(joined, "/NOICONS"); got != tc.wantNoIcons {
				t.Fatalf("args %v: /NOICONS = %v, want %v", plan.Args, got, tc.wantNoIcons)
			}
			tasks := ""
			for _, a := range plan.Args {
				if strings.HasPrefix(a, "/MERGETASKS=") {
					tasks = strings.TrimPrefix(a, "/MERGETASKS=")
				}
			}
			if len(tc.wantTasks) == 0 {
				if tasks != "" {
					t.Fatalf("/MERGETASKS не должен появляться: %q", tasks)
				}
				return
			}
			for _, want := range tc.wantTasks {
				if !strings.Contains(","+tasks+",", ","+want+",") {
					t.Fatalf("/MERGETASKS=%q не содержит %q", tasks, want)
				}
			}
			for _, deny := range tc.denyTasks {
				if strings.Contains(","+tasks+",", ","+deny+",") {
					t.Fatalf("/MERGETASKS=%q не должен содержать %q", tasks, deny)
				}
			}
			if !strings.Contains(joined, "/DIR="+dest) {
				t.Fatalf("args %v потеряли /DIR=", plan.Args)
			}
		})
	}
}

func TestSilentArgsNsisIgnoresOptions(t *testing.T) {
	dir := t.TempDir()
	installerPath := filepath.Join(dir, "setup.exe")
	dest := filepath.Join(dir, "Game")

	plain, err := silentArgs(EngineNsis, installerPath, dest, "", installOptions{})
	if err != nil {
		t.Fatalf("silentArgs error = %v", err)
	}
	declined, err := silentArgs(EngineNsis, installerPath, dest, "", installOptions{SkipShortcuts: true, SkipExtras: true})
	if err != nil {
		t.Fatalf("silentArgs error = %v", err)
	}
	if plain.CmdLine != declined.CmdLine || plain.Tail != declined.Tail {
		t.Fatalf("NSIS не принимает ключи отказа, строка не должна меняться: %q против %q", plain.CmdLine, declined.CmdLine)
	}
}
