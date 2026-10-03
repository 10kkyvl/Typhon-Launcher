package install

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type bitWriter struct {
	buf  []byte
	nbit int
}

func (w *bitWriter) put(v uint32, n int) {
	for i := n - 1; i >= 0; i-- {
		if w.nbit%8 == 0 {
			w.buf = append(w.buf, 0)
		}
		if (v>>uint(i))&1 == 1 {
			w.buf[len(w.buf)-1] |= 1 << uint(7-w.nbit%8)
		}
		w.nbit++
	}
}

// rar3Packed собирает поток RAR 2.9 с одним блоком таблиц Хаффмана и записью
// фильтра: filter — тело записи VM, flags — её флаговый байт. Тело короче, чем
// просит флаг, заставляет декодер читать «дальше конца» уже прочитанных данных.
func rar3Packed(filter []byte, flags byte) []byte {
	w := &bitWriter{}
	w.put(0, 1)
	w.put(0, 1)
	for i := 0; i < 20; i++ {
		switch i {
		case 0, 1, 19:
			w.put(2, 4)
		default:
			w.put(0, 4)
		}
	}
	zero := func() { w.put(0, 2) }
	one := func() { w.put(1, 2) }
	rep := func(n uint32) { w.put(2, 2); w.put(n-11, 7) }
	rep(138)
	rep(118)
	one()
	one()
	rep(138)
	for i := 0; i < 8; i++ {
		zero()
	}
	w.put(1, 1)
	w.put(uint32(flags), 8)
	for _, b := range filter {
		w.put(uint32(b), 8)
	}
	return w.buf
}

func rar3Archive(packed []byte, unpacked uint32, endBlock bool) []byte {
	out := []byte("Rar!\x1a\x07\x00")
	out = append(out, rar4Block(0x73, 0, make([]byte, 6))...)
	body := binary.LittleEndian.AppendUint32(nil, le32(len(packed)))
	body = binary.LittleEndian.AppendUint32(body, unpacked)
	body = append(body, 2)
	body = binary.LittleEndian.AppendUint32(body, 0x12345678)
	body = binary.LittleEndian.AppendUint32(body, 0)
	body = append(body, 29, 0x33)
	name := "Game/f.bin"
	body = binary.LittleEndian.AppendUint16(body, le16(len(name)))
	body = binary.LittleEndian.AppendUint32(body, 0x20)
	body = append(body, name...)
	out = append(out, rar4Block(0x74, 0x8000, body)...)
	out = append(out, packed...)
	if endBlock {
		out = append(out, rar4Block(0x7b, 0x4000, nil)...)
	}
	return out
}

type rar3Case struct {
	name     string
	filter   []byte
	flags    byte
	unexpEOF bool
}

// Первые три случая — io.ErrUnexpectedEOF из io.ReadFull над уже прочитанной
// записью фильтра (decode29.go, filters.go), на целом архиве; остальные —
// отказы декодера, у которых другая ошибка.
var rar3Cases = []rar3Case{
	{name: "filter wants 2 bytes, 1 present", filter: []byte{0xAA}, flags: 0x01, unexpEOF: true},
	{name: "filter wants 8 bytes, 3 present", filter: []byte{1, 2, 3}, flags: 0x07, unexpEOF: true},
	{name: "whole filter block present, VM code shorter than declared", filter: []byte{0x00, 0x50, 0x00, 0x00}, flags: 0x03, unexpEOF: true},
	{name: "filter wants 1 byte, 1 present", filter: []byte{0x01}},
	{name: "filter wants 2 bytes, 0 present", flags: 0x01},
}

func writeRar3(t *testing.T, c rar3Case, cut int) string {
	t.Helper()
	packed := rar3Packed(c.filter, c.flags)
	raw := rar3Archive(packed, 1000, true)
	raw = raw[:len(raw)-7-cut]
	path := filepath.Join(t.TempDir(), "game.rar")
	writeRar(t, path, raw)
	return path
}

func TestRar3DecoderFailureOnAnIntactArchiveGoesToTheTools(t *testing.T) {
	tool := fakePlanTool(t, fakePlan{Files: map[string]string{"Game/f.bin": strings.Repeat("x", 1000)}})
	for _, c := range rar3Cases {
		t.Run(c.name, func(t *testing.T) {
			archive := writeRar3(t, c, 0)
			err := decodeRar(context.Background(), archive, t.TempDir(), newReporter(nil, 0))
			var decodeErr *rarDecodeError
			if !errors.As(err, &decodeErr) || errors.Is(err, errArchiveIncomplete) {
				t.Fatalf("decodeRar = %v, want a decoder failure that is not a truncation", err)
			}
			if got := errors.Is(err, io.ErrUnexpectedEOF); got != c.unexpEOF {
				t.Fatalf("decodeRar = %v, io.ErrUnexpectedEOF = %v, want %v", err, got, c.unexpEOF)
			}

			useTools(t, tool)
			dest := filepath.Join(t.TempDir(), "out")
			if err := ExtractArchive(context.Background(), archive, dest, nil); err != nil {
				t.Fatalf("ExtractArchive: %v: the tool never ran", err)
			}
			if info, err := os.Stat(filepath.Join(dest, "Game", "f.bin")); err != nil || info.Size() != 1000 {
				t.Fatalf("f.bin = %v, %v", info, err)
			}
		})
	}
}

