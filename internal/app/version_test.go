package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return filepath.Join(wd, "..", "..")
}

func readVersionFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	v := strings.TrimSpace(string(data))
	if v == "" {
		return "", fmt.Errorf("%s is empty", path)
	}
	return v, nil
}

func readYAMLNestedValue(path, section, key string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	sectionHeader := section + ":"
	keyPrefix := key + ":"
	inSection := false
	for _, raw := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indented := strings.HasPrefix(raw, " ") || strings.HasPrefix(raw, "\t")
		if !indented {
			inSection = trimmed == sectionHeader || strings.HasPrefix(trimmed, sectionHeader+" ")
			continue
		}
		if !inSection || !strings.HasPrefix(trimmed, keyPrefix) {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(trimmed, keyPrefix))
		value = strings.Trim(value, `"'`)
		if value == "" {
			return "", fmt.Errorf("%s: %s.%s is empty", path, section, key)
		}
		return value, nil
	}
	return "", fmt.Errorf("%s: %s.%s not found", path, section, key)
}

type windowsInfoJSON struct {
	Fixed struct {
		FileVersion string `json:"file_version"`
	} `json:"fixed"`
	Info map[string]struct {
		ProductVersion string `json:"ProductVersion"`
	} `json:"info"`
}

func readInfoJSONVersions(path string) (fileVersion, productVersion string, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", fmt.Errorf("read %s: %w", path, err)
	}
	var v windowsInfoJSON
	if err := json.Unmarshal(data, &v); err != nil {
		return "", "", fmt.Errorf("parse %s: %w", path, err)
	}
	entry, ok := v.Info["0000"]
	if !ok {
		return "", "", fmt.Errorf("%s: info.0000 entry not found", path)
	}
	if v.Fixed.FileVersion == "" {
		return "", "", fmt.Errorf("%s: fixed.file_version is empty", path)
	}
	if entry.ProductVersion == "" {
		return "", "", fmt.Errorf("%s: info.0000.ProductVersion is empty", path)
	}
	return v.Fixed.FileVersion, entry.ProductVersion, nil
}

func coreVersion(v string, parts int) (string, error) {
	segs := strings.Split(strings.TrimSpace(v), ".")
	if len(segs) < parts {
		return "", fmt.Errorf("version %q has fewer than %d dot-separated parts", v, parts)
	}
	for _, s := range segs {
		if s == "" {
			return "", fmt.Errorf("version %q has an empty component", v)
		}
	}
	return strings.Join(segs[:parts], "."), nil
}

func TestCoreVersion(t *testing.T) {
	tests := []struct {
		name    string
		version string
		parts   int
		want    string
		wantErr bool
	}{
		{"exact three parts", "0.1.0", 3, "0.1.0", false},
		{"four parts trims build number", "0.1.0.0", 3, "0.1.0", false},
		{"four parts with nonzero build kept out of comparison window", "0.1.0.7", 3, "0.1.0", false},
		{"too few parts", "0.1", 3, "", true},
		{"empty input", "", 3, "", true},
		{"empty component", "0..0", 3, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := coreVersion(tt.version, tt.parts)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("coreVersion(%q, %d) = %q, want error", tt.version, tt.parts, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("coreVersion(%q, %d) unexpected error: %v", tt.version, tt.parts, err)
			}
			if got != tt.want {
				t.Fatalf("coreVersion(%q, %d) = %q, want %q", tt.version, tt.parts, got, tt.want)
			}
		})
	}
}

func TestReadVersionFile(t *testing.T) {
	tests := []struct {
		name    string
		content string
		missing bool
		want    string
		wantErr bool
	}{
		{"trims trailing newline", "0.1.0\n", false, "0.1.0", false},
		{"no trailing newline", "0.1.0", false, "0.1.0", false},
		{"empty file", "", false, "", true},
		{"missing file", "", true, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "VERSION")
			if !tt.missing {
				if err := os.WriteFile(path, []byte(tt.content), 0o644); err != nil {
					t.Fatalf("write fixture: %v", err)
				}
			}
			got, err := readVersionFile(path)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("readVersionFile(%q) = %q, want error", path, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("readVersionFile(%q) unexpected error: %v", path, err)
			}
			if got != tt.want {
				t.Fatalf("readVersionFile(%q) = %q, want %q", path, got, tt.want)
			}
		})
	}
}

