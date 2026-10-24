package cloudsaves

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"typhon/internal/hashdir"
)

const (
	maxUnpackedBytes = 1 << 30
	maxEntries       = 20000
)

type treeFile struct {
	rel  string
	abs  string
	size int64
	mod  time.Time
	hash string
}

type tree struct {
	files  []treeFile
	digest string
	sig    string
	size   int64
	newest time.Time
}

// scanTree lists the regular files below root. A symlink or any other
// non-regular entry is an error: skipping it would upload (or restore over) a
// save that silently lacks part of its files.
func scanTree(ctx context.Context, root string) ([]treeFile, error) {
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, fmt.Errorf("saves folder %s: %w", root, err)
	}
	var files []treeFile
	err = filepath.WalkDir(resolved, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		switch {
		case d.IsDir():
			return nil
		case d.Type()&fs.ModeSymlink != 0:
			return fmt.Errorf("%w: %s", errSymlink, path)
		case !d.Type().IsRegular():
			return fmt.Errorf("%w: %s", errIrregular, path)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(resolved, path)
		if err != nil {
			return err
		}
		files = append(files, treeFile{rel: filepath.ToSlash(rel), abs: path, size: info.Size(), mod: info.ModTime()})
		return nil
	})
	if err != nil {
		return nil, err
	}
	slices.SortFunc(files, func(a, b treeFile) int { return strings.Compare(a.rel, b.rel) })
	return files, nil
}

