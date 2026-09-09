package app

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
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

// fillDeterministic writes bytes that gzip cannot meaningfully shrink,
// without math/rand: gosec (G404) flags math/rand even in tests, and the
// project already avoids it for exactly this reason (see
// internal/catalog/resolve_bench_test.go). A hand-rolled xorshift32 is
// deterministic and good enough to defeat compression for a size assertion.
func fillDeterministic(seed uint32, data []byte) {
	state := seed | 1
	for i := range data {
		state ^= state << 13
		state ^= state >> 17
		state ^= state << 5
		data[i] = byte(state & 0xff)
	}
}

func seedLog(t *testing.T, dir, name string, size int, seed uint32) {
	t.Helper()
	data := make([]byte, size)
	fillDeterministic(seed, data)
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
		t.Fatalf("seed %s: %v", name, err)
	}
}

func gunzipBytes(t *testing.T, data []byte) []byte {
	t.Helper()
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("gzip.NewReader: %v", err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read gzip: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("close gzip reader: %v", err)
	}
	return out
}

func readBundleBytes(t *testing.T, data []byte) map[string]string {
	t.Helper()
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("open bundle: %v", err)
	}
	out := make(map[string]string, len(r.File))
	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open entry %s: %v", f.Name, err)
		}
		body, err := io.ReadAll(rc)
		if err != nil {
			t.Fatalf("read entry %s: %v", f.Name, err)
		}
		if err := rc.Close(); err != nil {
			t.Fatalf("close entry %s: %v", f.Name, err)
		}
		out[f.Name] = string(body)
	}
	return out
}

func TestBuildUploadBundleWithinLimitKeepsEverything(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, logFileName), []byte("current\n"), 0o600); err != nil {
		t.Fatalf("seed current log: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, logFileName+".1"), []byte("older\n"), 0o600); err != nil {
		t.Fatalf("seed backup log: %v", err)
	}

	data, dropped, err := buildUploadBundle(dir, "report", 1<<20)
	if err != nil {
		t.Fatalf("buildUploadBundle: %v", err)
	}
	if dropped != nil {
		t.Fatalf("dropped = %v, want none", dropped)
	}
	entries := readBundleBytes(t, gunzipBytes(t, data))
	want := map[string]string{"info.txt": "report", logFileName: "current\n", logFileName + ".1": "older\n"}
	if len(entries) != len(want) {
		t.Fatalf("entries = %v, want %v", entries, want)
	}
	for name, body := range want {
		if entries[name] != body {
			t.Fatalf("entry %s = %q, want %q", name, entries[name], body)
		}
	}
}

func TestBuildUploadBundleDropsOldestRotationToFit(t *testing.T) {
	dir := t.TempDir()
	seedLog(t, dir, logFileName, 4096, 1)
	seedLog(t, dir, logFileName+".1", 4096, 2)
	seedLog(t, dir, logFileName+".2", 4096, 3)

	report := "report"
	full, err := buildArchive(dir, report, []string{logFileName, logFileName + ".1", logFileName + ".2"})
	if err != nil {
		t.Fatalf("buildArchive full: %v", err)
	}
	fullGz, err := gzipBytes(full)
	if err != nil {
		t.Fatalf("gzipBytes full: %v", err)
	}
	twoFiles, err := buildArchive(dir, report, []string{logFileName, logFileName + ".1"})
	if err != nil {
		t.Fatalf("buildArchive two: %v", err)
	}
	twoGz, err := gzipBytes(twoFiles)
	if err != nil {
		t.Fatalf("gzipBytes two: %v", err)
	}
	if len(fullGz) <= len(twoGz) {
		t.Fatalf("test setup invalid: dropping a file did not shrink the gzip size (%d vs %d)", len(fullGz), len(twoGz))
	}

	data, dropped, err := buildUploadBundle(dir, report, int64(len(twoGz)))
	if err != nil {
		t.Fatalf("buildUploadBundle: %v", err)
	}
	if len(dropped) != 1 || dropped[0] != logFileName+".2" {
		t.Fatalf("dropped = %v, want [%s]", dropped, logFileName+".2")
	}
	entries := readBundleBytes(t, gunzipBytes(t, data))
	if _, ok := entries[logFileName+".2"]; ok {
		t.Fatal("dropped rotation is still present in the bundle")
	}
	if _, ok := entries[logFileName+".1"]; !ok {
		t.Fatal("kept rotation is missing from the bundle")
	}
	if _, ok := entries[logFileName]; !ok {
		t.Fatal("current log is missing from the bundle")
	}
}

