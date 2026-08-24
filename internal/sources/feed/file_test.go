package feed

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func writeFeedFile(t *testing.T, name, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write feed file: %v", err)
	}
	return path
}

func TestValidatePath(t *testing.T) {
	abs := filepath.Join(t.TempDir(), "feed.json")
	cases := []struct {
		name string
		in   string
		want error
	}{
		{name: "empty", in: "", want: ErrEmptyPath},
		{name: "blank", in: "   ", want: ErrEmptyPath},
		{name: "relative", in: filepath.Join("feeds", "local.json"), want: ErrRelativePath},
		{name: "absolute", in: abs, want: nil},
		{name: "trimmed", in: "  " + abs + "  ", want: nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ValidatePath(c.in)
			if !errors.Is(err, c.want) {
				t.Fatalf("error = %v, want %v", err, c.want)
			}
			if c.want == nil && got != abs {
				t.Fatalf("path = %q, want %q", got, abs)
			}
		})
	}
}

func TestReadFileSuccess(t *testing.T) {
	path := writeFeedFile(t, "feed.json", validFeedJSON)

	res, err := ReadFile(context.Background(), path)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if res.NotModified {
		t.Error("expected NotModified=false")
	}
	if len(res.Feed.Entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(res.Feed.Entries))
	}
	if res.Feed.Name != "Test" {
		t.Errorf("name = %q", res.Feed.Name)
	}
	if res.Bytes != int64(len(validFeedJSON)) {
		t.Errorf("bytes = %d, want %d", res.Bytes, len(validFeedJSON))
	}
	if res.Feed.Fingerprint == "" {
		t.Error("fingerprint is empty")
	}
}

func TestReadFileRejects(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "absent.json")
	broken := writeFeedFile(t, "broken.json", "{not json")
	empty := writeFeedFile(t, "empty.json", `{"name":"Test","version":1,"downloads":[]}`)

	cases := []struct {
		name string
		path string
		want error
	}{
		{name: "empty path", path: "", want: ErrEmptyPath},
		{name: "relative path", path: "feed.json", want: ErrRelativePath},
		{name: "missing file", path: missing, want: fs.ErrNotExist},
		{name: "directory", path: dir, want: nil},
		{name: "invalid json", path: broken, want: ErrInvalidJSON},
		{name: "no entries", path: empty, want: ErrEmptyFeed},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := ReadFile(context.Background(), c.path)
			if err == nil {
				t.Fatal("expected error")
			}
			if c.want != nil && !errors.Is(err, c.want) {
				t.Fatalf("error = %v, want %v", err, c.want)
			}
		})
	}
}

func TestReadFileRejectsTooLarge(t *testing.T) {
	path := filepath.Join(t.TempDir(), "big.json")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	truncErr := f.Truncate(MaxBytes + 1)
	if err := f.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if truncErr != nil {
		t.Fatalf("truncate: %v", truncErr)
	}

	if _, err := ReadFile(context.Background(), path); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("error = %v, want %v", err, ErrTooLarge)
	}
}

func TestReadFileHonoursCancelledContext(t *testing.T) {
	path := writeFeedFile(t, "feed.json", validFeedJSON)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := ReadFile(ctx, path); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want %v", err, context.Canceled)
	}
}

func TestReadFileFollowsSymlink(t *testing.T) {
	dir := t.TempDir()
	target := writeFeedFile(t, "feed.json", validFeedJSON)
	link := filepath.Join(dir, "link.json")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	res, err := ReadFile(context.Background(), link)
	if err != nil {
		t.Fatalf("ReadFile through symlink: %v", err)
	}
	if len(res.Feed.Entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(res.Feed.Entries))
	}
}
