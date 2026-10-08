package install

import (
	"context"
	"encoding/binary"
	"encoding/json"
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

const (
	fakeToolEnv    = "TYPHON_FAKE_ARCHIVE_TOOL"
	toolPayload    = "from tool"
	builtinPayload = "builtin!!"
)

type fakePlan struct {
	Files     map[string]string `json:"files"`
	HardLinks map[string]string `json:"hardLinks"`
	DirLinks  map[string]string `json:"dirLinks"`
	Stdout    string            `json:"stdout"`
	Stderr    string            `json:"stderr"`
	Exit      int               `json:"exit"`
}

func TestFakeArchiveTool(t *testing.T) {
	if os.Getenv(fakeToolEnv) == "" {
		return
	}
	args := os.Args
	for i, a := range args {
		if a == "--" {
			args = args[i+1:]
			break
		}
	}
	if len(args) < 4 {
		os.Exit(3)
	}
	mode, planJSON, dest := args[0], args[1], args[len(args)-1]
	switch mode {
	case "ok":
		plan := fakePlan{Files: map[string]string{"Game/game.exe": toolPayload}}
		if err := runFakePlan(dest, plan); err != nil {
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
	case "plan":
		var plan fakePlan
		if err := json.Unmarshal([]byte(planJSON), &plan); err != nil {
			fakeToolWrite(os.Stderr, err.Error()+"\n")
			os.Exit(4)
		}
		if err := runFakePlan(dest, plan); err != nil {
			fakeToolWrite(os.Stderr, err.Error()+"\n")
			os.Exit(4)
		}
		fakeToolWrite(os.Stdout, plan.Stdout)
		fakeToolWrite(os.Stderr, plan.Stderr)
		os.Exit(plan.Exit)
	}
	os.Exit(3)
}

func runFakePlan(dest string, plan fakePlan) error {
	for name, content := range plan.Files {
		target := filepath.Join(dest, filepath.FromSlash(name))
		//nolint:gosec // G703: the fake extractor inside the test binary writes only under the t.TempDir() destination its calling test passes (invariant 32)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		//nolint:gosec // G703: same t.TempDir() destination as above (invariant 32)
		if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
			return err
		}
	}
	for link, existing := range plan.HardLinks {
		target := filepath.Join(dest, filepath.FromSlash(link))
		//nolint:gosec // G703: same t.TempDir() destination as above (invariant 32)
		if err := os.Remove(target); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		//nolint:gosec // G703: same t.TempDir() destination as above (invariant 32)
		if err := os.Link(filepath.Join(dest, filepath.FromSlash(existing)), target); err != nil {
			return err
		}
	}
	for link, dir := range plan.DirLinks {
		target := filepath.Join(dest, filepath.FromSlash(link))
		//nolint:gosec // G703: same t.TempDir() destination as above (invariant 32)
		if err := os.Remove(target); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		if err := makeDirLink(target, dir); err != nil {
			return err
		}
	}
	return nil
}

func fakeToolWrite(w io.Writer, s string) {
	if _, err := io.WriteString(w, s); err != nil {
		os.Exit(4)
	}
}

func fakeToolArgs(mode, planJSON string) func(archive, dest string) []string {
	return func(archive, dest string) []string {
		return []string{"-test.run=^TestFakeArchiveTool$", "--", mode, planJSON, archive, dest}
	}
}

func fakeTool(t *testing.T, mode string) archiveTool {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(fakeToolEnv, "1")
	return archiveTool{name: "fake-" + mode, path: exe, args: fakeToolArgs(mode, "")}
}

func fakePlanTool(t *testing.T, plan fakePlan) archiveTool {
	t.Helper()
	encoded, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	tool := fakeTool(t, "plan")
	tool.args = fakeToolArgs("plan", string(encoded))
	return tool
}

func fakeAs(t *testing.T, tool func(path string) archiveTool, plan fakePlan) archiveTool {
	t.Helper()
	fake := fakePlanTool(t, plan)
	real := tool(fake.path)
	real.args = fake.args
	return real
}

func fakeUnrar(path string) archiveTool {
	return unrarTool(path, true)
}

func useTools(t *testing.T, tools ...archiveTool) {
	t.Helper()
	prev := lookupArchiveTools
	lookupArchiveTools = func(context.Context) toolSet { return toolSet{tools: tools} }
	t.Cleanup(func() { lookupArchiveTools = prev })
}

type rarEntry struct {
	name      string
	data      []byte
	badCRC    bool
	symlink   bool
	reparse   bool
	dir       bool
	encrypted bool
	declared  uint64
}

type rarOptions struct {
	multiVolume      bool
	encryptedHeaders bool
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

func rarBlockExtra(typ, flags, dataSize uint64, body, extra []byte) []byte {
	if len(extra) > 0 {
		flags |= 0x0001
	}
	hdr := slices.Concat(rarVint(typ), rarVint(flags))
	if len(extra) > 0 {
		hdr = append(hdr, rarVint(uint64(len(extra)))...)
	}
	if flags&0x0002 != 0 {
		hdr = append(hdr, rarVint(dataSize)...)
	}
	hdr = append(hdr, body...)
	hdr = append(hdr, extra...)
	full := append(rarVint(uint64(len(hdr))), hdr...)
	return append(binary.LittleEndian.AppendUint32(nil, crc32.ChecksumIEEE(full)), full...)
}

func rarBlock(typ, flags, dataSize uint64, body []byte) []byte {
	return rarBlockExtra(typ, flags, dataSize, body, nil)
}

func rarFileBlock(e rarEntry) []byte {
	fileFlags, hostOS, attrs := uint64(0x0004), uint64(0), uint64(0x20)
	switch {
	case e.dir:
		fileFlags, attrs = 0x0001, 0x10
		if e.reparse {
			attrs = 0x410
		}
	case e.symlink:
		hostOS, attrs = 1, 0o120777
	case e.reparse:
		attrs = 0x420
	}
	size := uint64(len(e.data))
	if e.declared != 0 {
		size = e.declared
	}
	body := slices.Concat(rarVint(fileFlags), rarVint(size), rarVint(attrs))
	if !e.dir {
		sum := crc32.ChecksumIEEE(e.data)
		if e.badCRC {
			sum ^= 0xffffffff
		}
		body = binary.LittleEndian.AppendUint32(body, sum)
	}
	body = slices.Concat(body, rarVint(0), rarVint(hostOS), rarVint(uint64(len(e.name))), []byte(e.name))
	var extra []byte
	if e.encrypted {
		record := slices.Concat(rarVint(1), rarVint(0), rarVint(0), []byte{15}, make([]byte, 16), make([]byte, 16))
		extra = slices.Concat(rarVint(uint64(len(record))), record)
	}
	if e.dir {
		return rarBlockExtra(2, 0, 0, body, extra)
	}
	return append(rarBlockExtra(2, 0x0002, uint64(len(e.data)), body, extra), e.data...)
}

func buildStoredRar(entries []rarEntry, opts rarOptions) []byte {
	out := []byte("Rar!\x1a\x07\x01\x00")
	if opts.encryptedHeaders {
		out = append(out, rarBlock(4, 0, 0, slices.Concat(rarVint(0), rarVint(0), []byte{15}, make([]byte, 16)))...)
	}
	arcFlags, endFlags := uint64(0), uint64(0)
	if opts.multiVolume {
		arcFlags, endFlags = 0x0001, 0x0001
	}
	out = append(out, rarBlock(1, 0, 0, rarVint(arcFlags))...)
	for _, e := range entries {
		out = append(out, rarFileBlock(e)...)
	}
	return append(out, rarBlock(5, 0, 0, rarVint(endFlags))...)
}

// writeStoredRar собирает RAR5 без сжатия: этого хватает, чтобы rardecode
// прочитал оглавление и упал на проверке контрольной суммы данных.
func writeStoredRar(t *testing.T, path string, entries []rarEntry) {
	t.Helper()
	writeRar(t, path, buildStoredRar(entries, rarOptions{}))
}

func writeRar(t *testing.T, path string, raw []byte) {
	t.Helper()
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

func storedRar(t *testing.T, entries ...rarEntry) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "game.rar")
	writeStoredRar(t, path, entries)
	return path
}

func brokenGame() rarEntry {
	return rarEntry{name: "Game/game.exe", data: []byte(builtinPayload), badCRC: true}
}

// shortGame воспроизводит отказ декодера на целом архиве: заголовок обещает
// больше байт, чем упаковано, и rardecode отвечает «decoded file too short»,
// как на 112 Operator.
func shortGame() rarEntry {
	return rarEntry{name: "Game/game.exe", data: []byte("short"), declared: uint64(len(toolPayload))}
}

type rar4Entry struct {
	name     string
	data     []byte
	badCRC   bool
	declared uint32
}

func le16(n int) uint16 { return uint16(n & 0xffff) }

func le32(n int) uint32 { return uint32(n & 0xffffffff) }

func rar4Block(typ byte, flags uint16, body []byte) []byte {
	head := binary.LittleEndian.AppendUint16([]byte{typ}, flags)
	head = binary.LittleEndian.AppendUint16(head, le16(7+len(body)))
	head = append(head, body...)
	crc := uint16(crc32.ChecksumIEEE(head) & 0xffff)
	return append(binary.LittleEndian.AppendUint16(nil, crc), head...)
}

// buildStoredRar4 собирает RAR 1.5-4 без сжатия. Конец архива у этого формата
// необязателен, и rardecode принимает обрыв на границе блока за его конец.
func buildStoredRar4(entries []rar4Entry, endBlock bool) []byte {
	out := []byte("Rar!\x1a\x07\x00")
	out = append(out, rar4Block(0x73, 0, make([]byte, 6))...)
	for _, e := range entries {
		sum := crc32.ChecksumIEEE(e.data)
		if e.badCRC {
			sum ^= 0xffffffff
		}
		unpacked := le32(len(e.data))
		if e.declared != 0 {
			unpacked = e.declared
		}
		body := binary.LittleEndian.AppendUint32(nil, le32(len(e.data)))
		body = binary.LittleEndian.AppendUint32(body, unpacked)
		body = append(body, 2)
		body = binary.LittleEndian.AppendUint32(body, sum)
		body = binary.LittleEndian.AppendUint32(body, 0)
		body = append(body, 20, 0x30)
		body = binary.LittleEndian.AppendUint16(body, le16(len(e.name)))
		body = binary.LittleEndian.AppendUint32(body, 0x20)
		body = append(body, e.name...)
		out = append(out, rar4Block(0x74, 0x8000, body)...)
		out = append(out, e.data...)
	}
	if endBlock {
		out = append(out, rar4Block(0x7b, 0x4000, nil)...)
	}
	return out
}

func brokenRar(t *testing.T, extra ...rarEntry) string {
	t.Helper()
	if len(builtinPayload) != len(toolPayload) {
		t.Fatalf("fixture payloads differ in size: %d and %d", len(builtinPayload), len(toolPayload))
	}
	return storedRar(t, append(extra, brokenGame())...)
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
	if string(got) != toolPayload {
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
		{name: "reparse point entry", modes: []string{"ok"}, extra: []rarEntry{{name: "Game/junction", reparse: true, dir: true}}, wantIs: []error{errArchiveUnsafe, errArchiveHasLinks, rardecode.ErrBadFileChecksum}},
		{name: "reparse point file entry", modes: []string{"ok"}, extra: []rarEntry{{name: "Game/link.lnk", data: []byte("x"), reparse: true}}, wantIs: []error{errArchiveUnsafe, errArchiveHasLinks, rardecode.ErrBadFileChecksum}},
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
	failing := archiveTool{name: "missing", path: filepath.Join(t.TempDir(), "absent.exe"), args: func(a, d string) []string { return nil }}
	useTools(t, failing, fakeTool(t, "ok"))
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
	useTools(t, writer, fakeTool(t, "ok"))
	dest := filepath.Join(t.TempDir(), "out")
	err := ExtractArchive(context.Background(), brokenRar(t), dest, nil)
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
	if string(data) == toolPayload {
		t.Fatal("the next tool ran after a disk write failure")
	}
}

func TestArchiveToolArgs(t *testing.T) {
	archive, dest := filepath.Join("C:", "dl", "-game [1].rar"), filepath.Join("C:", "games", "Game (x)")
	cases := []struct {
		tool archiveTool
		want []string
	}{
		{tool: sevenZipTool("7z"), want: []string{"x", "-y", "-aoa", "-snl-", "-bso0", "-bsp1", "-sccUTF-8", "-o" + dest, "--", archive}},
		{tool: unrarTool("unrar", true), want: []string{"x", "-y", "-o+", "-p-", "-idc", "-ol-", "--", archive, dest + string(filepath.Separator)}},
		{tool: unrarTool("unrar", false), want: []string{"x", "-y", "-o+", "-p-", "-idc", "--", archive, dest + string(filepath.Separator)}},
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
	const archive = "game.rar"
	pathErr := &fs.PathError{Op: "read", Path: archive, Err: fs.ErrPermission}
	archiveGone := &fs.PathError{Op: "open", Path: archive, Err: fs.ErrNotExist}
	volumeGone := &fs.PathError{Op: "open", Path: "game.r00", Err: fs.ErrNotExist}
	cases := []struct {
		name   string
		err    error
		atEOF  bool
		decode bool
		is     error
		isNot  error
	}{
		{name: "nil", err: nil},
		{name: "eof", err: io.EOF},
		{name: "file read", err: pathErr},
		{name: "wrapped file read", err: fmt.Errorf("next: %w", pathErr)},
		{name: "archive file gone", err: archiveGone, isNot: errArchiveIncomplete},
		{name: "checksum", err: rardecode.ErrBadFileChecksum, decode: true},
		{name: "unexpected EOF after the file really ended", err: io.ErrUnexpectedEOF, atEOF: true, is: errArchiveIncomplete, isNot: errUnsupportedArchive},
		{name: "unexpected EOF from the decoder while the file goes on", err: io.ErrUnexpectedEOF, decode: true},
		{name: "wrapped unexpected EOF from the decoder", err: fmt.Errorf("decode: %w", io.ErrUnexpectedEOF), decode: true},
		{name: "decoded file too short", err: rardecode.ErrShortFile, decode: true},
		{name: "decoder ran out of data", err: rardecode.ErrDecoderOutOfData, decode: true},
		{name: "encrypted", err: rardecode.ErrArchivedFileEncrypted, is: errArchiveEncrypted},
		{name: "bad password", err: rardecode.ErrBadPassword, is: errArchiveEncrypted},
		{name: "next volume missing", err: rardecode.ErrMultiVolume, is: errArchiveIncomplete},
		{name: "archive cut", err: rardecode.ErrUnexpectedArcEnd, is: errArchiveIncomplete},
		{name: "volume file missing", err: volumeGone, is: errArchiveIncomplete},
		{name: "wrapped volume file missing", err: fmt.Errorf("next: %w", volumeGone), is: errArchiveIncomplete},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := asDecodeError(archive, tc.err, tc.atEOF)
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
			if tc.isNot != nil && errors.Is(got, tc.isNot) {
				t.Fatalf("asDecodeError(%v) = %v, must not be %v", tc.err, got, tc.isNot)
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
