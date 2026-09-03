package install

import (
	"bytes"
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCopyDirCopiesTree(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	mkText(t, filepath.Join(src, "a.txt"), "alpha")
	mkText(t, filepath.Join(src, "sub", "b.txt"), "beta")
	mkFile(t, filepath.Join(src, "sub", "deep", "c.bin"), 4096)
	if err := os.MkdirAll(filepath.Join(src, "empty"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	dst := filepath.Join(tmp, "dst")

	var last Progress
	if err := CopyDir(context.Background(), src, dst, func(p Progress) { last = p }); err != nil {
		t.Fatalf("CopyDir: %v", err)
	}

	if data, err := os.ReadFile(filepath.Join(dst, "a.txt")); err != nil || string(data) != "alpha" {
		t.Fatalf("a.txt = %q, err = %v", data, err)
	}
	if data, err := os.ReadFile(filepath.Join(dst, "sub", "b.txt")); err != nil || string(data) != "beta" {
		t.Fatalf("b.txt = %q, err = %v", data, err)
	}
	if info, err := os.Stat(filepath.Join(dst, "sub", "deep", "c.bin")); err != nil || info.Size() != 4096 {
		t.Fatalf("c.bin stat err = %v", err)
	}
	if info, err := os.Stat(filepath.Join(dst, "empty")); err != nil || !info.IsDir() {
		t.Fatalf("empty dir not copied: %v", err)
	}
	if mustDirSize(t, src) != mustDirSize(t, dst) {
		t.Fatalf("sizes differ: %d vs %d", mustDirSize(t, src), mustDirSize(t, dst))
	}
	if last.BytesTotal == 0 || last.BytesDone != last.BytesTotal {
		t.Fatalf("final progress = %+v", last)
	}
	if err := verifyCopy(context.Background(), src, dst); err != nil {
		t.Fatalf("verifyCopy: %v", err)
	}
}

func TestCopyDirRefusesNonEmptyDest(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	mkText(t, filepath.Join(src, "a.txt"), "alpha")
	dst := filepath.Join(tmp, "dst")
	mkText(t, filepath.Join(dst, "keep.txt"), "keep")

	if err := CopyDir(context.Background(), src, dst, nil); !errors.Is(err, errDestExists) {
		t.Fatalf("err = %v, want errDestExists", err)
	}
	if data, err := os.ReadFile(filepath.Join(dst, "keep.txt")); err != nil || string(data) != "keep" {
		t.Fatalf("existing file damaged: %q %v", data, err)
	}
}

func TestCopyDirPreCancelled(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	mkText(t, filepath.Join(src, "a.txt"), "alpha")
	dst := filepath.Join(tmp, "dst")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := CopyDir(ctx, src, dst, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if exists(filepath.Join(dst, "a.txt")) {
		t.Fatal("file copied despite cancellation")
	}
}

func TestCopyDirCancelMidway(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	for _, name := range []string{"1.bin", "2.bin", "3.bin", "4.bin", "5.bin"} {
		mkFile(t, filepath.Join(src, name), 300*1024)
	}
	dst := filepath.Join(tmp, "dst")

	ctx, cancel := context.WithCancel(context.Background())
	err := CopyDir(ctx, src, dst, func(Progress) { cancel() })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if info, statErr := os.Stat(filepath.Join(dst, "5.bin")); statErr == nil && info.Size() == 300*1024 {
		t.Fatal("copy did not stop early")
	}
}

func TestCopyDirRejectsFileSource(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "a.txt")
	mkText(t, src, "alpha")
	if err := CopyDir(context.Background(), src, filepath.Join(tmp, "dst"), nil); !errors.Is(err, errNotDir) {
		t.Fatalf("err = %v, want errNotDir", err)
	}
}

func TestMoveDirSameVolumeRename(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	mkText(t, filepath.Join(src, "a.txt"), "alpha")
	mkFile(t, filepath.Join(src, "sub", "b.bin"), 2048)
	want := mustDirSize(t, src)
	dst := filepath.Join(tmp, "dst")

	var last Progress
	if err := MoveDir(context.Background(), src, dst, func(p Progress) { last = p }); err != nil {
		t.Fatalf("MoveDir: %v", err)
	}
	if exists(src) {
		t.Fatal("source still present after move")
	}
	if data, err := os.ReadFile(filepath.Join(dst, "a.txt")); err != nil || string(data) != "alpha" {
		t.Fatalf("a.txt = %q, err = %v", data, err)
	}
	if mustDirSize(t, dst) != want {
		t.Fatalf("size = %d, want %d", mustDirSize(t, dst), want)
	}
	if last.BytesDone != want || last.BytesTotal != want {
		t.Fatalf("final progress = %+v, want %d", last, want)
	}
}

func TestMoveDirRefusesNonEmptyDest(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	mkText(t, filepath.Join(src, "a.txt"), "alpha")
	dst := filepath.Join(tmp, "dst")
	mkText(t, filepath.Join(dst, "keep.txt"), "keep")

	if err := MoveDir(context.Background(), src, dst, nil); !errors.Is(err, errDestExists) {
		t.Fatalf("err = %v, want errDestExists", err)
	}
	if !exists(filepath.Join(src, "a.txt")) {
		t.Fatal("source removed on refused move")
	}
}

func TestMoveDirCopyFallback(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	mkText(t, filepath.Join(src, "a.txt"), "alpha")
	mkFile(t, filepath.Join(src, "sub", "b.bin"), 1024)
	dst := filepath.Join(tmp, "dst")

	if err := CopyDir(context.Background(), src, dst, nil); err != nil {
		t.Fatalf("CopyDir: %v", err)
	}
	if err := verifyCopy(context.Background(), src, dst); err != nil {
		t.Fatalf("verifyCopy: %v", err)
	}
	if err := os.RemoveAll(src); err != nil {
		t.Fatalf("remove src: %v", err)
	}
	if !exists(filepath.Join(dst, "sub", "b.bin")) {
		t.Fatal("fallback move lost files")
	}
}

func TestVerifyCopyDetectsSameSizeDifferentContent(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	dst := filepath.Join(tmp, "dst")
	mkText(t, filepath.Join(src, "a.txt"), "alpha")
	mkText(t, filepath.Join(dst, "a.txt"), "gamma")
	if err := verifyCopy(context.Background(), src, dst); err == nil {
		t.Fatal("expected verifyCopy to detect same-size, different-content corruption")
	}
}

func TestVerifyCopyDetectsMismatch(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	dst := filepath.Join(tmp, "dst")
	mkText(t, filepath.Join(src, "a.txt"), "alpha")
	mkText(t, filepath.Join(dst, "a.txt"), "al")
	if err := verifyCopy(context.Background(), src, dst); !errors.Is(err, errCopyVerify) {
		t.Fatalf("err = %v, want errCopyVerify", err)
	}

	missing := filepath.Join(tmp, "missing")
	if err := os.MkdirAll(missing, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := verifyCopy(context.Background(), src, missing); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestDirSize(t *testing.T) {
	root := t.TempDir()
	mkFile(t, filepath.Join(root, "a.bin"), 1000)
	mkFile(t, filepath.Join(root, "sub", "b.bin"), 2000)
	got, err := DirSize(context.Background(), root)
	if err != nil {
		t.Fatalf("DirSize: %v", err)
	}
	if got != 3000 {
		t.Fatalf("DirSize = %d, want 3000", got)
	}
}

func TestDirSizeReportsFailures(t *testing.T) {
	root := t.TempDir()
	mkFile(t, filepath.Join(root, "a.bin"), 1000)
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	cases := []struct {
		name string
		ctx  context.Context
		dir  string
		want error
	}{
		{name: "missing", ctx: context.Background(), dir: filepath.Join(root, "nope"), want: os.ErrNotExist},
		{name: "file instead of dir", ctx: context.Background(), dir: filepath.Join(root, "a.bin", "sub"), want: nil},
		{name: "cancelled", ctx: cancelled, dir: root, want: context.Canceled},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			size, err := DirSize(tc.ctx, tc.dir)
			if err == nil {
				t.Fatalf("DirSize = %d, want error", size)
			}
			if size != 0 {
				t.Fatalf("size = %d, want 0 alongside the error", size)
			}
			if tc.want != nil && !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestSameVolume(t *testing.T) {
	tmp := t.TempDir()
	a := filepath.Join(tmp, "a")
	b := filepath.Join(tmp, "b")
	got := SameVolume(a, b)
	if runtime.GOOS == "windows" {
		if !got {
			t.Fatalf("SameVolume(%s, %s) = false, want true", a, b)
		}
		if !SameVolume(`c:\one`, `C:\two`) {
			t.Fatal("volume comparison must be case-insensitive")
		}
		if SameVolume(`C:\one`, `D:\two`) {
			t.Fatal("different volumes must not match")
		}
		return
	}
	if got {
		t.Fatal("SameVolume must be false on non-windows")
	}
}

func TestEntryModeCaps(t *testing.T) {
	if got := entryMode(0o777); got != 0o755 {
		t.Fatalf("entryMode(0777) = %o, want 755", got)
	}
	if got := entryMode(0o666); got != 0o644 {
		t.Fatalf("entryMode(0666) = %o, want 644", got)
	}
}

func TestCopyStreamPropagatesWriteError(t *testing.T) {
	rep := newReporter(nil, 0)
	err := copyStream(context.Background(), failWriter{}, bytes.NewReader([]byte("data")), rep, make([]byte, 4))
	if err == nil {
		t.Fatal("expected write error")
	}
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("boom") }

// mkSymlink creates a symlink or, on a Windows machine without Developer
// Mode or elevation, skips the test: the fixture itself needs the same
// privilege the code under test is expected to reject.
func mkSymlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", link, err)
	}
	if err := os.Symlink(target, link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("cannot create symlink fixture (no Developer Mode/elevation): %v", err)
		}
		t.Fatalf("symlink: %v", err)
	}
}

func TestCopyDirRecreatesSymlink(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	mkText(t, filepath.Join(src, "real.txt"), "alpha")
	mkSymlink(t, filepath.Join(src, "real.txt"), filepath.Join(src, "link.txt"))
	dst := filepath.Join(tmp, "dst")

	err := CopyDir(context.Background(), src, dst, nil)
	if errors.Is(err, errSymlinkPrivilege) {
		t.Skipf("symlink creation requires a privilege this account lacks: %v", err)
	}
	if err != nil {
		t.Fatalf("CopyDir: %v", err)
	}
	info, lerr := os.Lstat(filepath.Join(dst, "link.txt"))
	if lerr != nil {
		t.Fatalf("lstat copied link: %v", lerr)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("link.txt was not recreated as a symlink, mode = %v", info.Mode())
	}
	got, rerr := os.Readlink(filepath.Join(dst, "link.txt"))
	if rerr != nil {
		t.Fatalf("readlink: %v", rerr)
	}
	if got != filepath.Join(src, "real.txt") {
		t.Fatalf("link target = %q, want %q", got, filepath.Join(src, "real.txt"))
	}
}

func TestMergeDirWithBackupRecreatesSymlinkOverExisting(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	mkText(t, filepath.Join(src, "real.txt"), "alpha")
	mkSymlink(t, filepath.Join(src, "real.txt"), filepath.Join(src, "link.txt"))
	dst := filepath.Join(tmp, "dst")
	mkText(t, filepath.Join(dst, "link.txt"), "stale regular file")
	backup := filepath.Join(tmp, "backup")

	err := MergeDirWithBackup(context.Background(), src, dst, backup, nil)
	if errors.Is(err, errSymlinkPrivilege) {
		t.Skipf("symlink creation requires a privilege this account lacks: %v", err)
	}
	if err != nil {
		t.Fatalf("MergeDirWithBackup: %v", err)
	}
	info, lerr := os.Lstat(filepath.Join(dst, "link.txt"))
	if lerr != nil {
		t.Fatalf("lstat merged link: %v", lerr)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("link.txt was not replaced by a symlink, mode = %v", info.Mode())
	}
	if data, err := os.ReadFile(filepath.Join(backup, "link.txt")); err != nil || string(data) != "stale regular file" {
		t.Fatalf("backup of replaced link.txt = %q, err = %v", data, err)
	}
}

// mkUnixSocket lays down a bound Unix-domain socket file: a directory entry
// that is neither a regular file nor a symlink, without needing cgo or a
// non-stdlib dependency.
func mkUnixSocket(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", path, err)
	}
	l, err := net.Listen("unix", path)
	if err != nil {
		t.Skipf("unix sockets unavailable on this OS/build: %v", err)
	}
	t.Cleanup(func() {
		if err := l.Close(); err != nil {
			t.Errorf("close socket listener: %v", err)
		}
	})
}

func TestCopyDirRejectsNonRegularNonSymlink(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	mkText(t, filepath.Join(src, "a.txt"), "alpha")
	mkUnixSocket(t, filepath.Join(src, "weird.sock"))
	dst := filepath.Join(tmp, "dst")

	if err := CopyDir(context.Background(), src, dst, nil); !errors.Is(err, errNonRegular) {
		t.Fatalf("err = %v, want errNonRegular", err)
	}
}

func TestMergeDirWithBackupRejectsNonRegularNonSymlink(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	mkText(t, filepath.Join(src, "a.txt"), "alpha")
	mkUnixSocket(t, filepath.Join(src, "weird.sock"))
	dst := filepath.Join(tmp, "dst")
	backup := filepath.Join(tmp, "backup")

	if err := MergeDirWithBackup(context.Background(), src, dst, backup, nil); !errors.Is(err, errNonRegular) {
		t.Fatalf("err = %v, want errNonRegular", err)
	}
}

func TestCopyDirRejectsDestinationInsideSource(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	mkText(t, filepath.Join(src, "a.txt"), "alpha")
	dst := filepath.Join(src, "nested", "dst")

	if err := CopyDir(context.Background(), src, dst, nil); !errors.Is(err, errNestedPaths) {
		t.Fatalf("err = %v, want errNestedPaths", err)
	}
	if exists(dst) {
		t.Fatal("nested destination must not be created")
	}
}

func TestCopyDirRejectsSourceInsideDestination(t *testing.T) {
	tmp := t.TempDir()
	dst := filepath.Join(tmp, "dst")
	src := filepath.Join(dst, "sub")
	mkText(t, filepath.Join(src, "a.txt"), "alpha")

	if err := CopyDir(context.Background(), src, dst, nil); !errors.Is(err, errNestedPaths) {
		t.Fatalf("err = %v, want errNestedPaths", err)
	}
}

func TestMergeDirWithBackupRejectsNestedPaths(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	mkText(t, filepath.Join(src, "a.txt"), "alpha")
	dst := filepath.Join(src, "nested", "dst")
	backup := filepath.Join(tmp, "backup")

	if err := MergeDirWithBackup(context.Background(), src, dst, backup, nil); !errors.Is(err, errNestedPaths) {
		t.Fatalf("err = %v, want errNestedPaths", err)
	}
}

func TestMoveDirRejectsNestedPaths(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	mkText(t, filepath.Join(src, "a.txt"), "alpha")
	dst := filepath.Join(src, "nested", "dst")

	if err := MoveDir(context.Background(), src, dst, nil); !errors.Is(err, errNestedPaths) {
		t.Fatalf("err = %v, want errNestedPaths", err)
	}
	if !exists(filepath.Join(src, "a.txt")) {
		t.Fatal("source must survive a rejected nested move")
	}
}

func TestCopyDirMissingSource(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "does-not-exist")
	dst := filepath.Join(tmp, "dst")
	if err := CopyDir(context.Background(), src, dst, nil); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("err = %v, want ErrNotExist", err)
	}
}

func TestCopyDirEmptySource(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	dst := filepath.Join(tmp, "dst")
	if err := CopyDir(context.Background(), src, dst, nil); err != nil {
		t.Fatalf("CopyDir: %v", err)
	}
	info, err := os.Stat(dst)
	if err != nil || !info.IsDir() {
		t.Fatalf("dst not created as a directory: %v", err)
	}
}

func TestMoveDirCancelledContextLeavesSourceIntact(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	mkText(t, filepath.Join(src, "a.txt"), "alpha")
	dst := filepath.Join(tmp, "dst")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := MoveDir(ctx, src, dst, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if !exists(filepath.Join(src, "a.txt")) {
		t.Fatal("source removed despite a cancelled context")
	}
}

func TestCopyFileProducesReadableFileAfterSync(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src.bin")
	mkFile(t, src, 512*1024)
	dst := filepath.Join(tmp, "dst.bin")
	rep := newReporter(nil, 0)
	if err := copyFile(context.Background(), src, dst, 0o644, rep, make([]byte, copyBufferSize)); err != nil {
		t.Fatalf("copyFile: %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read copied file: %v", err)
	}
	if len(got) != 512*1024 {
		t.Fatalf("copied size = %d, want %d", len(got), 512*1024)
	}
}

func TestMergeDirWithBackupBacksUpReplacedAndRecordsAdded(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	dst := filepath.Join(tmp, "dst")
	backup := filepath.Join(tmp, "backup")
	mkText(t, filepath.Join(src, "replaced.txt"), "new content")
	mkText(t, filepath.Join(src, "added.txt"), "brand new")
	mkText(t, filepath.Join(dst, "replaced.txt"), "old content")
	mkText(t, filepath.Join(dst, "untouched.txt"), "keep me")

	if err := MergeDirWithBackup(context.Background(), src, dst, backup, nil); err != nil {
		t.Fatalf("MergeDirWithBackup: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(dst, "replaced.txt")); err != nil || string(data) != "new content" {
		t.Fatalf("replaced.txt = %q, err = %v", data, err)
	}
	if data, err := os.ReadFile(filepath.Join(dst, "added.txt")); err != nil || string(data) != "brand new" {
		t.Fatalf("added.txt = %q, err = %v", data, err)
	}
	if data, err := os.ReadFile(filepath.Join(dst, "untouched.txt")); err != nil || string(data) != "keep me" {
		t.Fatalf("untouched.txt damaged: %q, err = %v", data, err)
	}
	if data, err := os.ReadFile(filepath.Join(backup, "replaced.txt")); err != nil || string(data) != "old content" {
		t.Fatalf("backup of replaced.txt = %q, err = %v", data, err)
	}
	list, err := os.ReadFile(filepath.Join(backup, "added.list"))
	if err != nil {
		t.Fatalf("read added.list: %v", err)
	}
	if string(list) != "added.txt\n" {
		t.Fatalf("added.list = %q", list)
	}
}

func TestMergeDirWithBackupNeverRemovesWithoutBackup(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	dst := filepath.Join(tmp, "dst")
	backup := filepath.Join(tmp, "backup")
	mkText(t, filepath.Join(src, "a.txt"), "new")
	mkText(t, filepath.Join(dst, "a.txt"), "old")

	if err := MergeDirWithBackup(context.Background(), src, dst, backup, nil); err != nil {
		t.Fatalf("MergeDirWithBackup: %v", err)
	}
	// The only acceptable states at any point are: original file present,
	// backup present, or the merged file present — never neither.
	if _, err := os.Stat(filepath.Join(dst, "a.txt")); err != nil {
		t.Fatal("a.txt missing from dst after merge")
	}
	if _, err := os.Stat(filepath.Join(backup, "a.txt")); err != nil {
		t.Fatal("a.txt missing from backup after merge")
	}
}

func TestRestoreMergeBackupUndoesCrashedMerge(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	dst := filepath.Join(tmp, "dst")
	backup := filepath.Join(tmp, "backup")
	mkText(t, filepath.Join(src, "replaced.txt"), "new content")
	mkText(t, filepath.Join(src, "added.txt"), "brand new")
	mkText(t, filepath.Join(dst, "replaced.txt"), "old content")
	mkText(t, filepath.Join(dst, "untouched.txt"), "keep me")

	original := map[string]string{
		"replaced.txt":  "old content",
		"untouched.txt": "keep me",
	}

	if err := MergeDirWithBackup(context.Background(), src, dst, backup, nil); err != nil {
		t.Fatalf("MergeDirWithBackup: %v", err)
	}
	// Simulate a crash mid-merge: an unfinished .typhon-tmp left behind.
	if err := os.WriteFile(filepath.Join(dst, "added.txt.typhon-tmp"), []byte("half written"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := RestoreMergeBackup(dst, backup); err != nil {
		t.Fatalf("RestoreMergeBackup: %v", err)
	}
	for name, want := range original {
		if data, err := os.ReadFile(filepath.Join(dst, name)); err != nil || string(data) != want {
			t.Fatalf("%s = %q, err = %v, want %q", name, data, err, want)
		}
	}
	if exists(filepath.Join(dst, "added.txt")) {
		t.Fatal("added.txt was not removed on restore")
	}
	if exists(filepath.Join(dst, "added.txt.typhon-tmp")) {
		t.Fatal("leftover tmp file was not removed on restore")
	}
	if exists(backup) {
		t.Fatal("backup directory must be gone after a full restore")
	}
}

func TestRestoreMergeBackupIsIdempotent(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	dst := filepath.Join(tmp, "dst")
	backup := filepath.Join(tmp, "backup")
	mkText(t, filepath.Join(src, "replaced.txt"), "new content")
	mkText(t, filepath.Join(dst, "replaced.txt"), "old content")

	if err := MergeDirWithBackup(context.Background(), src, dst, backup, nil); err != nil {
		t.Fatalf("MergeDirWithBackup: %v", err)
	}
	if err := RestoreMergeBackup(dst, backup); err != nil {
		t.Fatalf("first RestoreMergeBackup: %v", err)
	}
	if err := RestoreMergeBackup(dst, backup); err != nil {
		t.Fatalf("second RestoreMergeBackup on an already-restored tree: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(dst, "replaced.txt")); err != nil || string(data) != "old content" {
		t.Fatalf("replaced.txt = %q, err = %v", data, err)
	}
}

func mustDirSize(t *testing.T, dir string) int64 {
	t.Helper()
	size, err := DirSize(context.Background(), dir)
	if err != nil {
		t.Fatalf("DirSize(%s): %v", dir, err)
	}
	return size
}
