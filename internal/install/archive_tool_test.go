package install

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	rardecode "github.com/nwaples/rardecode/v2"
)

const fakeToolEnv = "TYPHON_FAKE_ARCHIVE_TOOL"

func TestFakeArchiveTool(t *testing.T) {
	mode := os.Getenv(fakeToolEnv)
	if mode == "" {
		return
	}
	args := os.Args
	for i, a := range args {
		if a == "--" {
			args = args[i+1:]
			break
		}
	}
	dest := args[len(args)-1]
	switch mode {
	case "ok":
		target := filepath.Join(dest, "Game", "game.exe")
		//nolint:gosec // G703: the fake extractor inside the test binary writes only under the t.TempDir() destination its calling test passes; real entry names are checked by checkToolEntries (invariant 32)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			fakeToolWrite(os.Stderr, err.Error()+"\n")
			os.Exit(2)
		}
		//nolint:gosec // G703: same t.TempDir() destination as above (invariant 32)
		if err := os.WriteFile(target, []byte("from tool"), 0o644); err != nil {
			fakeToolWrite(os.Stderr, err.Error()+"\n")
			os.Exit(2)
		}
		fakeToolWrite(os.Stdout, "  10% 1 - Game\\game.exe\b\b\b\b\b\b 100%\r\nEverything is Ok\n")
		os.Exit(0)
	case "fail":
		fakeToolWrite(os.Stdout, "  40%\r")
		fakeToolWrite(os.Stderr, "ERROR: CRC Failed : Game\\game.exe\n")
		os.Exit(2)
	case "writefail":
		fakeToolWrite(os.Stdout, "Write error in the file Game\\game.exe\n")
		os.Exit(5)
	case "hang":
		fakeToolWrite(os.Stdout, "  1%\r")
		<-time.After(time.Minute)
		os.Exit(0)
	}
	os.Exit(3)
}

func fakeToolWrite(w io.Writer, s string) {
	if _, err := io.WriteString(w, s); err != nil {
		os.Exit(4)
	}
}

func fakeTool(t *testing.T, mode string) archiveTool {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(fakeToolEnv, mode)
	return archiveTool{name: "fake-" + mode, path: exe, args: func(archive, dest string) []string {
		return []string{"-test.run=^TestFakeArchiveTool$", "--", archive, dest}
	}}
}

func useTools(t *testing.T, tools ...archiveTool) {
	t.Helper()
	prev := lookupArchiveTools
	lookupArchiveTools = func() []archiveTool { return tools }
	t.Cleanup(func() { lookupArchiveTools = prev })
}

type rarEntry struct {
	name    string
	data    []byte
	badCRC  bool
	symlink bool
}

func rarVint(v uint64) []byte {
	var out []byte
	for {
		b := byte(v & 0x7f)
		v >>= 7
		if v == 0 {
			return append(out, b)
		}
		out = append(out, b|0x80)
	}
}

func rarBlock(typ, flags, dataSize uint64, body []byte) []byte {
	hdr := slices.Concat(rarVint(typ), rarVint(flags))
	if flags&0x0002 != 0 {
		hdr = append(hdr, rarVint(dataSize)...)
	}
	hdr = append(hdr, body...)
	full := append(rarVint(uint64(len(hdr))), hdr...)
	return append(binary.LittleEndian.AppendUint32(nil, crc32.ChecksumIEEE(full)), full...)
}

// writeStoredRar собирает RAR5 без сжатия: этого хватает, чтобы rardecode
// прочитал оглавление и упал на проверке контрольной суммы данных.
func writeStoredRar(t *testing.T, path string, entries []rarEntry) {
	t.Helper()
	out := []byte("Rar!\x1a\x07\x01\x00")
	out = append(out, rarBlock(1, 0, 0, rarVint(0))...)
	for _, e := range entries {
		sum := crc32.ChecksumIEEE(e.data)
		if e.badCRC {
			sum ^= 0xffffffff
		}
		hostOS, attrs := uint64(0), uint64(0x20)
		if e.symlink {
			hostOS, attrs = 1, 0o120777
		}
		body := slices.Concat(rarVint(0x0004), rarVint(uint64(len(e.data))), rarVint(attrs))
		body = binary.LittleEndian.AppendUint32(body, sum)
		body = slices.Concat(body, rarVint(0), rarVint(hostOS), rarVint(uint64(len(e.name))), []byte(e.name))
		out = append(out, rarBlock(2, 0x0002, uint64(len(e.data)), body)...)
		out = append(out, e.data...)
	}
	out = append(out, rarBlock(5, 0, 0, rarVint(0))...)
	if err := os.WriteFile(path, out, 0o644); err != nil {
		t.Fatal(err)
	}
}

func brokenRar(t *testing.T, extra ...rarEntry) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "game.rar")
	entries := append(extra, rarEntry{name: "Game/game.exe", data: []byte("builtin payload"), badCRC: true})
	writeStoredRar(t, path, entries)
	return path
}

