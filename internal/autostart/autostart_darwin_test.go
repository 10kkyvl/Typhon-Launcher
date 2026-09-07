//go:build darwin

package autostart

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func stubLaunchctl(t *testing.T) {
	t.Helper()
	origBootstrap := launchctlBootstrap
	origBootout := launchctlBootout
	launchctlBootstrap = func(string) error { return nil }
	launchctlBootout = func(string) error { return nil }
	t.Cleanup(func() {
		launchctlBootstrap = origBootstrap
		launchctlBootout = origBootout
	})
}

func TestDarwinLaunchAgentRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	stubLaunchctl(t)
	mgr := darwinLaunchAgent{}

	enabled, err := mgr.IsEnabled()
	if err != nil {
		t.Fatalf("IsEnabled before Enable: %v", err)
	}
	if enabled {
		t.Fatal("expected disabled before Enable")
	}

	if err := mgr.Enable(); err != nil {
		t.Fatalf("Enable: %v", err)
	}

	enabled, err = mgr.IsEnabled()
	if err != nil {
		t.Fatalf("IsEnabled after Enable: %v", err)
	}
	if !enabled {
		t.Fatal("expected enabled after Enable")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	path := filepath.Join(home, "Library", "LaunchAgents", darwinLabel+".plist")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read plist: %v", err)
	}
	body := string(data)
	for _, want := range []string{
		"<key>Label</key>",
		"<string>" + darwinLabel + "</string>",
		"<key>ProgramArguments</key>",
		"<key>RunAtLoad</key>",
		"<true/>",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("plist missing %q\n---\n%s", want, body)
		}
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat plist: %v", err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Errorf("plist mode = %v, want 0644", info.Mode().Perm())
	}

	if err := mgr.Disable(); err != nil {
		t.Fatalf("Disable: %v", err)
	}
	enabled, err = mgr.IsEnabled()
	if err != nil {
		t.Fatalf("IsEnabled after Disable: %v", err)
	}
	if enabled {
		t.Fatal("expected disabled after Disable")
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("plist should be removed, stat err = %v", err)
	}
}

func TestDarwinLaunchAgentDisableNoOp(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	stubLaunchctl(t)
	if err := (darwinLaunchAgent{}).Disable(); err != nil {
		t.Fatalf("Disable when nothing registered should be nil, got %v", err)
	}
}

// Бандл может переехать (переустановка в другую папку), а старый plist от
// прежнего пути остаться на диске. IsEnabled должен считать это «не
// зарегистрировано для текущего бинаря», а не молча соврать «включено» —
// иначе Apply(true) никогда не перезапишет plist на верный путь.
func TestDarwinLaunchAgentPathMismatchIsNotEnabled(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	stubLaunchctl(t)
	mgr := darwinLaunchAgent{}
	if err := mgr.Enable(); err != nil {
		t.Fatalf("Enable: %v", err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	path := filepath.Join(home, "Library", "LaunchAgents", darwinLabel+".plist")
	if err := os.WriteFile(path, []byte(launchAgentPlist("/definitely/not/this/binary")), 0o644); err != nil {
		t.Fatalf("rewrite plist: %v", err)
	}
	enabled, err := mgr.IsEnabled()
	if err != nil {
		t.Fatalf("IsEnabled with mismatched path: %v", err)
	}
	if enabled {
		t.Fatal("stale registration for a different executable must not read as enabled")
	}
}

// Файл лежит под нашим фиксированным именем, поэтому битый plist там — это
// порча состояния, а не «автозапуск просто не настроен»: IsEnabled должен
// вернуть ошибку, а не молчаливый false.
func TestDarwinLaunchAgentCorruptPlistIsAnError(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	stubLaunchctl(t)
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	dir := filepath.Join(home, "Library", "LaunchAgents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(dir, darwinLabel+".plist")
	if err := os.WriteFile(path, []byte("not a plist"), 0o644); err != nil {
		t.Fatalf("write corrupt plist: %v", err)
	}
	if _, err := (darwinLaunchAgent{}).IsEnabled(); err == nil {
		t.Fatal("corrupt plist under our own label must surface as an error, not a silent false")
	}
}

func TestLaunchAgentExecutableParsing(t *testing.T) {
	body := launchAgentPlist("/path/to/exe")
	got, err := launchAgentExecutable([]byte(body))
	if err != nil {
		t.Fatalf("launchAgentExecutable: %v", err)
	}
	if got != "/path/to/exe" {
		t.Errorf("got %q, want /path/to/exe", got)
	}
	if _, err := launchAgentExecutable([]byte("not a plist")); err == nil {
		t.Fatal("malformed input must return an error")
	}
}

func TestForPlatformOverridesOnDarwin(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	fallback := &fakeManager{}
	got := ForPlatform(fallback)
	if _, ok := got.(darwinLaunchAgent); !ok {
		t.Fatalf("ForPlatform on darwin must return darwinLaunchAgent, got %T", got)
	}
	if _, err := got.IsEnabled(); err != nil {
		t.Fatalf("IsEnabled via ForPlatform: %v", err)
	}
	if fallback.isEnabledCalls != 0 {
		t.Fatal("darwin override must not delegate to the fallback manager")
	}
}