// digestOf is the rule the cloud contract fixes: sha256 over the files sorted
// by relative path, each contributing "<path>\n<sha256 of content>\n". It
// covers content and names only, so an archive with other timestamps but the
// same files has the same digest.
func digestOf(files []treeFile) string {
	sorted := slices.Clone(files)
	slices.SortFunc(sorted, func(a, b treeFile) int { return strings.Compare(a.rel, b.rel) })
	h := sha256.New()
	for _, f := range sorted {
		h.Write([]byte(f.rel + "\n" + f.hash + "\n"))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func hashTree(ctx context.Context, root string) (tree, error) {
	files, err := scanTree(ctx, root)
	if err != nil {
		return tree{}, err
	}
	return hashFiles(ctx, files)
}

func summarize(files []treeFile) tree {
	t := tree{files: files}
	for _, f := range files {
		t.size += f.size
		if f.mod.After(t.newest) {
			t.newest = f.mod
		}
	}
	return t
}

func hashFiles(ctx context.Context, files []treeFile) (tree, error) {
	if len(files) == 0 {
		return tree{}, nil
	}
	buf := make([]byte, 1<<20)
	for i := range files {
		f := &files[i]
		hash, size, err := hashdir.HashFile(ctx, f.abs, buf)
		if err != nil {
			return tree{}, fmt.Errorf("hash %s: %w", f.rel, err)
		}
		f.hash, f.size = hash, size
	}
	t := summarize(files)
	t.digest = digestOf(files)
	return t, nil
}

func signature(files []treeFile) string {
	h := sha256.New()
	for _, f := range files {
		h.Write(fmt.Appendf(nil, "%s|%d|%d;", f.rel, f.size, f.mod.UnixNano()))
	}
	return hex.EncodeToString(h.Sum(nil))
}

type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (c ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}

type archive struct {
	path   string
	size   int64
	digest string
}

// buildArchive zips the files into a temporary file inside dir and returns it
// together with the digest of the bytes that actually went in: a file that
// changed since the scan is hashed as it is streamed, so the digest always
// describes the archive.
func buildArchive(ctx context.Context, dir string, files []treeFile, maxBytes int64) (a archive, err error) {
	if len(files) == 0 {
		return archive{}, errEmpty
	}
	if len(files) > maxEntries {
		return archive{}, fmt.Errorf("%w: файлов %d, допустимо %d", errTooLarge, len(files), maxEntries)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return archive{}, err
	}
	out, err := os.CreateTemp(dir, "upload-*.zip")
	if err != nil {
		return archive{}, err
	}
	closed := false
	defer func() {
		if err == nil {
			return
		}
		errs := []error{err}
		if !closed {
			errs = append(errs, out.Close())
		}
		err = errors.Join(append(errs, os.Remove(out.Name()))...)
	}()

	zw := zip.NewWriter(out)
	hashed := make([]treeFile, len(files))
	var total int64
	for i, f := range files {
		h, n, err := addFile(ctx, zw, f, maxUnpackedBytes-total)
		if err != nil {
			return archive{}, err
		}
		total += n
		f.hash, f.size = h, n
		hashed[i] = f
	}
	if err := zw.Close(); err != nil {
		return archive{}, err
	}
	if err := out.Sync(); err != nil {
		return archive{}, err
	}
	info, err := out.Stat()
	if err != nil {
		return archive{}, err
	}
	closed = true
	if err := out.Close(); err != nil {
		return archive{}, err
	}
	if info.Size() > maxBytes {
		return archive{}, fmt.Errorf("%w: архив %d байт, допустимо %d", errTooLarge, info.Size(), maxBytes)
	}
	return archive{path: out.Name(), size: info.Size(), digest: digestOf(hashed)}, nil
}

func addFile(ctx context.Context, zw *zip.Writer, f treeFile, budget int64) (string, int64, error) {
	src, err := os.Open(f.abs)
	if err != nil {
		return "", 0, err
	}
	hash, n, err := copyHashed(ctx, zw, src, f, budget)
	closeErr := src.Close()
	if err != nil {
		return "", 0, err
	}
	if closeErr != nil {
		return "", 0, closeErr
	}
	return hash, n, nil
}

func copyHashed(ctx context.Context, zw *zip.Writer, src *os.File, f treeFile, budget int64) (string, int64, error) {
	info, err := src.Stat()
	if err != nil {
		return "", 0, err
	}
	if !info.Mode().IsRegular() {
		return "", 0, fmt.Errorf("%w: %s", errIrregular, f.abs)
	}
	w, err := zw.CreateHeader(&zip.FileHeader{Name: f.rel, Method: zip.Deflate, Modified: info.ModTime()})
	if err != nil {
		return "", 0, err
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(w, h), io.LimitReader(ctxReader{ctx, src}, budget+1))
	if err != nil {
		return "", 0, err
	}
	if n > budget {
		return "", 0, fmt.Errorf("%w: распакованный размер больше %d байт", errTooLarge, int64(maxUnpackedBytes))
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

func entryParts(name string) ([]string, bool, error) {
	if name == "" || strings.ContainsAny(name, "\\:\x00") || strings.HasPrefix(name, "/") {
		return nil, false, fmt.Errorf("%w: недопустимое имя записи %q", errInvalid, name)
	}
	isDir := strings.HasSuffix(name, "/")
	parts := strings.Split(strings.TrimSuffix(name, "/"), "/")
	for _, p := range parts {
		if p == "" || p == "." || p == ".." {
			return nil, false, fmt.Errorf("%w: недопустимое имя записи %q", errInvalid, name)
		}
	}
	return parts, isDir, nil
}

// extractArchive unpacks zipPath into dest, which must be an empty directory
// the caller owns. Every entry name is checked before anything is written, and
// the unpacked size is counted from the bytes actually read, not from the
// sizes the archive declares.
func extractArchive(ctx context.Context, zipPath, dest string) (err error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("%w: %w", errInvalid, err)
	}
	defer func() { err = errors.Join(err, zr.Close()) }()

	if len(zr.File) > maxEntries {
		return fmt.Errorf("%w: записей %d, допустимо %d", errTooLarge, len(zr.File), maxEntries)
	}
	seen := make(map[string]bool, len(zr.File))
	type item struct {
		f      *zip.File
		isDir  bool
		target string
	}
	items := make([]item, 0, len(zr.File))
	for _, f := range zr.File {
		parts, isDir, err := entryParts(f.Name)
		if err != nil {
			return err
		}
		mode := f.Mode()
		if mode&fs.ModeSymlink != 0 || (!isDir && !mode.IsRegular()) {
			return fmt.Errorf("%w: запись %q не обычный файл", errInvalid, f.Name)
		}
		key := strings.ToLower(strings.Join(parts, "/"))
		if seen[key] {
			return fmt.Errorf("%w: запись %q повторяется", errInvalid, f.Name)
		}
		seen[key] = true
		target := filepath.Join(dest, filepath.Join(parts...))
		rel, err := filepath.Rel(dest, target)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("%w: запись %q выходит за папку распаковки", errInvalid, f.Name)
		}
		items = append(items, item{f: f, isDir: isDir || mode.IsDir(), target: target})
	}

	var total int64
	files := 0
	for _, it := range items {
		if err := ctx.Err(); err != nil {
			return err
		}
		if it.isDir {
			if err := os.MkdirAll(it.target, 0o700); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(it.target), 0o700); err != nil {
			return err
		}
		n, err := extractFile(ctx, it.f, it.target, maxUnpackedBytes-total)
		if err != nil {
			return err
		}
		total += n
		files++
	}
	if files == 0 {
		return fmt.Errorf("%w: в архиве нет файлов", errInvalid)
	}
	return nil
}

func extractFile(ctx context.Context, f *zip.File, target string, budget int64) (int64, error) {
	rc, err := f.Open()
	if err != nil {
		return 0, fmt.Errorf("%w: %w", errInvalid, err)
	}
	out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return 0, errors.Join(err, rc.Close())
	}
	n, copyErr := io.Copy(out, io.LimitReader(ctxReader{ctx, rc}, budget+1))
	err = errors.Join(copyErr, rc.Close(), out.Close())
	if ctxErr := ctx.Err(); ctxErr != nil {
		return 0, ctxErr
	}
	if err != nil {
		return 0, fmt.Errorf("%w: %s: %w", errInvalid, f.Name, err)
	}
	if n > budget {
		return 0, fmt.Errorf("%w: распакованный размер больше %d байт", errTooLarge, int64(maxUnpackedBytes))
	}
	if !f.Modified.IsZero() {
		if err := os.Chtimes(target, f.Modified, f.Modified); err != nil {
			return 0, err
		}
	}
	return n, nil
}
