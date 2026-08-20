package install

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestVersionFromFiles(t *testing.T) {
	cases := []struct {
		name    string
		file    string
		content string
		want    string
	}{
		{name: "plain", file: "version.txt", content: "1.2.3\n", want: "1.2.3"},
		{name: "labelled", file: "VERSION", content: "Build v2.10.44.1 (steam)", want: "2.10.44.1"},
		{name: "ini", file: "version.ini", content: "[game]\nversion=0.9\n", want: "0.9"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			mkText(t, filepath.Join(root, c.file), c.content)
			got, ok := VersionFromFiles(root)
			if !ok {
				t.Fatal("expected a version")
			}
			if got.Version != c.want {
				t.Fatalf("version = %q, want %q", got.Version, c.want)
			}
			if got.Source != "version_file" || got.Confidence != "medium" {
				t.Fatalf("info = %+v", got)
			}
		})
	}
}

func TestVersionFromFilesNotFound(t *testing.T) {
	root := t.TempDir()
	mkText(t, filepath.Join(root, "readme.txt"), "version 1.2.3 mentioned here")
	if _, ok := VersionFromFiles(root); ok {
		t.Fatal("must not read versions from unrelated files")
	}

	mkText(t, filepath.Join(root, "version.txt"), "no digits at all")
	if _, ok := VersionFromFiles(root); ok {
		t.Fatal("must not invent a version")
	}

	if _, ok := VersionFromFiles(filepath.Join(root, "missing")); ok {
		t.Fatal("missing dir must not yield a version")
	}
}

func TestExeVersionMissingFile(t *testing.T) {
	if _, ok := ExeVersion(filepath.Join(t.TempDir(), "nope.exe")); ok {
		t.Fatal("missing exe must not yield version info")
	}
}

func TestExeVersionOnSystemBinary(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("PE metadata is windows only")
	}
	path := filepath.Join(os.Getenv("SystemRoot"), "System32", "notepad.exe")
	if !exists(path) {
		t.Skip("notepad.exe unavailable")
	}
	info, ok := ExeVersion(path)
	if !ok {
		t.Fatal("expected version info for notepad.exe")
	}
	if info.Version == "" || info.Source != "pe_metadata" {
		t.Fatalf("info = %+v", info)
	}
	if info.Confidence != "high" && info.Confidence != "medium" {
		t.Fatalf("confidence = %q", info.Confidence)
	}
}