func TestStoredRarFixtureBreaksBuiltinDecoderOnData(t *testing.T) {
	archive := brokenRar(t)
	err := decodeRar(context.Background(), archive, t.TempDir(), newReporter(nil, 0))
	var decodeErr *rarDecodeError
	if !errors.As(err, &decodeErr) || !errors.Is(err, rardecode.ErrBadFileChecksum) {
		t.Fatalf("err = %v, want rarDecodeError wrapping ErrBadFileChecksum", err)
	}
}

func TestExtractArchiveFallsBackToExternalTool(t *testing.T) {
	useTools(t, fakeTool(t, "ok"))
	archive := brokenRar(t)
	dest := filepath.Join(t.TempDir(), "out")
	var last Progress
	if err := ExtractArchive(context.Background(), archive, dest, func(p Progress) { last = p }); err != nil {
		t.Fatalf("ExtractArchive: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dest, "Game", "game.exe"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "from tool" {
		t.Fatalf("content = %q, want the external tool's output over the builtin partial file", got)
	}
	if last.BytesTotal == 0 || last.BytesDone != last.BytesTotal {
		t.Fatalf("final progress = %+v, want done == total", last)
	}
}

func TestExtractRarFallbackFailures(t *testing.T) {
	cases := []struct {
		name   string
		modes  []string
		extra  []rarEntry
		wantIs []error
		want   []string
	}{
		{name: "no tool", wantIs: []error{errArchiveToolMissing, rardecode.ErrBadFileChecksum}},
		{name: "tool fails", modes: []string{"fail"}, wantIs: []error{errArchiveToolFailed, errToolExit, rardecode.ErrBadFileChecksum}, want: []string{"CRC Failed", "код 2"}},
		{name: "unsafe name", modes: []string{"ok"}, extra: []rarEntry{{name: "../evil.txt", data: []byte("x")}}, wantIs: []error{errArchiveUnsafe, errUnsafePath, rardecode.ErrBadFileChecksum}},
		{name: "symlink entry", modes: []string{"ok"}, extra: []rarEntry{{name: "Game/link", data: []byte("target"), symlink: true}}, wantIs: []error{errArchiveUnsafe, errArchiveHasLinks, rardecode.ErrBadFileChecksum}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var tools []archiveTool
			for _, m := range tc.modes {
				tools = append(tools, fakeTool(t, m))
			}
			useTools(t, tools...)
			archive := brokenRar(t, tc.extra...)
			dest := filepath.Join(t.TempDir(), "out")
			err := ExtractArchive(context.Background(), archive, dest, nil)
			for _, want := range tc.wantIs {
				if !errors.Is(err, want) {
					t.Fatalf("err = %v, want errors.Is %v", err, want)
				}
			}
			for _, want := range tc.want {
				if err == nil || !strings.Contains(err.Error(), want) {
					t.Fatalf("err = %v, want it to mention %q", err, want)
				}
			}
			if _, statErr := os.Stat(filepath.Join(dest, "..", "evil.txt")); !errors.Is(statErr, fs.ErrNotExist) {
				t.Fatalf("entry escaped the destination: %v", statErr)
			}
		})
	}
}

func TestExtractRarTriesNextToolAfterFailure(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	failing := archiveTool{name: "missing", path: filepath.Join(t.TempDir(), "absent.exe"), args: func(a, d string) []string { return nil }}
	working := archiveTool{name: "fake-ok", path: exe, args: func(archive, dest string) []string {
		return []string{"-test.run=^TestFakeArchiveTool$", "--", archive, dest}
	}}
	t.Setenv(fakeToolEnv, "ok")
	useTools(t, failing, working)
	dest := filepath.Join(t.TempDir(), "out")
	if err := ExtractArchive(context.Background(), brokenRar(t), dest, nil); err != nil {
		t.Fatalf("ExtractArchive: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "Game", "game.exe")); err != nil {
		t.Fatal(err)
	}
}

func TestExtractRarStopsOnToolWriteError(t *testing.T) {
	writer := fakeTool(t, "writefail")
	writer.exitCauses = map[int]error{5: errToolWrite}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	next := archiveTool{name: "next", path: exe, args: func(archive, dest string) []string {
		return []string{"-test.run=^TestFakeArchiveTool$", "--", archive, dest}
	}}
	useTools(t, writer, next)
	dest := filepath.Join(t.TempDir(), "out")
	err = ExtractArchive(context.Background(), brokenRar(t), dest, nil)
	if !errors.Is(err, errToolWrite) || errors.Is(err, errArchiveToolFailed) {
		t.Fatalf("err = %v, want the write failure itself, not a generic tool failure", err)
	}
	if !strings.Contains(err.Error(), "Write error") {
		t.Fatalf("err = %v, want the tool's own message", err)
	}
	data, err := os.ReadFile(filepath.Join(dest, "Game", "game.exe"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) == "from tool" {
		t.Fatal("the next tool ran after a disk write failure")
	}
}

func TestArchiveToolArgs(t *testing.T) {
	archive, dest := filepath.Join("C:", "dl", "-game [1].rar"), filepath.Join("C:", "games", "Game (x)")
	cases := []struct {
		tool archiveTool
		want []string
	}{
		{tool: sevenZipTool("7z"), want: []string{"x", "-y", "-aoa", "-bso0", "-bsp1", "-sccUTF-8", "-o" + dest, "--", archive}},
		{tool: unrarTool("unrar"), want: []string{"x", "-y", "-o+", "-p-", "-idc", "--", archive, dest + string(filepath.Separator)}},
	}
	for _, tc := range cases {
		if got := tc.tool.args(archive, dest); !slices.Equal(got, tc.want) {
			t.Fatalf("%s args = %q, want %q", tc.tool.name, got, tc.want)
		}
	}
}

func TestArchiveToolKilledOnCancel(t *testing.T) {
	tool := fakeTool(t, "hang")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- tool.extract(ctx, "a.rar", t.TempDir(), func(int) { cancel() })
	}()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want context.Canceled", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("extract did not return after cancel: the tool process was left running")
	}
}

