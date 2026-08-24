package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestValidateComponent(t *testing.T) {
	cases := []struct {
		name    string
		c       Component
		wantErr bool
	}{
		{
			name: "valid go component",
			c: Component{
				Name: "example.com/mod", Version: "v1.0.0", License: "MIT",
				Source: "go", LicenseFile: "LICENSE",
			},
		},
		{
			name: "valid npm component",
			c: Component{
				Name: "left-pad", Version: "1.0.0", License: "MIT",
				Source: "npm", LicenseFile: "LICENSE",
			},
		},
		{
			name: "valid manual review component",
			c: Component{
				Name: "vendor blob", License: "unknown",
				ManualReview: true, Note: "no local license file",
			},
		},
		{
			name:    "empty name",
			c:       Component{License: "MIT", Source: "go", Version: "v1.0.0", LicenseFile: "LICENSE"},
			wantErr: true,
		},
		{
			name:    "empty license",
			c:       Component{Name: "mod", Source: "go", Version: "v1.0.0", LicenseFile: "LICENSE"},
			wantErr: true,
		},
		{
			name:    "manual review without note",
			c:       Component{Name: "mod", License: "MIT", ManualReview: true},
			wantErr: true,
		},
		{
			name:    "unknown source",
			c:       Component{Name: "mod", License: "MIT", Source: "cargo", Version: "1", LicenseFile: "LICENSE"},
			wantErr: true,
		},
		{
			name:    "missing version",
			c:       Component{Name: "mod", License: "MIT", Source: "go", LicenseFile: "LICENSE"},
			wantErr: true,
		},
		{
			name:    "missing license file",
			c:       Component{Name: "mod", License: "MIT", Source: "go", Version: "v1.0.0"},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateComponent(tc.c)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestLicensePath(t *testing.T) {
	cases := []struct {
		name    string
		c       Component
		want    string
		wantErr bool
	}{
		{
			name: "go module",
			c:    Component{Name: "github.com/foo/bar", Version: "v1.2.3", Source: "go", LicenseFile: "LICENSE"},
			want: filepath.Join("cache", "github.com/foo/bar@v1.2.3", "LICENSE"),
		},
		{
			name: "npm package",
			c:    Component{Name: "left-pad", Version: "1.0.0", Source: "npm", LicenseFile: "LICENSE.md"},
			want: filepath.Join("npm", "left-pad", "LICENSE.md"),
		},
		{
			name:    "unknown source",
			c:       Component{Name: "foo", Source: "cargo"},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := licensePath(tc.c, "cache", "npm")
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestReadLicenseText(t *testing.T) {
	dir := t.TempDir()

	present := filepath.Join(dir, "LICENSE")
	writeFile(t, present, "  MIT License text  \n")

	empty := filepath.Join(dir, "EMPTY")
	writeFile(t, empty, "   \n")

	missing := filepath.Join(dir, "MISSING")

	cases := []struct {
		name    string
		path    string
		want    string
		wantErr bool
	}{
		{name: "present and trimmed", path: present, want: "MIT License text"},
		{name: "empty file is an error", path: empty, wantErr: true},
		{name: "missing file is an error", path: missing, wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := readLicenseText(tc.path)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBuildGroupsGroupsIdenticalTexts(t *testing.T) {
	gomod := t.TempDir()
	npm := t.TempDir()

	writeFile(t, filepath.Join(gomod, "github.com/a/one@v1.0.0", "LICENSE"), "MIT TEXT")
	writeFile(t, filepath.Join(gomod, "github.com/a/two@v2.0.0", "LICENSE"), "MIT TEXT")
	writeFile(t, filepath.Join(gomod, "github.com/a/three@v3.0.0", "LICENSE"), "BSD TEXT")
	writeFile(t, filepath.Join(npm, "left-pad", "LICENSE"), "MIT TEXT")

	components := []Component{
		{Name: "github.com/a/one", Version: "v1.0.0", License: "MIT", Source: "go", LicenseFile: "LICENSE"},
		{Name: "github.com/a/two", Version: "v2.0.0", License: "MIT", Source: "go", LicenseFile: "LICENSE"},
		{Name: "github.com/a/three", Version: "v3.0.0", License: "BSD-3-Clause", Source: "go", LicenseFile: "LICENSE"},
		{Name: "left-pad", Version: "1.0.0", License: "MIT", Source: "npm", LicenseFile: "LICENSE"},
		{Name: "vendor blob", License: "unknown", ManualReview: true, Note: "no local file"},
	}

	groups, manual, err := buildGroups(components, gomod, npm)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(manual) != 1 || manual[0].Name != "vendor blob" {
		t.Fatalf("expected exactly one manual component, got %+v", manual)
	}
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d: %+v", len(groups), groups)
	}

	var mitGroup *Group
	for i := range groups {
		if groups[i].Text == "MIT TEXT" {
			mitGroup = &groups[i]
		}
	}
	if mitGroup == nil {
		t.Fatalf("no group found for MIT TEXT")
	}
	if len(mitGroup.Components) != 3 {
		t.Fatalf("expected 3 components sharing MIT TEXT, got %d", len(mitGroup.Components))
	}
}

func TestBuildGroupsMissingLicenseFileFails(t *testing.T) {
	gomod := t.TempDir()
	npm := t.TempDir()

	components := []Component{
		{Name: "github.com/a/missing", Version: "v1.0.0", License: "MIT", Source: "go", LicenseFile: "LICENSE"},
	}

	_, _, err := buildGroups(components, gomod, npm)
	if err == nil {
		t.Fatalf("expected error for missing license file, got nil")
	}
	if !strings.Contains(err.Error(), "github.com/a/missing") {
		t.Fatalf("error should name the missing component, got: %v", err)
	}
}

func TestSortComponentsStable(t *testing.T) {
	cs := []Component{
		{Name: "zeta", Version: "v1.0.0"},
		{Name: "alpha", Version: "v2.0.0"},
		{Name: "alpha", Version: "v1.0.0"},
	}
	sortComponents(cs)
	want := []string{"alpha v1.0.0", "alpha v2.0.0", "zeta v1.0.0"}
	for i, c := range cs {
		got := c.Name + " " + c.Version
		if got != want[i] {
			t.Fatalf("index %d: got %q, want %q", i, got, want[i])
		}
	}
}

func TestGenerateIsDeterministic(t *testing.T) {
	gomod := t.TempDir()
	npm := t.TempDir()
	manifestDir := t.TempDir()

	writeFile(t, filepath.Join(gomod, "github.com/a/one@v1.0.0", "LICENSE"), "MIT TEXT")
	writeFile(t, filepath.Join(npm, "left-pad", "LICENSE"), "MIT TEXT")

	manifestPath := filepath.Join(manifestDir, "manifest.json")
	writeFile(t, manifestPath, `{
		"components": [
			{"name": "left-pad", "version": "1.0.0", "license": "MIT", "source": "npm", "licenseFile": "LICENSE"},
			{"name": "github.com/a/one", "version": "v1.0.0", "license": "MIT", "source": "go", "licenseFile": "LICENSE"},
			{"name": "vendor blob", "license": "unknown", "manualReview": true, "note": "no local file"}
		]
	}`)

	first, err := generate(manifestPath, gomod, npm)
	if err != nil {
		t.Fatalf("first generate: %v", err)
	}
	second, err := generate(manifestPath, gomod, npm)
	if err != nil {
		t.Fatalf("second generate: %v", err)
	}
	if string(first) != string(second) {
		t.Fatalf("generate output is not deterministic:\n--- first ---\n%s\n--- second ---\n%s", first, second)
	}
	if !strings.Contains(string(first), "vendor blob") {
		t.Fatalf("expected manual review component to appear in output")
	}
	if !strings.Contains(string(first), "no local file") {
		t.Fatalf("expected manual review note to appear in output")
	}
}

func TestGenerateMissingLicenseFileFails(t *testing.T) {
	gomod := t.TempDir()
	npm := t.TempDir()
	manifestDir := t.TempDir()

	manifestPath := filepath.Join(manifestDir, "manifest.json")
	writeFile(t, manifestPath, `{
		"components": [
			{"name": "github.com/a/missing", "version": "v1.0.0", "license": "MIT", "source": "go", "licenseFile": "LICENSE"}
		]
	}`)

	if _, err := generate(manifestPath, gomod, npm); err == nil {
		t.Fatalf("expected error for missing license file, got nil")
	}
}

func TestLoadManifestRejectsInvalidComponent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.json")
	writeFile(t, path, `{"components": [{"name": "mod", "source": "go"}]}`)

	if _, err := loadManifest(path); err == nil {
		t.Fatalf("expected error for invalid component, got nil")
	}
}

func TestLoadManifestRejectsMissingFile(t *testing.T) {
	dir := t.TempDir()
	if _, err := loadManifest(filepath.Join(dir, "does-not-exist.json")); err == nil {
		t.Fatalf("expected error for missing manifest, got nil")
	}
}

func TestLoadManifestRejectsCorruptJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.json")
	writeFile(t, path, `{not valid json`)

	if _, err := loadManifest(path); err == nil {
		t.Fatalf("expected error for corrupt manifest, got nil")
	}
}
