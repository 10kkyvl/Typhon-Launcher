package download

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"typhon/internal/hashdir"
	"typhon/internal/platform"
)

var (
	errRepointEmptyPath    = errors.New("путь для переноса не задан")
	errRepointSamePath     = errors.New("новый путь совпадает со старым")
	errRepointNested       = errors.New("старый и новый путь не должны быть вложены друг в друга")
	errDownloadsActive     = errors.New("нельзя переносить папку загрузок, пока что-то скачивается")
	errRepointVerifyFailed = errors.New("проверка перенесённых загрузок не прошла")
	errRepointNotDir       = errors.New("папка загрузок повреждена: путь не является каталогом")
	errRepointNonRegular   = errors.New("неподдерживаемый тип файла в папке загрузок")
	errRepointOldRootStuck = errors.New("загрузки перенесены, но старую папку не удалось удалить")
)

// Repoint moves the whole downloads tree from oldRoot to newRoot and makes
// every stored Destination agree with the new location. It is used when the
// library root moves: settings derives DownloadsPath from LibraryPath, so
// there is no separate "move downloads" setting, only this operation.
//
// Repoint is self-contained: it validates, stops live torrents, moves the
// tree and rewrites bookkeeping in one call. It does not checkpoint through
// internal/relocate's journal (that package cannot reach into m.engines),
// so a process crash during the rare cross-volume copy fallback is not
// resumable — see the caller's risk notes.
//
//wails:ignore
func (m *Manager) Repoint(ctx context.Context, oldRoot, newRoot string) error {
	oldRoot = strings.TrimSpace(oldRoot)
	newRoot = strings.TrimSpace(newRoot)
	if oldRoot == "" || newRoot == "" {
		return errRepointEmptyPath
	}
	oldRoot = filepath.Clean(oldRoot)
	newRoot = filepath.Clean(newRoot)
	if platform.SamePath(oldRoot, newRoot) {
		return errRepointSamePath
	}
	if platform.Inside(oldRoot, newRoot) || platform.Inside(newRoot, oldRoot) {
		return errRepointNested
	}

	m.mu.Lock()
	for _, d := range m.items {
		if !downloadSettled(d.Status) || m.jobs[d.ID] != nil {
			m.mu.Unlock()
			return fmt.Errorf("%s: %w", d.Name, errDownloadsActive)
		}
	}
	dropIDs := make([]string, 0, len(m.engines))
	for id := range m.engines {
		dropIDs = append(dropIDs, id)
	}
	m.mu.Unlock()

	for _, id := range dropIDs {
		m.mu.Lock()
		eng := m.engines[id]
		delete(m.engines, id)
		m.mu.Unlock()
		if eng != nil {
			eng.drop()
		}
	}

	if err := moveTreeIfPresent(ctx, oldRoot, newRoot); err != nil {
		return err
	}

	m.mu.Lock()
	type moved struct {
		item *Download
		was  string
	}
	changed := make([]moved, 0, len(m.items))
	for _, d := range m.items {
		if !platform.Inside(oldRoot, d.Destination) {
			continue
		}
		rel, err := filepath.Rel(oldRoot, d.Destination)
		if err != nil {
			for _, c := range changed {
				c.item.Destination = c.was
			}
			m.mu.Unlock()
			return fmt.Errorf("rebase destination %s: %w", d.Destination, err)
		}
		changed = append(changed, moved{item: d, was: d.Destination})
		d.Destination = filepath.Join(newRoot, rel)
	}
	if len(changed) > 0 {
		if err := m.store.save(recordsFromLocked(m.items)); err != nil {
			for _, c := range changed {
				c.item.Destination = c.was
			}
			m.mu.Unlock()
			return fmt.Errorf("persist downloads: %w", err)
		}
	}
	m.wg.Add(1)
	m.mu.Unlock()

	go m.restore()
	return nil
}

func downloadSettled(s Status) bool {
	return s == StatusCompleted || s == StatusFailed || s == StatusPaused
}

// recordsFromLocked mirrors persistLocked's record construction. It is
// duplicated rather than shared because persistLocked swallows its save
// error via slog (manager.go is off limits here), and Repoint must see that
// error to roll back the Destination rewrite it just made in memory.
func recordsFromLocked(items []*Download) []record {
	records := make([]record, 0, len(items))
	for _, d := range items {
		records = append(records, record{
			ID:          d.ID,
			Name:        d.Name,
			Type:        d.Type,
			Source:      d.Source,
			InfoHash:    d.InfoHash,
			Destination: d.Destination,
			Status:      d.Status,
			Selected:    selectedIndices(d),
			Downloaded:  d.Downloaded,
			Total:       d.Total,
			Seeding:     d.Seeding,
			Flat:        d.Flat,
			InPlace:     d.InPlace,
			Origin:      d.Origin,
			AddedAt:     d.AddedAt,
			CompletedAt: d.CompletedAt,
			Error:       d.Error,
		})
	}
	return records
}

