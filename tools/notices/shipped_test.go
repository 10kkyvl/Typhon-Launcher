package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// shippedTargets are the release builds: the Windows installer and the macOS
// bundle. Each compiles a different set of modules into the executable.
var shippedTargets = []struct{ goos, goarch, cgo string }{
	{"windows", "amd64", "0"},
	{"darwin", "arm64", "1"},
}

func shippedModules(t *testing.T) map[string]string {
	t.Helper()
	repo, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	mods := map[string]string{}
	for _, target := range shippedTargets {
		cmd := exec.Command("go", "list", "-deps", "-tags", "production",
			"-f", "{{with .Module}}{{if not .Main}}{{.Path}} {{.Version}}{{end}}{{end}}", ".")
		cmd.Dir = repo
		cmd.Env = append(os.Environ(), "GOOS="+target.goos, "GOARCH="+target.goarch, "CGO_ENABLED="+target.cgo)
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("go list for %s/%s: %v", target.goos, target.goarch, err)
		}
		for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			path, version, ok := strings.Cut(line, " ")
			if !ok {
				t.Fatalf("go list for %s/%s: module %q has no version", target.goos, target.goarch, line)
			}
			mods[path] = version
		}
	}
	if len(mods) == 0 {
		t.Fatal("go list found no modules in the release builds")
	}
	return mods
}

func TestManifestCoversShippedModules(t *testing.T) {
	m, err := loadManifest("manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	listed := map[string]string{}
	for _, c := range m.Components {
		if c.Source == "go" && !strings.HasPrefix(c.Name, "@") {
			listed[c.Name] = c.Version
		}
	}
	shipped := shippedModules(t)
	for path, version := range shipped {
		got, ok := listed[path]
		switch {
		case !ok:
			t.Errorf("%s %s is compiled into a release build but missing from manifest.json", path, version)
		case got != version:
			t.Errorf("%s: manifest.json has %s, the release build compiles %s", path, got, version)
		}
	}
	for path, version := range listed {
		if _, ok := shipped[path]; !ok {
			t.Errorf("%s %s is in manifest.json but no release build compiles it", path, version)
		}
	}
}
