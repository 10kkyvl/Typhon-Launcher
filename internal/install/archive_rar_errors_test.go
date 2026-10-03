package install

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	rardecode "github.com/nwaples/rardecode/v2"

	"typhon/internal/uierr"
)

func neverConsultTools(t *testing.T) {
	t.Helper()
	prev := lookupArchiveTools
	lookupArchiveTools = func(context.Context) toolSet {
		t.Error("external tools were consulted for a failure they cannot fix")
		return toolSet{}
	}
	t.Cleanup(func() { lookupArchiveTools = prev })
}

func TestExtractArchiveRarFailureCauses(t *testing.T) {
	files := []rarEntry{
		{name: "Game/a.bin", data: bytes.Repeat([]byte{'a'}, 2500)},
		{name: "Game/b.bin", data: bytes.Repeat([]byte{'b'}, 3000)},
	}
	whole := buildStoredRar(files, rarOptions{})
	cut := func(n int) func(*testing.T, string) {
		return func(t *testing.T, path string) { writeRar(t, path, whole[:len(whole)-n]) }
	}
	cases := []struct {
		name     string
		prepare  func(t *testing.T, path string)
		wantIs   []error
		wantNot  []error
		listable bool
	}{
		{name: "truncated by 20 bytes", prepare: cut(20), wantIs: []error{errArchiveIncomplete, io.ErrUnexpectedEOF}, wantNot: []error{errUnsupportedArchive}},
		{name: "truncated by 3000 bytes", prepare: cut(3000), wantIs: []error{errArchiveIncomplete, io.ErrUnexpectedEOF}, wantNot: []error{errUnsupportedArchive}},
		{name: "truncated by 4800 bytes", prepare: cut(4800), wantIs: []error{errArchiveIncomplete, io.ErrUnexpectedEOF}, wantNot: []error{errUnsupportedArchive}},
		{
			name: "next volume is missing",
			prepare: func(t *testing.T, path string) {
				writeRar(t, path, buildStoredRar(files, rarOptions{multiVolume: true}))
			},
			wantIs:  []error{errArchiveIncomplete, fs.ErrNotExist},
			wantNot: []error{errUnsupportedArchive},
		},
		{
			name: "encrypted headers",
			prepare: func(t *testing.T, path string) {
				writeRar(t, path, buildStoredRar(files, rarOptions{encryptedHeaders: true}))
			},
			wantIs:  []error{errArchiveEncrypted, rardecode.ErrArchiveEncrypted},
			wantNot: []error{errUnsupportedArchive},
		},
		{
			name: "encrypted file data",
			prepare: func(t *testing.T, path string) {
				writeStoredRar(t, path, []rarEntry{{name: "Game/a.bin", data: bytes.Repeat([]byte{'a'}, 64), encrypted: true}})
			},
			wantIs:   []error{errArchiveEncrypted, rardecode.ErrArchivedFileEncrypted},
			wantNot:  []error{errUnsupportedArchive},
			listable: true,
		},
		{name: "archive does not exist", prepare: func(*testing.T, string) {}, wantIs: []error{fs.ErrNotExist}, wantNot: []error{errUnsupportedArchive, errArchiveIncomplete}},
		{
			name: "archive is not readable",
			prepare: func(t *testing.T, path string) {
				writeRar(t, path, whole)
				makeUnreadable(t, path)
			},
			wantIs:  []error{fs.ErrPermission},
			wantNot: []error{errUnsupportedArchive, errArchiveIncomplete},
		},
		{name: "not a rar at all", prepare: func(t *testing.T, path string) { writeRar(t, path, make([]byte, 128)) }, wantIs: []error{errUnsupportedArchive}, wantNot: []error{errArchiveIncomplete, errArchiveEncrypted}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			neverConsultTools(t)
			archive := filepath.Join(t.TempDir(), "game.rar")
			tc.prepare(t, archive)
			dest := filepath.Join(t.TempDir(), "out")
			err := ExtractArchive(context.Background(), archive, dest, nil)
			if err == nil {
				t.Fatal("ExtractArchive succeeded on a damaged archive")
			}
			for _, want := range tc.wantIs {
				if !errors.Is(err, want) {
					t.Fatalf("err = %v, want errors.Is %v", err, want)
				}
			}
			for _, not := range tc.wantNot {
				if errors.Is(err, not) {
					t.Fatalf("err = %v, must not be %v", err, not)
				}
			}
			_, estErr := EstimateExtracted(archive)
			if tc.listable {
				if estErr != nil {
					t.Fatalf("EstimateExtracted: %v", estErr)
				}
				return
			}
			if estErr == nil {
				t.Fatal("EstimateExtracted succeeded on a damaged archive")
			}
			if got, want := uierr.Code(estErr), uierr.Code(err); got != want {
				t.Fatalf("estimate code = %q, extract code = %q: the planner and the extractor disagree", got, want)
			}
		})
	}
}

func TestExtractArchiveRarUnknownFileErrorKeepsCause(t *testing.T) {
	neverConsultTools(t)
	archive := filepath.Join(t.TempDir(), "game.rar")
	if err := os.Mkdir(archive, 0o755); err != nil {
		t.Fatal(err)
	}
	err := ExtractArchive(context.Background(), archive, filepath.Join(t.TempDir(), "out"), nil)
	var pathErr *fs.PathError
	if !errors.Is(err, errUnsupportedArchive) || !errors.As(err, &pathErr) {
		t.Fatalf("err = %v, want errUnsupportedArchive that still wraps the *fs.PathError", err)
	}
}

func TestDecodeRarClassifiesOpenFailure(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "gone.rar")
	err := decodeRar(context.Background(), missing, t.TempDir(), newReporter(nil, 0))
	if !errors.Is(err, fs.ErrNotExist) || errors.Is(err, errUnsupportedArchive) {
		t.Fatalf("err = %v, want the read failure itself, not errUnsupportedArchive", err)
	}
	var decodeErr *rarDecodeError
	if errors.As(err, &decodeErr) {
		t.Fatalf("err = %v: an unreadable archive must not start the external tool search", err)
	}
}
