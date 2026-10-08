package install

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	rardecode "github.com/nwaples/rardecode/v2"
)

func rar4Games() []rar4Entry {
	return []rar4Entry{
		{name: "Game/a.bin", data: bytes.Repeat([]byte{'a'}, 20000)},
		{name: "Game/b.bin", data: bytes.Repeat([]byte{'b'}, 20000)},
		{name: "Game/c.bin", data: bytes.Repeat([]byte{'c'}, 20000)},
	}
}

func TestExtractArchiveRar4(t *testing.T) {
	whole := buildStoredRar4(rar4Games(), true)
	cases := []struct {
		name      string
		raw       []byte
		wantFiles int
		wantIs    []error
		wantNot   []error
		listWorks bool
	}{
		{name: "complete archive", raw: whole, wantFiles: 3},
		{name: "complete archive without the end block", raw: whole[:len(whole)-7], wantFiles: 3},
		{
			name:      "cut in the middle of the data (55%)",
			raw:       whole[:len(whole)*55/100],
			wantIs:    []error{errArchiveIncomplete, io.ErrUnexpectedEOF},
			wantNot:   []error{errUnsupportedArchive, errArchiveToolFailed},
			listWorks: true,
		},
		{
			name:      "last 10 bytes cut",
			raw:       whole[:len(whole)-10],
			wantIs:    []error{errArchiveIncomplete, io.ErrUnexpectedEOF},
			wantNot:   []error{errUnsupportedArchive, errArchiveToolFailed},
			listWorks: true,
		},
		{
			name:    "cut inside a file header",
			raw:     whole[:7+13+42+20000+20],
			wantIs:  []error{errArchiveIncomplete, io.ErrUnexpectedEOF},
			wantNot: []error{errUnsupportedArchive, errArchiveToolFailed},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			neverConsultTools(t)
			archive := filepath.Join(t.TempDir(), "game.rar")
			writeRar(t, archive, tc.raw)
			dest := filepath.Join(t.TempDir(), "out")
			err := ExtractArchive(context.Background(), archive, dest, nil)
			if tc.wantIs == nil {
				if err != nil {
					t.Fatalf("ExtractArchive: %v", err)
				}
				for _, name := range []string{"a.bin", "b.bin", "c.bin"} {
					info, statErr := os.Stat(filepath.Join(dest, "Game", name))
					if statErr != nil || info.Size() != 20000 {
						t.Fatalf("%s: %v, %v", name, info, statErr)
					}
				}
				return
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
			if _, estErr := EstimateExtracted(archive); tc.listWorks && estErr != nil {
				t.Fatalf("EstimateExtracted: %v: the listing of a truncated RAR 4 is expected to look complete", estErr)
			}
		})
	}
}

func TestDecoderFailureOnACompleteArchiveStillFallsBack(t *testing.T) {
	cases := []struct {
		name    string
		archive func(t *testing.T) string
		want    error
	}{
		{name: "RAR 5, decoded file too short", archive: func(t *testing.T) string { return storedRar(t, shortGame()) }, want: rardecode.ErrShortFile},
		{name: "RAR 5, bad checksum", archive: func(t *testing.T) string { return storedRar(t, brokenGame()) }, want: rardecode.ErrBadFileChecksum},
		{
			name: "RAR 4, decoded file too short",
			archive: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "game.rar")
				writeRar(t, path, buildStoredRar4([]rar4Entry{{name: "Game/game.exe", data: []byte("short"), declared: uint32(len(toolPayload))}}, true))
				return path
			},
			want: rardecode.ErrShortFile,
		},
		{
			name: "RAR 4, bad checksum",
			archive: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "game.rar")
				writeRar(t, path, buildStoredRar4([]rar4Entry{{name: "Game/game.exe", data: []byte(builtinPayload), badCRC: true}}, true))
				return path
			},
			want: rardecode.ErrBadFileChecksum,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			archive := tc.archive(t)
			err := decodeRar(context.Background(), archive, t.TempDir(), newReporter(nil, 0))
			var decodeErr *rarDecodeError
			if !errors.As(err, &decodeErr) || !errors.Is(err, tc.want) || errors.Is(err, errArchiveIncomplete) {
				t.Fatalf("decodeRar = %v, want a decoder failure wrapping %v", err, tc.want)
			}
			useTools(t, fakeTool(t, "ok"))
			dest := filepath.Join(t.TempDir(), "out")
			if err := ExtractArchive(context.Background(), archive, dest, nil); err != nil {
				t.Fatalf("ExtractArchive: %v", err)
			}
			got, err := os.ReadFile(filepath.Join(dest, "Game", "game.exe"))
			if err != nil || string(got) != toolPayload {
				t.Fatalf("game.exe = %q, %v: the external tool was not used", got, err)
			}
		})
	}
}
