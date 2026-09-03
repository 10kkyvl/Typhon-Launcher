package install

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"typhon/internal/hashdir"
	"typhon/internal/platform"
)

// errnoPrivilegeNotHeld is Windows ERROR_PRIVILEGE_NOT_HELD, the error
// os.Symlink returns without Developer Mode or elevation. It is not the
// generic access-denied code, so errors.Is(err, os.ErrPermission) misses it.
const errnoPrivilegeNotHeld = syscall.Errno(1314)

var (
	errDestExists       = errors.New("путь назначения уже существует")
	errNotDir           = errors.New("источник не является папкой")
	errCopyVerify       = errors.New("копирование завершилось с ошибкой проверки")
	errNonRegular       = errors.New("неподдерживаемый тип файла в источнике")
	errSymlinkPrivilege = errors.New("нет прав на создание символических ссылок")
	errNestedPaths      = errors.New("источник и назначение не должны быть вложены друг в друга")
)

func CopyDir(ctx context.Context, src, dst string, onProgress func(Progress)) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errNotDir
	}
	if err := checkNotNested(src, dst); err != nil {
		return err
	}
	if !destAvailable(dst) {
		return errDestExists
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}

	total, err := DirSize(ctx, src)
	if err != nil {
		return err
	}
	rep := newReporter(onProgress, total)
	buf := make([]byte, copyBufferSize)
	walkErr := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return copySymlink(path, target)
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("%w: %s", errNonRegular, rel)
		}
		entry, err := d.Info()
		if err != nil {
			return err
		}
		rep.setFile(rel)
		return copyFile(ctx, path, target, entry.Mode(), rep, buf)
	})
	if walkErr != nil {
		return walkErr
	}
	rep.flush()
	return nil
}

const (
	mergeTmpSuffix   = ".typhon-tmp"
	mergeAddedList   = "added.list"
	mergeAddedFileMd = 0o644
)

// MergeDirWithBackup applies src on top of dst the way a patch does, but
// never removes a target file before a copy of it (or a record that it did
// not exist) is safely rented aside in backup: RestoreMergeBackup can then
// undo the merge after a crash at any point (invariant 13).
func MergeDirWithBackup(ctx context.Context, src, dst, backup string, onProgress func(Progress)) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errNotDir
	}
	if err := checkNotNested(src, dst); err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(backup, 0o755); err != nil {
		return err
	}

	total, err := DirSize(ctx, src)
	if err != nil {
		return err
	}
	rep := newReporter(onProgress, total)
	buf := make([]byte, copyBufferSize)
	walkErr := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if err := backupBeforeReplace(target, backup, rel); err != nil {
			return err
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return copySymlink(path, target)
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("%w: %s", errNonRegular, rel)
		}
		entry, err := d.Info()
		if err != nil {
			return err
		}
		rep.setFile(rel)
		tmp := target + mergeTmpSuffix
		if err := copyFile(ctx, path, tmp, entry.Mode(), rep, buf); err != nil {
			return err
		}
		return os.Rename(tmp, target)
	})
	if walkErr != nil {
		return walkErr
	}
	rep.flush()
	return nil
}

// backupBeforeReplace moves target out of the way before MergeDirWithBackup
// overwrites it, or, when target does not exist yet, records rel in
// added.list so RestoreMergeBackup knows to delete it rather than look for
// a backup that was never made.
func backupBeforeReplace(target, backup, rel string) error {
	if _, err := os.Lstat(target); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return appendAddedList(backup, rel)
		}
		return err
	}
	backupPath := filepath.Join(backup, rel)
	if err := os.MkdirAll(filepath.Dir(backupPath), 0o755); err != nil {
		return err
	}
	return os.Rename(target, backupPath)
}

func appendAddedList(backup, rel string) error {
	f, err := os.OpenFile(filepath.Join(backup, mergeAddedList), os.O_APPEND|os.O_CREATE|os.O_WRONLY, mergeAddedFileMd)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(rel + "\n"); err != nil {
		return errors.Join(err, f.Close())
	}
	if err := f.Sync(); err != nil {
		return errors.Join(err, f.Close())
	}
	return f.Close()
}

// RestoreMergeBackup undoes a MergeDirWithBackup call, whether it finished,
// failed partway, or the process crashed mid-copy. It is idempotent: a
// second call on an already-restored (or half-restored) backup completes
// without touching what a prior call already fixed.
func RestoreMergeBackup(dst, backup string) error {
	if _, err := os.Stat(backup); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	addedPath := filepath.Join(backup, mergeAddedList)
	data, err := os.ReadFile(addedPath)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	for _, rel := range strings.Split(string(data), "\n") {
		if rel == "" {
			continue
		}
		target := filepath.Join(dst, rel)
		if err := removeExisting(target); err != nil {
			return err
		}
		if err := removeExisting(target + mergeTmpSuffix); err != nil {
			return err
		}
	}

	walkErr := filepath.WalkDir(backup, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(backup, path)
		if err != nil {
			return err
		}
		if rel == "." || rel == mergeAddedList {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		target := filepath.Join(dst, rel)
		if err := removeExisting(target + mergeTmpSuffix); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.Rename(path, target)
	})
	if walkErr != nil {
		return walkErr
	}
	return os.RemoveAll(backup)
}

