package asset

import (
	"bytes"
	"compress/gzip"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func sourceTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for name, body := range map[string]string{
		"go.mod":                                    "module typhon\n",
		"go.sum":                                    "a b\nc d\n",
		"cmd/installguard/main_windows.go":          "package main\n",
		"cmd/installguard/main_windows_test.go":     "package main\n",
		"internal/installguard/policy.go":           "package installguard\n",
		"internal/installguard/policy_test.go":      "package installguard\n",
		"internal/installguard/verifier_windows.go": "package installguard\n",
		"internal/installguard/asset/generate.go":   "package main\n",
		"internal/installguard/asset/sources.go":    "package asset\n",
	} {
		write(t, root, name, body)
	}
	return root
}

func write(t *testing.T, root, name, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func hashOf(t *testing.T, root string) string {
	t.Helper()
	got, err := SourceHash(root)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestSourceHashFileSet(t *testing.T) {
	for _, tc := range []struct {
		name    string
		change  func(t *testing.T, root string)
		changed bool
	}{
		{"edit bridge main", func(t *testing.T, r string) { write(t, r, "cmd/installguard/main_windows.go", "package main // x\n") }, true},
		{"edit guard source", func(t *testing.T, r string) {
			write(t, r, "internal/installguard/policy.go", "package installguard // x\n")
		}, true},
		{"edit verifier source", func(t *testing.T, r string) {
			write(t, r, "internal/installguard/verifier_windows.go", "package installguard // x\n")
		}, true},
		{"new guard source", func(t *testing.T, r string) {
			write(t, r, "internal/installguard/audio_windows.go", "package installguard\n")
		}, true},
		{"new bridge source", func(t *testing.T, r string) { write(t, r, "cmd/installguard/flags.go", "package main\n") }, true},
		{"edit go.mod", func(t *testing.T, r string) { write(t, r, "go.mod", "module typhon\ngo 1.25\n") }, true},
		{"edit go.sum", func(t *testing.T, r string) { write(t, r, "go.sum", "a b\n") }, true},
		{"edit generate.go", func(t *testing.T, r string) {
			write(t, r, "internal/installguard/asset/generate.go", "package main // x\n")
		}, true},
		{"rename guard source", func(t *testing.T, r string) {
			if err := os.Rename(filepath.Join(r, "internal", "installguard", "policy.go"), filepath.Join(r, "internal", "installguard", "rules.go")); err != nil {
				t.Fatal(err)
			}
		}, true},
		{"edit bridge test", func(t *testing.T, r string) {
			write(t, r, "cmd/installguard/main_windows_test.go", "package main // x\n")
		}, false},
		{"edit guard test", func(t *testing.T, r string) {
			write(t, r, "internal/installguard/policy_test.go", "package installguard // x\n")
		}, false},
		{"new test file", func(t *testing.T, r string) {
			write(t, r, "internal/installguard/extra_test.go", "package installguard\n")
		}, false},
		{"edit asset embed source", func(t *testing.T, r string) {
			write(t, r, "internal/installguard/asset/sources.go", "package asset // x\n")
		}, false},
		{"new non-go file", func(t *testing.T, r string) { write(t, r, "internal/installguard/notes.txt", "x") }, false},
		{"new nested go file", func(t *testing.T, r string) { write(t, r, "internal/installguard/nested/x.go", "package nested\n") }, false},
		{"crlf go.sum", func(t *testing.T, r string) { write(t, r, "go.sum", "a b\r\nc d\r\n") }, false},
		{"crlf go.mod", func(t *testing.T, r string) { write(t, r, "go.mod", "module typhon\r\n") }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := sourceTree(t)
			before := hashOf(t, root)
			tc.change(t, root)
			if got := hashOf(t, root) != before; got != tc.changed {
				t.Fatalf("hash changed = %v, want %v", got, tc.changed)
			}
		})
	}
}

func TestSourceHashIsStable(t *testing.T) {
	first, second := sourceTree(t), sourceTree(t)
	if hashOf(t, first) != hashOf(t, second) {
		t.Fatal("identical trees hash differently")
	}
}

func TestSourceHashRejectsIncompleteTree(t *testing.T) {
	for _, missing := range []string{"go.mod", "go.sum", "internal/installguard/asset/generate.go", "cmd/installguard", "internal/installguard"} {
		t.Run(missing, func(t *testing.T) {
			root := sourceTree(t)
			if err := os.RemoveAll(filepath.Join(root, filepath.FromSlash(missing))); err != nil {
				t.Fatal(err)
			}
			if got, err := SourceHash(root); err == nil {
				t.Fatalf("hash %q computed without %s", got, missing)
			}
		})
	}
	if _, err := SourceHash(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("hash computed for a root that does not exist")
	}
}

func TestEmbeddedBridgeIsCurrent(t *testing.T) {
	want, err := SourceHash("../../..")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("helper.exe.gz")
	if err != nil {
		t.Fatal(err)
	}
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if r.Comment != "source-sha256:"+want {
		t.Fatalf("stale Win32 bridge, asset says %q, sources hash to %q: run go generate ./internal/installguard/asset", r.Comment, "source-sha256:"+want)
	}
}

func outsideSources(dirs []string) []string {
	var outside []string
	for _, dir := range dirs {
		if !slices.Contains(sourceDirs, dir) {
			outside = append(outside, dir)
		}
	}
	return outside
}

func TestOutsideSources(t *testing.T) {
	for _, tc := range []struct {
		name string
		dirs []string
		want []string
	}{
		{"bridge and guard", []string{"cmd/installguard", "internal/installguard"}, nil},
		{"guard subpackage", []string{"cmd/installguard", "internal/installguard", "internal/installguard/audio"}, []string{"internal/installguard/audio"}},
		{"other launcher package", []string{"cmd/installguard", "internal/installguard", "internal/uierr"}, []string{"internal/uierr"}},
		{"asset package", []string{"cmd/installguard", "internal/installguard/asset"}, []string{"internal/installguard/asset"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := outsideSources(tc.dirs); !slices.Equal(got, tc.want) {
				t.Fatalf("outside = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBridgeBuildsOnlyFromHashedSources(t *testing.T) {
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "list", "-deps", "-f", "{{if .Module}}{{if .Module.Main}}{{.Dir}}{{end}}{{end}}", "./cmd/installguard")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOOS=windows", "GOARCH=amd64", "CGO_ENABLED=0")
	out, err := cmd.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			t.Fatalf("go list: %v\n%s", err, exit.Stderr)
		}
		t.Fatal(err)
	}
	var dirs []string
	for _, dir := range strings.Fields(string(out)) {
		rel, err := filepath.Rel(root, dir)
		if err != nil {
			t.Fatal(err)
		}
		dirs = append(dirs, filepath.ToSlash(rel))
	}
	if !slices.Contains(dirs, "cmd/installguard") {
		t.Fatalf("go list did not report the bridge itself: %q", dirs)
	}
	if outside := outsideSources(dirs); len(outside) > 0 {
		t.Fatalf("the Win32 bridge is built from %q, which SourceHash does not cover: add the directories to sourceDirs so changes there mark helper.exe.gz stale", outside)
	}
}