// moveTreeIfPresent moves oldRoot to newRoot. A missing oldRoot is not an
// error: it means there is nothing to move yet, or (on a recovery retry)
// that a previous call already finished the move.
func moveTreeIfPresent(ctx context.Context, oldRoot, newRoot string) error {
	info, err := os.Stat(oldRoot)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("stat %s: %w", oldRoot, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s: %w", oldRoot, errRepointNotDir)
	}
	if err := os.MkdirAll(filepath.Dir(newRoot), 0o755); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	// Повторный вход после аварии в окне между удавшимся переименованием и
	// неудавшимся удалением старой папки. Данные уже на новом месте: копировать
	// их второй раз нельзя, и переименовать поверх занятого пути тоже нельзя —
	// без этой ветки перенос корня библиотеки застревал бы навсегда.
	populated, err := dirHasEntries(newRoot)
	if err != nil {
		return err
	}
	if populated {
		if rmErr := os.RemoveAll(oldRoot); rmErr != nil {
			return fmt.Errorf("%w: %s: %w", errRepointOldRootStuck, oldRoot, rmErr)
		}
		return nil
	}
	// Пустой каталог назначения создаётся при настройке библиотеки, а
	// os.Rename на занятый путь на Windows падает.
	if err := os.Remove(newRoot); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("clear empty %s: %w", newRoot, err)
	}
	if err := os.Rename(oldRoot, newRoot); err == nil {
		return nil
	}

	staging := newRoot + ".repoint-staging"
	if err := os.RemoveAll(staging); err != nil {
		return fmt.Errorf("clear staging %s: %w", staging, err)
	}
	manifest, err := hashdir.Build(ctx, oldRoot, nil)
	if err != nil {
		return err
	}
	if err := copyTree(ctx, oldRoot, staging); err != nil {
		removeBestEffort(staging)
		return err
	}
	result, err := hashdir.Verify(ctx, staging, manifest, nil)
	if err != nil {
		removeBestEffort(staging)
		return err
	}
	if len(result.Issues) > 0 || len(result.Extra) > 0 {
		removeBestEffort(staging)
		return fmt.Errorf("%w: %s", errRepointVerifyFailed, staging)
	}
	if err := os.Rename(staging, newRoot); err != nil {
		return err
	}
	if err := os.RemoveAll(oldRoot); err != nil {
		return fmt.Errorf("remove old downloads %s: %w", oldRoot, err)
	}
	return nil
}

// dirHasEntries reports whether path is a directory that already holds
// something. A missing path is not an error here: it simply holds nothing.
func dirHasEntries(path string) (bool, error) {
	entries, err := os.ReadDir(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read %s: %w", path, err)
	}
	return len(entries) > 0, nil
}

func removeBestEffort(path string) {
	if err := os.RemoveAll(path); err != nil {
		slog.Warn("remove repoint staging", "path", path, "error", err)
	}
}

// copyTree is a trimmed copy of internal/install's CopyDir. It cannot be
// reused directly: internal/install imports internal/download, so the
// reverse import would be a cycle.
func copyTree(ctx context.Context, src, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	buf := make([]byte, 1<<20)
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
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
			return copySymlinkRepoint(path, target)
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("%w: %s", errRepointNonRegular, rel)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return copyFileRepoint(ctx, path, target, info.Mode(), buf)
	})
}

func copySymlinkRepoint(src, dst string) error {
	target, err := os.Readlink(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.Symlink(target, dst)
}

func copyFileRepoint(ctx context.Context, src, dst string, mode fs.FileMode, buf []byte) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		closeAndLog(in, src)
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode.Perm()|0o600)
	if err != nil {
		closeAndLog(in, src)
		return err
	}
	if err := copyStreamRepoint(ctx, out, in, buf); err != nil {
		closeAndLog(in, src)
		if cerr := out.Close(); cerr != nil {
			return fmt.Errorf("%w: close: %w", err, cerr)
		}
		return err
	}
	closeErr := in.Close()
	if err := out.Sync(); err != nil {
		if cerr := out.Close(); cerr != nil {
			return fmt.Errorf("%w: close: %w", err, cerr)
		}
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return closeErr
}

func closeAndLog(f *os.File, path string) {
	if err := f.Close(); err != nil {
		slog.Warn("close file", "path", path, "error", err)
	}
}

func copyStreamRepoint(ctx context.Context, out *os.File, in *os.File, buf []byte) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, rerr := in.Read(buf)
		if n > 0 {
			if _, werr := out.Write(buf[:n]); werr != nil {
				return werr
			}
		}
		if rerr == io.EOF {
			return nil
		}
		if rerr != nil {
			return rerr
		}
	}
}