// removeExisting clears the way for a fresh write in MergeDir; the caller
// writes the replacement right after, so a missing target is not an error.
func removeExisting(target string) error {
	if err := os.Remove(target); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// copySymlink recreates a symlink instead of silently skipping it: skipping
// followed by a source RemoveAll (MoveDir's fallback path) would otherwise
// delete the link's only record.
func copySymlink(src, dst string) error {
	target, err := os.Readlink(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if err := removeExisting(dst); err != nil {
		return err
	}
	if err := os.Symlink(target, dst); err != nil {
		if runtime.GOOS == "windows" && (errors.Is(err, errnoPrivilegeNotHeld) || errors.Is(err, os.ErrPermission)) {
			return fmt.Errorf("%w: %s: %w", errSymlinkPrivilege, dst, err)
		}
		return err
	}
	return nil
}

func checkNotNested(src, dst string) error {
	if platform.Inside(src, dst) || platform.Inside(dst, src) {
		return fmt.Errorf("%w: %s, %s", errNestedPaths, src, dst)
	}
	return nil
}

// CopyDirVerified copies src into dst and hashes both trees afterward, so a
// caller that is about to do something destructive to src (or trust dst as a
// safety backup) can rely on the copy instead of just on CopyDir returning nil.
func CopyDirVerified(ctx context.Context, src, dst string, onProgress func(Progress)) error {
	if err := CopyDir(ctx, src, dst, onProgress); err != nil {
		return err
	}
	return verifyCopy(ctx, src, dst)
}

func MoveDir(ctx context.Context, src, dst string, onProgress func(Progress)) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errNotDir
	}
	if err := checkNotNested(src, dst); err != nil {
		return err
	}
	if !destAvailable(dst) {
		return errDestExists
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	renameErr := os.Rename(src, dst)
	if renameErr == nil {
		total, err := DirSize(ctx, dst)
		if err != nil {
			return err
		}
		rep := newReporter(onProgress, total)
		rep.done = total
		rep.flush()
		return nil
	}
	slog.Info("rename failed, falling back to copy", "src", src, "dst", dst, "error", renameErr)

	if err := CopyDir(ctx, src, dst, onProgress); err != nil {
		return err
	}
	if err := verifyCopy(ctx, src, dst); err != nil {
		return err
	}
	return os.RemoveAll(src)
}

func copyFile(ctx context.Context, src, dst string, mode fs.FileMode, rep *reporter, buf []byte) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, entryMode(mode))
	if err != nil {
		return err
	}
	if err := copyStream(ctx, out, in, rep, buf); err != nil {
		if cerr := out.Close(); cerr != nil {
			return fmt.Errorf("%w: close: %w", err, cerr)
		}
		return err
	}
	if err := out.Sync(); err != nil {
		if cerr := out.Close(); cerr != nil {
			return fmt.Errorf("%w: close: %w", err, cerr)
		}
		return err
	}
	return out.Close()
}

// verifyCopy hashes both trees and compares content, not just size: two
// files of equal length can still differ, and a size-only check would let
// the caller delete the only good copy of the data.
func verifyCopy(ctx context.Context, src, dst string) error {
	m, err := hashdir.Build(ctx, src, nil)
	if err != nil {
		return err
	}
	result, err := hashdir.Verify(ctx, dst, m, nil)
	if err != nil {
		return err
	}
	if len(result.Issues) > 0 {
		issue := result.Issues[0]
		return fmt.Errorf("%w: %s: %s", errCopyVerify, issue.Path, issue.Kind)
	}
	if len(result.Extra) > 0 {
		return fmt.Errorf("%w: unexpected extra file %s", errCopyVerify, result.Extra[0])
	}
	return nil
}

func DirSize(ctx context.Context, dir string) (int64, error) {
	var total int64
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("stat %s: %w", path, err)
		}
		total += info.Size()
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("measure %s: %w", dir, err)
	}
	return total, nil
}

func SameVolume(a, b string) bool {
	if runtime.GOOS != "windows" {
		return false
	}
	absA, err := filepath.Abs(a)
	if err != nil {
		return false
	}
	absB, err := filepath.Abs(b)
	if err != nil {
		return false
	}
	volA := filepath.VolumeName(absA)
	volB := filepath.VolumeName(absB)
	if volA == "" || volB == "" {
		return false
	}
	return strings.EqualFold(volA, volB)
}

func destAvailable(path string) bool {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return true
	}
	if err != nil || !info.IsDir() {
		return false
	}
	entries, err := os.ReadDir(path)
	return err == nil && len(entries) == 0
}
