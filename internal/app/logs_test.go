package app

import (
	"archive/zip"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"typhon/internal/platform"
)

func readBundle(t *testing.T, path string) map[string]string {
	t.Helper()
	r, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("open bundle: %v", err)
	}
	defer func() {
		if err := r.Close(); err != nil {
			t.Fatalf("close bundle: %v", err)
		}
	}()
	out := make(map[string]string, len(r.File))
	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open entry %s: %v", f.Name, err)
		}
		data, err := io.ReadAll(rc)
		if err != nil {
			t.Fatalf("read entry %s: %v", f.Name, err)
		}
		if err := rc.Close(); err != nil {
			t.Fatalf("close entry %s: %v", f.Name, err)
		}
		out[f.Name] = string(data)
	}
	return out
}

func TestWriteLogBundleCollectsLogs(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(t.TempDir(), "typhon-logs.zip")
	for name, body := range map[string]string{
		logFileName:          "current run\n",
		logFileName + ".old": "previous run\n",
		"settings.json":      `{"secret":"keep out"}`,
		"typhon.logbook":     "not a log",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}

	bundle, err := writeLogBundle(dir, out, "report body")
	if err != nil {
		t.Fatalf("writeLogBundle: %v", err)
	}
	if bundle.Path != out || bundle.Name != "typhon-logs.zip" || bundle.Dir != filepath.Dir(out) {
		t.Fatalf("bundle = %+v, want path %s", bundle, out)
	}
	info, err := os.Stat(out)
	if err != nil {
		t.Fatalf("stat bundle: %v", err)
	}
	if bundle.SizeBytes != info.Size() {
		t.Fatalf("SizeBytes = %d, file is %d", bundle.SizeBytes, info.Size())
	}

	entries := readBundle(t, out)
	want := map[string]string{
		"info.txt":           "report body",
		logFileName:          "current run\n",
		logFileName + ".old": "previous run\n",
	}
	if len(entries) != len(want) {
		t.Fatalf("bundle entries = %v, want exactly %v", entries, want)
	}
	for name, body := range want {
		if entries[name] != body {
			t.Fatalf("entry %s = %q, want %q", name, entries[name], body)
		}
	}
}

func TestWriteLogBundleKeepsTailOfLargeLog(t *testing.T) {
	maxExportedLogBytes = 8
	t.Cleanup(func() { maxExportedLogBytes = 16 << 20 })

	dir := t.TempDir()
	out := filepath.Join(t.TempDir(), "logs.zip")
	if err := os.WriteFile(filepath.Join(dir, logFileName), []byte("head-part-tail-part"), 0o600); err != nil {
		t.Fatalf("seed log: %v", err)
	}

	if _, err := writeLogBundle(dir, out, "report"); err != nil {
		t.Fatalf("writeLogBundle: %v", err)
	}
	if got := readBundle(t, out)[logFileName]; got != "ail-part" {
		t.Fatalf("log entry = %q, want the last 8 bytes", got)
	}
}

func TestWriteLogBundleErrors(t *testing.T) {
	empty := t.TempDir()
	withLog := t.TempDir()
	if err := os.WriteFile(filepath.Join(withLog, logFileName), []byte("line\n"), 0o600); err != nil {
		t.Fatalf("seed log: %v", err)
	}

	tests := []struct {
		name string
		dir  string
		path string
		want error
	}{
		{"no source dir", "", filepath.Join(t.TempDir(), "logs.zip"), platform.ErrEmptyPath},
		{"no destination", withLog, "", platform.ErrEmptyPath},
		{"source dir missing", filepath.Join(empty, "gone"), filepath.Join(t.TempDir(), "logs.zip"), fs.ErrNotExist},
		{"no log files", empty, filepath.Join(t.TempDir(), "logs.zip"), ErrNoLogs},
		{"destination dir missing", withLog, filepath.Join(t.TempDir(), "gone", "logs.zip"), fs.ErrNotExist},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bundle, err := writeLogBundle(tt.dir, tt.path, "report")
			if !errors.Is(err, tt.want) {
				t.Fatalf("writeLogBundle error = %v, want %v", err, tt.want)
			}
			if bundle != (LogBundle{}) {
				t.Fatalf("bundle = %+v, want zero value on error", bundle)
			}
			if tt.path != "" {
				if _, err := os.Stat(tt.path); !errors.Is(err, fs.ErrNotExist) {
					t.Fatalf("stat %s = %v, want the bundle not to exist", tt.path, err)
				}
			}
		})
	}
}

func TestLogReportNamesTheDataDir(t *testing.T) {
	dir := t.TempDir()
	report := logReport(dir)
	for _, want := range []string{"Typhon " + Version, dir} {
		if !strings.Contains(report, want) {
			t.Fatalf("report %q does not mention %q", report, want)
		}
	}
}