func TestScanToolOutput(t *testing.T) {
	cases := []struct {
		name     string
		out      string
		percents []int
		tail     string
	}{
		{name: "7-zip", out: "\b\b\b\b  45% 3 - Game\\a.dll\r   99% 9 - b\r100%\r\n", percents: []int{45, 99, 100}},
		{name: "unrar", out: "Extracting from game.rar\n\nExtracting  Game\\a.dll   12%\b\b\b\b  OK \nAll OK\n", percents: []int{12}, tail: "Extracting from game.rar OK All OK"},
		{name: "empty", out: ""},
		{name: "unterminated", out: "ERROR: Unexpected end of archive", tail: "ERROR: Unexpected end of archive"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got []int
			tail := &tailBuffer{}
			if err := scanToolOutput(strings.NewReader(tc.out), tail, func(p int) { got = append(got, p) }); err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got, tc.percents) {
				t.Fatalf("percents = %v, want %v", got, tc.percents)
			}
			if tail.text() != tc.tail {
				t.Fatalf("tail = %q, want %q", tail.text(), tc.tail)
			}
		})
	}
}

func TestAsDecodeError(t *testing.T) {
	pathErr := &fs.PathError{Op: "read", Path: "game.rar", Err: fs.ErrPermission}
	cases := []struct {
		name   string
		err    error
		decode bool
		is     error
	}{
		{name: "nil", err: nil},
		{name: "eof", err: io.EOF},
		{name: "file read", err: pathErr},
		{name: "wrapped file read", err: fmt.Errorf("next: %w", pathErr)},
		{name: "checksum", err: rardecode.ErrBadFileChecksum, decode: true},
		{name: "truncated", err: io.ErrUnexpectedEOF, decode: true},
		{name: "encrypted", err: rardecode.ErrArchivedFileEncrypted, is: errArchiveEncrypted},
		{name: "bad password", err: rardecode.ErrBadPassword, is: errArchiveEncrypted},
		{name: "next volume missing", err: rardecode.ErrMultiVolume, is: errArchiveIncomplete},
		{name: "archive cut", err: rardecode.ErrUnexpectedArcEnd, is: errArchiveIncomplete},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := asDecodeError(tc.err)
			var decodeErr *rarDecodeError
			if errors.As(got, &decodeErr) != tc.decode {
				t.Fatalf("asDecodeError(%v) = %#v, decode want %v", tc.err, got, tc.decode)
			}
			if !errors.Is(got, tc.err) {
				t.Fatalf("cause lost: %v", got)
			}
			if tc.is != nil && !errors.Is(got, tc.is) {
				t.Fatalf("asDecodeError(%v) = %v, want errors.Is %v", tc.err, got, tc.is)
			}
		})
	}
}

func TestDiskWriteErrorIsNotDecodeError(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "file")
	if err := os.WriteFile(blocker, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	src := rarSource{r: strings.NewReader("payload")}
	err := writeEntry(context.Background(), filepath.Join(blocker, "child"), 0o644, src, newReporter(nil, 0), make([]byte, 8))
	var decodeErr *rarDecodeError
	if err == nil || errors.As(err, &decodeErr) {
		t.Fatalf("err = %v, want a plain filesystem error", err)
	}
}

func TestReporterRestartAndPercent(t *testing.T) {
	var got []int64
	rep := newReporter(func(p Progress) { got = append(got, p.BytesDone) }, 200)
	rep.add(150)
	rep.restart()
	rep.setPercent(50)
	rep.setPercent(40)
	rep.setPercent(150)
	rep.flush()
	if got[len(got)-1] != 200 {
		t.Fatalf("progress = %v, want to end at total", got)
	}
	if !slices.Contains(got, 0) {
		t.Fatalf("progress = %v, want a restart to 0 before the tool runs", got)
	}
}
