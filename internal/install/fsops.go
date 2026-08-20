package install

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

var (
	errDestExists = errors.New("путь назначения уже существует")
	errNotDir     = errors.New("источник не является папкой")
	errCopyVerify = errors.New("копирование завершилось с ошибкой проверки")
)

func CopyDir(ctx context.Context, src, dst string, onProgress func(Progress)) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errNotDir
	}
	if !destAvailable(dst) {
		return errDestExists
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}

	rep := newReporter(onProgress, DirSize(src))
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
		if !d.Type().IsRegular() {
			return nil
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

func MoveDir(ctx context.Context, src, dst string, onProgress func(Progress)) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errNotDir
	}
	if !destAvailable(dst) {
		return errDestExists
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	renameErr := os.Rename(src, dst)
	if renameErr == nil {
		total := DirSize(dst)
		rep := newReporter(onProgress, total)
		rep.done = total
		rep.flush()
		return nil
	}
	slog.Info("rename failed, falling back to copy", "src", src, "dst", dst, "error", renameErr)

	if err := CopyDir(ctx, src, dst, onProgress); err != nil {
		return err
	}
	if err := verifyCopy(src, dst); err != nil {
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
		out.Close()
		return err
	}
	return out.Close()
}

func verifyCopy(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		want, err := d.Info()
		if err != nil {
			return err
		}
		got, err := os.Stat(filepath.Join(dst, rel))
		if err != nil {
			return err
		}
		if got.Size() != want.Size() {
			slog.Error("copy size mismatch", "file", rel, "want", want.Size(), "got", got.Size())
			return errCopyVerify
		}
		return nil
	})
}

func DirSize(dir string) int64 {
	var total int64
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			if info, err := d.Info(); err == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return total
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