func TestRar3ToolDecidesWhatAnIntactArchiveWas(t *testing.T) {
	c := rar3Cases[0]
	cases := []struct {
		name   string
		tool   func(t *testing.T) archiveTool
		wantIs []error
	}{
		{
			name: "7-Zip says the archive ends early",
			tool: func(t *testing.T) archiveTool {
				return fakeAs(t, sevenZipTool, fakePlan{Stderr: "ERRORS:\nUnexpected end of archive\n", Exit: 2})
			},
			wantIs: []error{errArchiveIncomplete},
		},
		{
			name: "UnRAR confirms a checksum error",
			tool: func(t *testing.T) archiveTool {
				return fakeAs(t, fakeUnrar, fakePlan{Stdout: "Game\\f.bin - checksum error\nTotal errors: 1\n", Exit: 3})
			},
			wantIs: []error{errArchiveCorrupt},
		},
		{
			name: "the tool fails without saying why",
			tool: func(t *testing.T) archiveTool {
				return fakeAs(t, sevenZipTool, fakePlan{Stderr: "ERROR: Data Error : Game\\f.bin\n", Exit: 2})
			},
			wantIs: []error{errArchiveToolFailed, errToolExit},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			useTools(t, tc.tool(t))
			err := ExtractArchive(context.Background(), writeRar3(t, c, 0), filepath.Join(t.TempDir(), "out"), nil)
			for _, want := range tc.wantIs {
				if !errors.Is(err, want) {
					t.Fatalf("err = %v, want errors.Is %v", err, want)
				}
			}
		})
	}
	t.Run("no tool is installed", func(t *testing.T) {
		useToolSet(t, toolSet{})
		err := ExtractArchive(context.Background(), writeRar3(t, c, 0), filepath.Join(t.TempDir(), "out"), nil)
		if !errors.Is(err, errArchiveToolMissing) || errors.Is(err, errArchiveIncomplete) || !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("err = %v, want errArchiveToolMissing around the decoder's own error and no claim of truncation", err)
		}
	})
}

func TestRar3FileThatReallyEndedIsIncomplete(t *testing.T) {
	for _, c := range rar3Cases[:3] {
		for _, cut := range []int{1, 5, 20} {
			t.Run(fmt.Sprintf("%s, %d bytes cut", c.name, cut), func(t *testing.T) {
				neverConsultTools(t)
				archive := writeRar3(t, c, cut)
				err := ExtractArchive(context.Background(), archive, filepath.Join(t.TempDir(), "out"), nil)
				if !errors.Is(err, errArchiveIncomplete) || !errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, errArchiveToolMissing) {
					t.Fatalf("err = %v, want errArchiveIncomplete without a tool", err)
				}
			})
		}
	}
}

func TestEOFTrackerSeesTheEndOfTheCurrentVolumeOnly(t *testing.T) {
	dir := t.TempDir()
	first, second := filepath.Join(dir, "a.bin"), filepath.Join(dir, "b.bin")
	for _, path := range []string{first, second} {
		if err := os.WriteFile(path, []byte("12345"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	eof := &volumeEOF{}
	fsys := eofFS{eof: eof}
	closeFile := func(f fs.File) {
		if err := f.Close(); err != nil {
			t.Errorf("close: %v", err)
		}
	}
	f, err := fsys.Open(first)
	if err != nil {
		t.Fatal(err)
	}
	defer closeFile(f)
	buf := make([]byte, 16)
	if n, err := f.Read(buf); n != 5 || err != nil || eof.reached() {
		t.Fatalf("first read = %d, %v, reached = %v: a short read is not the end of the file", n, err, eof.reached())
	}
	if _, err := f.Read(buf); !errors.Is(err, io.EOF) || !eof.reached() {
		t.Fatalf("second read: %v, reached = %v, want EOF to be noticed", err, eof.reached())
	}
	next, err := fsys.Open(second)
	if err != nil || eof.reached() {
		t.Fatalf("opening the next volume: %v, reached = %v, want the flag cleared", err, eof.reached())
	}
	defer closeFile(next)
	if _, err := fsys.Open(filepath.Join(dir, "absent.bin")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing volume: %v, want ErrNotExist for the missing-volume classification", err)
	}
	var none *volumeEOF
	if none.reached() {
		t.Fatal("a nil tracker must read as not reached")
	}
}
