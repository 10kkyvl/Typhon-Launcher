//go:build windows

package shortcut

import (
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"testing"
	"unicode/utf16"

	"golang.org/x/sys/windows"
)

func u16(t *testing.T, n int) uint16 {
	t.Helper()
	if n < 0 {
		t.Fatalf("negative length %d", n)
	}
	if n > math.MaxUint16 {
		t.Fatalf("value %d does not fit the reparse buffer field", n)
	}
	return uint16(n & math.MaxUint16)
}

// linkDir creates a directory junction: unlike a symlink it needs no
// SeCreateSymbolicLink privilege, so the test runs without developer mode.
func linkDir(t *testing.T, target, link string) {
	t.Helper()
	abs, err := filepath.Abs(target)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(link, 0o755); err != nil {
		t.Fatal(err)
	}

	sub := utf16.Encode([]rune(`\??\` + abs))
	shown := utf16.Encode([]rune(abs))
	pathBytes := (len(sub) + 1 + len(shown) + 1) * 2
	buf := make([]byte, 16+pathBytes)
	binary.LittleEndian.PutUint32(buf[0:], windows.IO_REPARSE_TAG_MOUNT_POINT)
	binary.LittleEndian.PutUint16(buf[4:], u16(t, 8+pathBytes))
	binary.LittleEndian.PutUint16(buf[8:], 0)
	binary.LittleEndian.PutUint16(buf[10:], u16(t, len(sub)*2))
	binary.LittleEndian.PutUint16(buf[12:], u16(t, (len(sub)+1)*2))
	binary.LittleEndian.PutUint16(buf[14:], u16(t, len(shown)*2))
	off := 16
	for _, u := range sub {
		binary.LittleEndian.PutUint16(buf[off:], u)
		off += 2
	}
	off += 2
	for _, u := range shown {
		binary.LittleEndian.PutUint16(buf[off:], u)
		off += 2
	}

	p, err := windows.UTF16PtrFromString(link)
	if err != nil {
		t.Fatal(err)
	}
	h, err := windows.CreateFile(p, windows.GENERIC_WRITE, 0, nil, windows.OPEN_EXISTING,
		windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := windows.CloseHandle(h); err != nil {
			t.Errorf("close junction handle: %v", err)
		}
	}()

	var n uint32
	if err := windows.DeviceIoControl(h, windows.FSCTL_SET_REPARSE_POINT, &buf[0], u32(t, len(buf)), nil, 0, &n, nil); err != nil {
		t.Fatal(err)
	}
}

func u32(t *testing.T, n int) uint32 {
	t.Helper()
	if n < 0 {
		t.Fatalf("negative length %d", n)
	}
	if n > math.MaxUint32 {
		t.Fatalf("value %d does not fit uint32", n)
	}
	return uint32(n & math.MaxUint32)
}
