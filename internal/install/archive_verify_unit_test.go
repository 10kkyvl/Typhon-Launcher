package install

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"

	rardecode "github.com/nwaples/rardecode/v2"
)

func listingOf(headers ...rardecode.FileHeader) []*rardecode.File {
	files := make([]*rardecode.File, 0, len(headers))
	for _, h := range headers {
		files = append(files, &rardecode.File{FileHeader: h})
	}
	return files
}

func TestVerifyExtractedAgainstListing(t *testing.T) {
	cases := []struct {
		name    string
		disk    map[string]string
		dirs    []string
		listed  []rardecode.FileHeader
		wantIs  error
		wantIn  string
		wantErr bool
	}{
		{name: "nothing listed, nothing written"},
		{
			name:   "sizes match",
			disk:   map[string]string{"Game/a.bin": "12345", "Game/empty.txt": ""},
			listed: []rardecode.FileHeader{{Name: "Game/a.bin", UnPackedSize: 5}, {Name: "Game/empty.txt"}},
		},
		{
			name:   "parent directories are implied by the entries below them",
			disk:   map[string]string{"Game/data/deep/a.bin": "x"},
			listed: []rardecode.FileHeader{{Name: "Game/data/deep/a.bin", UnPackedSize: 1}},
		},
		{
			name:   "listed empty directory exists",
			dirs:   []string{"Game/empty"},
			listed: []rardecode.FileHeader{{Name: "Game/empty/", IsDir: true}},
		},
		{
			name:   "unknown size: existence is enough",
			disk:   map[string]string{"Game/a.bin": "any length at all"},
			listed: []rardecode.FileHeader{{Name: "Game/a.bin", UnPackedSize: 1, UnKnownSize: true}},
		},
		{
			name:    "unknown size: the file must still exist",
			listed:  []rardecode.FileHeader{{Name: "Game/a.bin", UnKnownSize: true}},
			wantIs:  errFileGone,
			wantIn:  `"Game/a.bin"`,
			wantErr: true,
		},
		{
			name:    "size differs",
			disk:    map[string]string{"Game/a.bin": "123"},
			listed:  []rardecode.FileHeader{{Name: "Game/a.bin", UnPackedSize: 5}},
			wantIs:  errToolOutput,
			wantIn:  "ожидалось 5 байт, на диске 3",
			wantErr: true,
		},
		{
			name:    "the first mismatch in listing order is the one reported",
			disk:    map[string]string{"Game/b.bin": "1"},
			listed:  []rardecode.FileHeader{{Name: "Game/z.bin", UnPackedSize: 1}, {Name: "Game/a.bin", UnPackedSize: 1}, {Name: "Game/b.bin", UnPackedSize: 1}},
			wantIs:  errFileGone,
			wantIn:  `"Game/z.bin"`,
			wantErr: true,
		},
		{
			name:    "unlisted file",
			disk:    map[string]string{"Game/a.bin": "1", "Game/stray.dll": "1"},
			listed:  []rardecode.FileHeader{{Name: "Game/a.bin", UnPackedSize: 1}},
			wantIs:  errArchiveUnlisted,
			wantIn:  `"Game/stray.dll"`,
			wantErr: true,
		},
		{
			name:    "unlisted directory",
			dirs:    []string{"Game/stray"},
			disk:    map[string]string{"Game/a.bin": "1"},
			listed:  []rardecode.FileHeader{{Name: "Game/a.bin", UnPackedSize: 1}},
			wantIs:  errArchiveMismatch,
			wantErr: true,
		},
		{
			name:    "directory where a file is listed",
			dirs:    []string{"Game/a.bin"},
			listed:  []rardecode.FileHeader{{Name: "Game/a.bin", UnPackedSize: 1}},
			wantIs:  errToolOutput,
			wantIn:  "на месте файла каталог",
			wantErr: true,
		},
		{
			name:    "file where a directory is listed",
			disk:    map[string]string{"Game/data": "1"},
			listed:  []rardecode.FileHeader{{Name: "Game/data", IsDir: true}},
			wantIs:  errToolOutput,
			wantIn:  "на месте каталога файл",
			wantErr: true,
		},
		{
			name:    "listed name leaves the destination",
			listed:  []rardecode.FileHeader{{Name: "../evil.txt", UnPackedSize: 1}},
			wantIs:  errUnsafePath,
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dest := t.TempDir()
			for name, content := range tc.disk {
				mkText(t, filepath.Join(dest, filepath.FromSlash(name)), content)
			}
			for _, dir := range tc.dirs {
				if err := os.MkdirAll(filepath.Join(dest, filepath.FromSlash(dir)), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			err := verifyExtracted(context.Background(), dest, listingOf(tc.listed...), newReporter(nil, 0))
			if !tc.wantErr {
				if err != nil {
					t.Fatalf("verifyExtracted: %v", err)
				}
				return
			}
			if !errors.Is(err, tc.wantIs) {
				t.Fatalf("err = %v, want errors.Is %v", err, tc.wantIs)
			}
			if tc.wantIn != "" && !strings.Contains(err.Error(), tc.wantIn) {
				t.Fatalf("err = %v, want it to mention %q", err, tc.wantIn)
			}
		})
	}
}

func TestVerifyExtractedStopsWhenCancelled(t *testing.T) {
	dest := t.TempDir()
	mkText(t, filepath.Join(dest, "a.bin"), "1")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := verifyExtracted(ctx, dest, listingOf(rardecode.FileHeader{Name: "a.bin", UnPackedSize: 1}), newReporter(nil, 0))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestVerifyExtractedKeepsReadOnlyFiles(t *testing.T) {
	dest := t.TempDir()
	target := filepath.Join(dest, "Game", "readme.txt")
	mkText(t, target, "read only")
	if err := os.Chmod(target, 0o400); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(target, 0o600); err != nil {
			t.Errorf("restore mode: %v", err)
		}
	})
	files := listingOf(rardecode.FileHeader{Name: "Game/readme.txt", UnPackedSize: 9})
	if err := verifyExtracted(context.Background(), dest, files, newReporter(nil, 0)); err != nil {
		t.Fatalf("verifyExtracted: %v", err)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o200 != 0 {
		t.Fatalf("mode = %v: the read-only attribute was not put back", info.Mode())
	}
}

func TestVerifyExtractedFindsLinksWithoutHelpFromTheListing(t *testing.T) {
	t.Run("hard link", func(t *testing.T) {
		dest := t.TempDir()
		mkText(t, filepath.Join(dest, "a.bin"), "1")
		if err := os.Link(filepath.Join(dest, "a.bin"), filepath.Join(dest, "b.bin")); err != nil {
			t.Fatal(err)
		}
		files := listingOf(rardecode.FileHeader{Name: "a.bin", UnPackedSize: 1}, rardecode.FileHeader{Name: "b.bin", UnPackedSize: 1})
		if err := verifyExtracted(context.Background(), dest, files, newReporter(nil, 0)); !errors.Is(err, errArchiveHardLinked) || !errors.Is(err, errArchiveUnsafe) {
			t.Fatalf("err = %v, want errArchiveHardLinked", err)
		}
	})
	t.Run("link to a directory", func(t *testing.T) {
		dest, outside := t.TempDir(), t.TempDir()
		mkText(t, filepath.Join(outside, "secret.bin"), "1")
		if err := makeDirLink(filepath.Join(dest, "lnk"), outside); err != nil {
			t.Fatal(err)
		}
		files := listingOf(rardecode.FileHeader{Name: "lnk", IsDir: true})
		err := verifyExtracted(context.Background(), dest, files, newReporter(nil, 0))
		if !errors.Is(err, errArchiveHasLinks) || !errors.Is(err, errArchiveUnsafe) {
			t.Fatalf("err = %v, want errArchiveHasLinks", err)
		}
		if strings.Contains(err.Error(), "secret.bin") {
			t.Fatalf("err = %v: the walk went through the link", err)
		}
	})
}

func TestIsLinkEntry(t *testing.T) {
	cases := []struct {
		name string
		h    rardecode.FileHeader
		want bool
	}{
		{name: "windows file", h: rardecode.FileHeader{HostOS: rardecode.HostOSWindows, Attributes: 0x20}},
		{name: "windows read-only file", h: rardecode.FileHeader{HostOS: rardecode.HostOSWindows, Attributes: 0x21}},
		{name: "windows directory", h: rardecode.FileHeader{HostOS: rardecode.HostOSWindows, Attributes: 0x10, IsDir: true}},
		{name: "windows junction", h: rardecode.FileHeader{HostOS: rardecode.HostOSWindows, Attributes: 0x410, IsDir: true}, want: true},
		{name: "windows file symlink", h: rardecode.FileHeader{HostOS: rardecode.HostOSWindows, Attributes: 0x420}, want: true},
		{name: "unix regular file", h: rardecode.FileHeader{HostOS: rardecode.HostOSUnix, Attributes: 0o100644}},
		{name: "unix directory", h: rardecode.FileHeader{HostOS: rardecode.HostOSUnix, Attributes: 0o040755, IsDir: true}},
		{name: "unix permissions without a type", h: rardecode.FileHeader{HostOS: rardecode.HostOSUnix, Attributes: 0o644}},
		{name: "unix symlink", h: rardecode.FileHeader{HostOS: rardecode.HostOSUnix, Attributes: 0o120777}, want: true},
		{name: "unix fifo", h: rardecode.FileHeader{HostOS: rardecode.HostOSUnix, Attributes: 0o010644}, want: true},
		{name: "unix character device", h: rardecode.FileHeader{HostOS: rardecode.HostOSUnix, Attributes: 0o020644}, want: true},
		{name: "unix socket", h: rardecode.FileHeader{HostOS: rardecode.HostOSUnix, Attributes: 0o140644}, want: true},
		{name: "unknown host regular file", h: rardecode.FileHeader{HostOS: rardecode.HostOSUnknown, Attributes: 0o644}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isLinkEntry(&tc.h); got != tc.want {
				t.Fatalf("isLinkEntry(%+v) = %v, want %v", tc.h, got, tc.want)
			}
		})
	}
}

