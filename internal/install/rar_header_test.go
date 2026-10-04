package install

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"path/filepath"
	"slices"
	"testing"

	rardecode "github.com/nwaples/rardecode/v2"

	"typhon/internal/uierr"
)

const unsupportedArchiveCode = "install.unsupported_archive"

func zeroed(raw []byte, from, to int) []byte {
	out := slices.Clone(raw)
	clear(out[from:to])
	return out
}

func flipped(raw []byte, at int) []byte {
	out := slices.Clone(raw)
	out[at] ^= 0xff
	return out
}

func TestRarDamagedHeadersAreClassifiedNotUnsupported(t *testing.T) {
	big := []rarEntry{
		{name: "Game/a.bin", data: bytes.Repeat([]byte{'a'}, 150000)},
		{name: "Game/b.bin", data: bytes.Repeat([]byte{'b'}, 150000)},
	}
	small := []rarEntry{
		{name: "Game/a.bin", data: bytes.Repeat([]byte{'a'}, 2500)},
		{name: "Game/b.bin", data: bytes.Repeat([]byte{'b'}, 3000)},
	}
	const rar5EndBlock = 8
	big5 := buildStoredRar(big, rarOptions{})
	small5 := buildStoredRar(small, rarOptions{})
	bHeader5 := len(buildStoredRar(small[:1], rarOptions{})) - rar5EndBlock
	bigHeader5 := len(buildStoredRar(big[:1], rarOptions{})) - rar5EndBlock

	rar4Files := []rar4Entry{
		{name: "Game/a.bin", data: bytes.Repeat([]byte{'a'}, 2500)},
		{name: "Game/b.bin", data: bytes.Repeat([]byte{'b'}, 3000)},
	}
	small4 := buildStoredRar4(rar4Files, true)
	bHeader4 := len(buildStoredRar4(rar4Files[:1], false))

	cases := []struct {
		name      string
		raw       []byte
		wantIs    []error
		wantNot   []error
		dataFirst bool
	}{
		{
			name:      "rar5 last 20 bytes zeroed: end block is blank",
			raw:       zeroed(small5, len(small5)-20, len(small5)),
			wantIs:    []error{errArchiveIncomplete, rardecode.ErrBadBlockHeader},
			wantNot:   []error{errUnsupportedArchive, errArchiveCorrupt},
			dataFirst: true,
		},
		{
			name:      "rar5 last 100 KB zeroed",
			raw:       zeroed(big5, len(big5)-100*1024, len(big5)),
			wantIs:    []error{errArchiveIncomplete, rardecode.ErrBadBlockHeader},
			wantNot:   []error{errUnsupportedArchive, errArchiveCorrupt},
			dataFirst: true,
		},
		{
			name:      "rar5 zeroed from inside the first file to the end: second header is blank",
			raw:       zeroed(big5, bigHeader5-1000, len(big5)),
			wantIs:    []error{errArchiveIncomplete, rardecode.ErrBadBlockHeader},
			wantNot:   []error{errUnsupportedArchive, errArchiveCorrupt},
			dataFirst: true,
		},
		{
			name:    "rar5 blank header in the middle, intact tail",
			raw:     zeroed(small5, bHeader5, bHeader5+16),
			wantIs:  []error{errArchiveCorrupt, rardecode.ErrBadBlockHeader},
			wantNot: []error{errUnsupportedArchive, errArchiveIncomplete},
		},
		{
			name:    "rar5 header crc mismatch, intact tail",
			raw:     flipped(small5, bHeader5+8),
			wantIs:  []error{errArchiveCorrupt, rardecode.ErrBadHeaderCRC},
			wantNot: []error{errUnsupportedArchive, errArchiveIncomplete},
		},
		{
			name: "rar5 header size overwritten, intact tail",
			raw: func() []byte {
				raw := slices.Clone(small5)
				copy(raw[bHeader5+4:], []byte{0xff, 0xff, 0xff, 0x0f})
				return raw
			}(),
			wantIs:  []error{errArchiveCorrupt, rardecode.ErrBadBlockHeader},
			wantNot: []error{errUnsupportedArchive, errArchiveIncomplete},
		},
		{
			name:    "rar5 header crc mismatch and only four zero bytes at the end",
			raw:     zeroed(flipped(small5, bHeader5+8), len(small5)-4, len(small5)),
			wantIs:  []error{errArchiveCorrupt, rardecode.ErrBadHeaderCRC},
			wantNot: []error{errUnsupportedArchive, errArchiveIncomplete},
		},
		{
			name:    "rar4 second header and everything after it zeroed",
			raw:     zeroed(small4, bHeader4, len(small4)),
			wantIs:  []error{errArchiveIncomplete},
			wantNot: []error{errUnsupportedArchive, errArchiveCorrupt},
		},
		{
			name:    "rar4 header crc mismatch, intact tail",
			raw:     flipped(small4, bHeader4+1),
			wantIs:  []error{errArchiveCorrupt},
			wantNot: []error{errUnsupportedArchive, errArchiveIncomplete},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			neverConsultTools(t)
			archive := filepath.Join(t.TempDir(), "game.rar")
			writeRar(t, archive, tc.raw)

			check := func(t *testing.T, what string, err error) {
				t.Helper()
				if err == nil {
					t.Fatalf("%s succeeded on a damaged archive", what)
				}
				for _, want := range tc.wantIs {
					if !errors.Is(err, want) {
						t.Fatalf("%s: err = %v, want errors.Is %v", what, err, want)
					}
				}
				for _, not := range tc.wantNot {
					if errors.Is(err, not) {
						t.Fatalf("%s: err = %v, must not be %v", what, err, not)
					}
				}
				if uierr.Code(err) == unsupportedArchiveCode {
					t.Fatalf("%s: code = %q, a damaged archive is not an unsupported format", what, uierr.Code(err))
				}
			}

			_, estErr := EstimateExtracted(archive)
			check(t, "EstimateExtracted", estErr)
			extErr := ExtractArchive(context.Background(), archive, filepath.Join(t.TempDir(), "out"), nil)
			check(t, "ExtractArchive", extErr)
			if got, want := uierr.Code(estErr), uierr.Code(extErr); got != want {
				t.Fatalf("estimate code = %q, extract code = %q: the planner and the extractor disagree", got, want)
			}

			if tc.dataFirst {
				return
			}
			decErr := decodeRar(context.Background(), archive, t.TempDir(), newReporter(nil, 0))
			var decodeErr *rarDecodeError
			if errors.As(decErr, &decodeErr) {
				t.Fatalf("decodeRar: err = %v: a damaged header must not send the archive to an external tool", decErr)
			}
			check(t, "decodeRar", decErr)
		})
	}
}

