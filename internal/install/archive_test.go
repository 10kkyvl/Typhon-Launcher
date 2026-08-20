package install

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractZipContents(t *testing.T) {
	tmp := t.TempDir()
	archive := filepath.Join(tmp, "a.zip")
	writeZip(t, archive, []zipEntry{
		{name: "top.txt", data: []byte("top")},
		{name: "sub/nested.txt", data: []byte("nested")},
		{name: "sub/deep/more.bin", data: bytes.Repeat([]byte{1}, 1024)},
	})
	dest := filepath.Join(tmp, "out")

	var last Progress
	calls := 0
	if err := ExtractArchive(context.Background(), archive, dest, func(p Progress) {
		last = p
		calls++
	}); err != nil {
		t.Fatalf("ExtractArchive: %v", err)
	}

	for name, want := range map[string]string{
		"top.txt":        "top",
		"sub/nested.txt": "nested",
	} {
		data, err := os.ReadFile(filepath.Join(dest, filepath.FromSlash(name)))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if string(data) != want {
			t.Fatalf("%s = %q, want %q", name, data, want)
		}
	}
	if info, err := os.Stat(filepath.Join(dest, "sub", "deep", "more.bin")); err != nil || info.Size() != 1024 {
		t.Fatalf("nested file stat = %v, err = %v", info, err)
	}
	if calls == 0 {
		t.Fatal("no progress reported")
	}
	if last.BytesTotal != 1033 || last.BytesDone != last.BytesTotal {
		t.Fatalf("final progress = %+v, want done==total==1033", last)
	}
}

func TestEstimateExtractedZip(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "a.zip")
	writeZip(t, archive, []zipEntry{
		{name: "a", data: bytes.Repeat([]byte{'a'}, 100)},
		{name: "b", data: bytes.Repeat([]byte{'b'}, 250)},
		{name: "dir/", data: nil},
	})
	got, err := EstimateExtracted(archive)
	if err != nil {
		t.Fatalf("EstimateExtracted: %v", err)
	}
	if got != 350 {
		t.Fatalf("estimate = %d, want 350", got)
	}
}

func TestEstimateExtractedUnsupported(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.tar.gz")
	mkFile(t, path, 10)
	if _, err := EstimateExtracted(path); !errors.Is(err, errUnsupportedArchive) {
		t.Fatalf("err = %v, want unsupported", err)
	}
}

func TestExtractArchiveUnsupportedExtension(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "a.tar.gz")
	mkFile(t, path, 10)
	err := ExtractArchive(context.Background(), path, filepath.Join(tmp, "out"), nil)
	if !errors.Is(err, errUnsupportedArchive) {
		t.Fatalf("err = %v, want unsupported", err)
	}
}

func TestExtractArchiveBrokenZip(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "a.zip")
	mkFile(t, path, 64)
	err := ExtractArchive(context.Background(), path, filepath.Join(tmp, "out"), nil)
	if !errors.Is(err, errUnsupportedArchive) {
		t.Fatalf("err = %v, want unsupported", err)
	}
}

func TestUnsupportedRarAndSevenZipVariants(t *testing.T) {
	tmp := t.TempDir()
	for _, name := range []string{"a.rar", "a.7z"} {
		path := filepath.Join(tmp, name)
		mkFile(t, path, 128)
		if err := ExtractArchive(context.Background(), path, filepath.Join(tmp, name+".out"), nil); !errors.Is(err, errUnsupportedArchive) {
			t.Fatalf("%s: err = %v, want unsupported", name, err)
		}
		if _, err := EstimateExtracted(path); err == nil {
			t.Fatalf("%s: expected estimate error", name)
		}
	}
}

func TestArchiveTypeDispatch(t *testing.T) {
	cases := map[string]Type{
		"a.ZIP":     TypeArchiveZip,
		"a.7z":      TypeArchive7z,
		"a.Rar":     TypeArchiveRar,
		"a.tar":     TypeUnknown,
		"a.zip.001": TypeUnknown,
	}
	for name, want := range cases {
		if got := archiveType(name); got != want {
			t.Fatalf("archiveType(%s) = %s, want %s", name, got, want)
		}
		if IsArchive(name) != (want != TypeUnknown) {
			t.Fatalf("IsArchive(%s) = %v", name, IsArchive(name))
		}
	}
}

func TestExtractArchivePreCancelled(t *testing.T) {
	tmp := t.TempDir()
	archive := filepath.Join(tmp, "a.zip")
	writeZip(t, archive, []zipEntry{{name: "a.txt", data: []byte("data")}})
	dest := filepath.Join(tmp, "out")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := ExtractArchive(ctx, archive, dest, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if exists(filepath.Join(dest, "a.txt")) {
		t.Fatal("file extracted despite cancellation")
	}
}

func TestExtractArchiveCancelMidway(t *testing.T) {
	tmp := t.TempDir()
	archive := filepath.Join(tmp, "a.zip")
	entries := make([]zipEntry, 0, 5)
	for _, name := range []string{"1.bin", "2.bin", "3.bin", "4.bin", "5.bin"} {
		entries = append(entries, zipEntry{name: name, data: bytes.Repeat([]byte{'z'}, 300*1024)})
	}
	writeZip(t, archive, entries)
	dest := filepath.Join(tmp, "out")

	ctx, cancel := context.WithCancel(context.Background())
	err := ExtractArchive(ctx, archive, dest, func(Progress) { cancel() })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if exists(filepath.Join(dest, "5.bin")) {
		t.Fatal("extraction did not stop early")
	}
}