func TestEntryLabelIsSafeToShow(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{name: "plain", in: "Game/data/a.bin"},
		{name: "line break and escape", in: "Game/a\nb\x1b[31m.bin"},
		{name: "text direction override", in: "Game/\u202eexe.txt"},
		{name: "invalid utf-8", in: "Game/\xff\xfe.bin"},
		{name: "very long", in: "Game/" + strings.Repeat("long", 100)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := entryLabel(tc.in)
			if len([]rune(got)) > entryLabelRunes+8 {
				t.Fatalf("label is %d runes long: %q", len([]rune(got)), got)
			}
			for _, r := range got {
				if unicode.IsControl(r) || r == '\u202e' || r == unicode.ReplacementChar {
					t.Fatalf("label %q still carries %U", got, r)
				}
			}
		})
	}
	if got := entryLabel("Game/data/a.bin"); got != `"Game/data/a.bin"` {
		t.Fatalf("label = %s, want a plain quoted name", got)
	}
}

func TestSevenZipWriteFailureMarkers(t *testing.T) {
	cases := []struct {
		name   string
		detail string
		want   bool
	}{
		{name: "output file denied", detail: "ERROR: Cannot open output file : Отказано в доступе. : C:\\g\\Game\\a.exe", want: true},
		{name: "output file locked", detail: "ERROR: Cannot delete output file : Процесс не может получить доступ к файлу. : C:\\g\\a.exe", want: true},
		{name: "folder", detail: "ERROR: Cannot create folder : Невозможно создать файл. : C:\\g\\Game", want: true},
		{name: "output folder cannot be deleted", detail: "ERROR: Cannot delete output folder : Каталог не пуст. : C:\\g\\Game", want: true},
		{name: "upper case", detail: "ERROR: CANNOT OPEN OUTPUT FILE : denied", want: true},
		{name: "crc", detail: "ERROR: CRC Failed : Game\\a.exe"},
		{name: "data error", detail: "ERROR: Data Error : Game\\a.exe"},
		{name: "unexpected end", detail: "ERROR: Unexpected end of archive"},
		{name: "headers", detail: "ERROR: Headers Error"},
		{name: "symbolic link", detail: "ERROR: Cannot create symbolic link : Клиент не обладает требуемыми правами. : out\\junc"},
		{name: "system error without text", detail: "System ERROR: Недостаточно места на диске."},
		{name: "empty"},
	}
	seen := func(tool archiveTool, text string) bool {
		scan := newMarkerScan(tool.writeMarkers)
		scan.feed(text)
		return scan.found(tool.writeMarkers)
	}
	tool := sevenZipTool("7z")
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := seen(tool, tc.detail); got != tc.want {
				t.Fatalf("write failure in %q = %v, want %v", tc.detail, got, tc.want)
			}
		})
	}
	if seen(unrarTool("unrar", true), "ERROR: Cannot open output file") {
		t.Fatal("UnRAR reports write failures by exit code, not by 7-Zip's phrases")
	}
}