func TestStoppedInZeroTail(t *testing.T) {
	tail := func(solid, zeros int) []byte {
		return append(bytes.Repeat([]byte{1}, solid), make([]byte, zeros)...)
	}
	cases := []struct {
		name    string
		content []byte
		reach   int64
		want    bool
	}{
		{name: "stopped inside a long zero tail", content: tail(100, 3<<20), reach: 100 + 4096, want: true},
		{name: "zero tail of exactly the minimum block", content: tail(100, 8), reach: 108, want: true},
		{name: "zero tail shorter than a block", content: tail(100, 7), reach: 107},
		{name: "stopped before a nonzero byte of the tail", content: append(tail(100, 20), 1), reach: 110},
		{name: "stopped in data, zeros only at the very end", content: tail(100, 20), reach: 50},
		{name: "nothing was read", content: tail(100, 20), reach: 0},
		{name: "empty file", content: nil, reach: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "v.rar")
			writeRar(t, path, tc.content)
			got, err := (&volumeEOF{name: path, reach: tc.reach}).stoppedInZeroTail()
			if err != nil || got != tc.want {
				t.Fatalf("stoppedInZeroTail = %v, %v, want %v", got, err, tc.want)
			}
		})
	}
	t.Run("no tracker", func(t *testing.T) {
		var none *volumeEOF
		if got, err := none.stoppedInZeroTail(); got || err != nil {
			t.Fatalf("stoppedInZeroTail = %v, %v, want false, nil", got, err)
		}
	})
	t.Run("volume vanished", func(t *testing.T) {
		got, err := (&volumeEOF{name: filepath.Join(t.TempDir(), "gone.rar"), reach: 10}).stoppedInZeroTail()
		if got || !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("stoppedInZeroTail = %v, %v, want an error instead of a guess", got, err)
		}
	})
}

func TestRarHeaderCauseKeepsTailCheckFailure(t *testing.T) {
	eof := &volumeEOF{name: filepath.Join(t.TempDir(), "gone.rar"), reach: 10}
	err := rarHeaderCause(rardecode.ErrBadHeaderCRC, eof)
	if !errors.Is(err, errArchiveCorrupt) || !errors.Is(err, rardecode.ErrBadHeaderCRC) || !errors.Is(err, fs.ErrNotExist) || errors.Is(err, errArchiveIncomplete) {
		t.Fatalf("err = %v, want errArchiveCorrupt carrying both the header error and the tail check failure", err)
	}
	if rarHeaderCause(rardecode.ErrBadFileChecksum, eof) != nil || rarHeaderCause(nil, eof) != nil {
		t.Fatal("only header errors may be classified here")
	}
}

func TestEOFTrackerFollowsReadsAndSeeks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v.bin")
	writeRar(t, path, make([]byte, 100))
	eof := &volumeEOF{}
	f, err := eofFS{eof: eof}.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer closeReadOnly(path, f)
	seeker, ok := f.(io.Seeker)
	if !ok {
		t.Fatal("tracked file must stay seekable: rardecode skips data with Seek")
	}
	if _, err := seeker.Seek(60, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	if n, err := f.Read(make([]byte, 10)); n != 10 || err != nil || eof.reach != 70 || eof.name != path {
		t.Fatalf("read = %d, %v, reach = %d, name = %q, want reach 70 after a seek", n, err, eof.reach, eof.name)
	}
	if _, err := seeker.Seek(5, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Read(make([]byte, 5)); err != nil || eof.reach != 70 {
		t.Fatalf("reach = %d, err = %v: reach is a high-water mark and must not go back", eof.reach, err)
	}
}