func TestBuildUploadBundleNeverDropsTheCurrentLog(t *testing.T) {
	dir := t.TempDir()
	seedLog(t, dir, logFileName, 4096, 4)
	seedLog(t, dir, logFileName+".1", 4096, 5)

	data, dropped, err := buildUploadBundle(dir, "report", 1)
	if err != nil {
		t.Fatalf("buildUploadBundle: %v", err)
	}
	if len(dropped) != 1 || dropped[0] != logFileName+".1" {
		t.Fatalf("dropped = %v, want [%s]", dropped, logFileName+".1")
	}
	entries := readBundleBytes(t, gunzipBytes(t, data))
	if _, ok := entries[logFileName]; !ok {
		t.Fatal("current log must never be dropped, even when it alone still exceeds the cap")
	}
}

func TestBuildUploadBundleNoLogs(t *testing.T) {
	dir := t.TempDir()
	if _, _, err := buildUploadBundle(dir, "report", 1<<20); !errors.Is(err, ErrNoLogs) {
		t.Fatalf("buildUploadBundle error = %v, want %v", err, ErrNoLogs)
	}
}

// The data directory is under the OS account name on every platform, so the
// report that ships inside the bundle names the directory only in scrubbed
// form: the shape stays readable, the account name does not travel.
func TestLogReportKeepsTheDataDirOutOfTheBundle(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, logFileName), []byte("started\n"), 0o600); err != nil {
		t.Fatalf("seed log: %v", err)
	}
	data, _, err := buildUploadBundle(dir, logReport(dir), 1<<20)
	if err != nil {
		t.Fatalf("buildUploadBundle: %v", err)
	}
	report := readBundleBytes(t, gunzipBytes(t, data))["info.txt"]
	if !strings.Contains(report, "Typhon "+Version) {
		t.Fatalf("report %q does not mention the version", report)
	}
	if strings.Contains(report, dir) {
		t.Fatalf("report %q carries the raw data directory %q", report, dir)
	}
}

// Both exits from the config directory -- the zip written to Downloads and
// the gzip sent to the server -- carry the same scrubbed text, so what a user
// can read before sending is what a send contains.
func TestLogBundlesScrubLogText(t *testing.T) {
	const line = "open C:\\Users\\Egor\\AppData\\Typhon\\state.json: access denied\n" +
		"refresh accessToken=eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.dozjgNryP4J3jVm failed\n"
	leaks := []string{"Egor", "AppData", "eyJhbGciOiJIUzI1NiJ9", "dozjgNryP4J3jVm"}

	t.Run("upload", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, logFileName), []byte(line), 0o600); err != nil {
			t.Fatalf("seed log: %v", err)
		}
		data, _, err := buildUploadBundle(dir, "report", 1<<20)
		if err != nil {
			t.Fatalf("buildUploadBundle: %v", err)
		}
		assertScrubbed(t, readBundleBytes(t, gunzipBytes(t, data))[logFileName], leaks)
	})

	t.Run("export", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, logFileName), []byte(line), 0o600); err != nil {
			t.Fatalf("seed log: %v", err)
		}
		out := filepath.Join(t.TempDir(), "bundle.zip")
		if _, err := writeLogBundle(dir, out, "report"); err != nil {
			t.Fatalf("writeLogBundle: %v", err)
		}
		assertScrubbed(t, readBundle(t, out)[logFileName], leaks)
	})
}

func assertScrubbed(t *testing.T, body string, leaks []string) {
	t.Helper()
	for _, leak := range leaks {
		if strings.Contains(body, leak) {
			t.Fatalf("bundled log leaked %q:\n%s", leak, body)
		}
	}
	if !strings.Contains(body, "access denied") {
		t.Fatalf("bundled log dropped the diagnostic text:\n%s", body)
	}
}