func TestReadYAMLNestedValue(t *testing.T) {
	valid := "version: '3'\ninfo:\n  companyName: \"Typhon\"\n  version: \"0.1.0\"\n  comments: \"\"\ndev_mode:\n  version: \"9.9.9\"\n"
	tests := []struct {
		name    string
		content string
		missing bool
		want    string
		wantErr bool
	}{
		{"finds nested key inside section", valid, false, "0.1.0", false},
		{"missing section", "other:\n  version: \"1.2.3\"\n", false, "", true},
		{"missing key in section", "info:\n  companyName: \"Typhon\"\n", false, "", true},
		{"missing file", "", true, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "config.yml")
			if !tt.missing {
				if err := os.WriteFile(path, []byte(tt.content), 0o644); err != nil {
					t.Fatalf("write fixture: %v", err)
				}
			}
			got, err := readYAMLNestedValue(path, "info", "version")
			if tt.wantErr {
				if err == nil {
					t.Fatalf("readYAMLNestedValue(%q) = %q, want error", path, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("readYAMLNestedValue(%q) unexpected error: %v", path, err)
			}
			if got != tt.want {
				t.Fatalf("readYAMLNestedValue(%q) = %q, want %q", path, got, tt.want)
			}
		})
	}
}

func TestReadInfoJSONVersions(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		missing     bool
		wantFile    string
		wantProduct string
		wantErr     bool
	}{
		{
			name:        "three part versions",
			content:     `{"fixed":{"file_version":"0.1.0"},"info":{"0000":{"ProductVersion":"0.1.0"}}}`,
			wantFile:    "0.1.0",
			wantProduct: "0.1.0",
		},
		{
			name:        "four part file_version",
			content:     `{"fixed":{"file_version":"0.1.0.0"},"info":{"0000":{"ProductVersion":"0.1.0.0"}}}`,
			wantFile:    "0.1.0.0",
			wantProduct: "0.1.0.0",
		},
		{"malformed json", `{"fixed":`, false, "", "", true},
		{"missing 0000 entry", `{"fixed":{"file_version":"0.1.0"},"info":{}}`, false, "", "", true},
		{"empty file_version", `{"fixed":{"file_version":""},"info":{"0000":{"ProductVersion":"0.1.0"}}}`, false, "", "", true},
		{"missing file", "", true, "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "info.json")
			if !tt.missing {
				if err := os.WriteFile(path, []byte(tt.content), 0o644); err != nil {
					t.Fatalf("write fixture: %v", err)
				}
			}
			gotFile, gotProduct, err := readInfoJSONVersions(path)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("readInfoJSONVersions(%q) = (%q, %q), want error", path, gotFile, gotProduct)
				}
				return
			}
			if err != nil {
				t.Fatalf("readInfoJSONVersions(%q) unexpected error: %v", path, err)
			}
			if gotFile != tt.wantFile || gotProduct != tt.wantProduct {
				t.Fatalf("readInfoJSONVersions(%q) = (%q, %q), want (%q, %q)", path, gotFile, gotProduct, tt.wantFile, tt.wantProduct)
			}
		})
	}
}

func TestVersionSourcesMatch(t *testing.T) {
	root := repoRoot(t)

	fileVersion, err := readVersionFile(filepath.Join(root, "VERSION"))
	if err != nil {
		t.Fatalf("read VERSION: %v", err)
	}

	if fileVersion != Version {
		t.Fatalf("VERSION file = %q, app.Version default = %q, want equal", fileVersion, Version)
	}

	configVersion, err := readYAMLNestedValue(filepath.Join(root, "build", "config.yml"), "info", "version")
	if err != nil {
		t.Fatalf("read build/config.yml: %v", err)
	}

	winFileVersion, winProductVersion, err := readInfoJSONVersions(filepath.Join(root, "build", "windows", "info.json"))
	if err != nil {
		t.Fatalf("read build/windows/info.json: %v", err)
	}

	cases := []struct {
		name string
		got  string
	}{
		{"build/config.yml info.version", configVersion},
		{"build/windows/info.json fixed.file_version", winFileVersion},
		{"build/windows/info.json info.0000.ProductVersion", winProductVersion},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			core, err := coreVersion(tc.got, 3)
			if err != nil {
				t.Fatalf("%s = %q: %v", tc.name, tc.got, err)
			}
			if core != fileVersion {
				t.Fatalf("%s = %q (core %q), want core version %q (from VERSION file)", tc.name, tc.got, core, fileVersion)
			}
		})
	}
}
