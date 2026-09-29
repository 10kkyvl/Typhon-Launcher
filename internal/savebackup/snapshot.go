package savebackup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"typhon/internal/hashdir"
	"typhon/internal/install"
	"typhon/internal/storage"
	"typhon/internal/uierr"
)

const (
	filesDirName    = "files"
	snapshotFile    = "snapshot.json"
	partialSuffix   = ".partial"
	idTimeLayout    = "20060102-150405"
	maxIDAttempts   = 1000
	maxMetadataSize = 1 << 20
	maxNameLen      = 128
)

type dedupMode int

const (
	dedupNone dedupMode = iota
	dedupLatest
	dedupAny
)

// dedupFor keeps automatic copies from crowding out useful ones: a session or
// a pre-restore copy identical to the newest snapshot adds nothing, while a
// manual or update copy is something the player or the update asked for.
func dedupFor(kind Kind) dedupMode {
	if kind == KindSession || kind == KindPreRestore {
		return dedupLatest
	}
	return dedupNone
}

type captureOpts struct {
	dedup   dedupMode
	at      time.Time
	protect string
	rotate  bool
}

func validName(s string) bool {
	if s == "" || len(s) > maxNameLen || s[0] == '.' || s[len(s)-1] == '.' {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
		default:
			return false
		}
	}
	return true
}

func validSnapshotID(s string) bool {
	return validName(s) && !strings.HasSuffix(s, partialSuffix) && !strings.Contains(s, "..")
}

func digestOf(entries []hashdir.Entry) string {
	sorted := slices.Clone(entries)
	slices.SortFunc(sorted, func(a, b hashdir.Entry) int { return strings.Compare(a.Path, b.Path) })
	h := sha256.New()
	for _, e := range sorted {
		h.Write([]byte(e.Path))
		h.Write([]byte{0})
		h.Write([]byte(e.Hash))
		h.Write([]byte{'\n'})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// capture copies src into a new snapshot. The bool reports whether a snapshot
// was created: false with a nil error means dedup found an identical one and
// returned it. A created snapshot comes back together with the rotation error
// if trimming old copies failed. The caller holds the game lock.
func (s *Service) capture(ctx context.Context, gameID, src string, kind Kind, opts captureOpts) (Snapshot, bool, error) {
	if err := ctx.Err(); err != nil {
		return Snapshot{}, false, err
	}
	info, err := os.Stat(src)
	if err != nil {
		return Snapshot{}, false, fmt.Errorf("папка сохранений %s: %w", src, err)
	}
	if !info.IsDir() {
		return Snapshot{}, false, fmt.Errorf("%w: %s", errSavesNotDir, src)
	}
	manifest, err := hashdir.Build(ctx, src, nil)
	if err != nil {
		return Snapshot{}, false, fmt.Errorf("чтение сохранений %s: %w", src, err)
	}
	digest := digestOf(manifest.Entries)

	existing, err := s.list(ctx, gameID)
	if err != nil {
		return Snapshot{}, false, err
	}
	if dup, ok := duplicate(existing, digest, kind, opts.dedup); ok {
		return dup, false, nil
	}

	gameDir := s.gameDir(gameID)
	if err := os.MkdirAll(gameDir, 0o700); err != nil {
		return Snapshot{}, false, err
	}
	if err := install.CheckFreeSpace(gameDir, manifest.TotalSize); err != nil {
		return Snapshot{}, false, uierr.Wrap(codeNoFreeSpace, err)
	}

	at := opts.at
	if at.IsZero() {
		at = s.now()
	}
	at = at.UTC()
	id, partial, err := reserveID(gameDir, kind, at)
	if err != nil {
		return Snapshot{}, false, err
	}
	fail := func(cause error) (Snapshot, bool, error) {
		return Snapshot{}, false, errors.Join(cause, os.RemoveAll(partial))
	}

	files := filepath.Join(partial, filesDirName)
	if err := install.CopyDir(ctx, src, files, nil); err != nil {
		return fail(fmt.Errorf("копирование сохранений %s: %w", src, err))
	}
	if err := verifyFiles(ctx, files, manifest); err != nil {
		return fail(err)
	}

	snap := Snapshot{
		ID:         id,
		GameID:     gameID,
		Kind:       kind,
		CreatedAt:  at,
		SourcePath: src,
		SizeBytes:  manifest.TotalSize,
		Files:      len(manifest.Entries),
		Digest:     digest,
	}
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return fail(err)
	}
	if err := storage.WriteAtomic(filepath.Join(partial, snapshotFile), data); err != nil {
		return fail(err)
	}
	final := filepath.Join(gameDir, id)
	if err := os.Rename(partial, final); err != nil {
		return fail(fmt.Errorf("публикация копии %s: %w", id, err))
	}
	snap.Path = filepath.Join(final, filesDirName)

	if !opts.rotate {
		return snap, true, nil
	}
	if err := s.rotate(ctx, gameID, snap.ID, opts.protect); err != nil {
		return snap, true, fmt.Errorf("копия %s создана, но старые копии не удалены: %w", id, err)
	}
	return snap, true, nil
}

func duplicate(existing []Snapshot, digest string, kind Kind, mode dedupMode) (Snapshot, bool) {
	for _, snap := range existing {
		if snap.Broken {
			continue
		}
		switch mode {
		case dedupLatest:
			return snap, snap.Digest == digest
		case dedupAny:
			if snap.Kind == kind && snap.Digest == digest {
				return snap, true
			}
		}
		if mode != dedupAny {
			break
		}
	}
	return Snapshot{}, false
}

func verifyFiles(ctx context.Context, dir string, manifest hashdir.Manifest) error {
	result, err := hashdir.Verify(ctx, dir, manifest, nil)
	if err != nil {
		return err
	}
	if len(result.Issues) > 0 {
		issue := result.Issues[0]
		return fmt.Errorf("%w: %s: %s", errVerify, issue.Path, issue.Kind)
	}
	if len(result.Extra) > 0 {
		return fmt.Errorf("%w: лишний файл %s", errVerify, result.Extra[0])
	}
	return nil
}

func reserveID(gameDir string, kind Kind, at time.Time) (id, partial string, err error) {
	base := at.Format(idTimeLayout) + "-" + string(kind)
	for n := 1; n <= maxIDAttempts; n++ {
		id = base
		if n > 1 {
			id = fmt.Sprintf("%s-%d", base, n)
		}
		final := filepath.Join(gameDir, id)
		if _, err := os.Lstat(final); err == nil {
			continue
		} else if !errors.Is(err, fs.ErrNotExist) {
			return "", "", err
		}
		partial = final + partialSuffix
		err := os.Mkdir(partial, 0o700)
		if err == nil {
			return id, partial, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return "", "", err
		}
	}
	return "", "", fmt.Errorf("не удалось подобрать свободный идентификатор копии в %s", gameDir)
}

func (s *Service) list(ctx context.Context, gameID string) ([]Snapshot, error) {
	entries, err := os.ReadDir(s.gameDir(gameID))
	if errors.Is(err, fs.ErrNotExist) {
		return []Snapshot{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("чтение списка копий: %w", err)
	}
	out := make([]Snapshot, 0, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !entry.IsDir() || !validSnapshotID(entry.Name()) {
			continue
		}
		snap, ok := s.readSnapshot(gameID, entry.Name())
		if ok {
			out = append(out, snap)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].ID > out[j].ID
	})
	return out, nil
}

// readSnapshot reports false only for a directory that vanished between the
// listing and the read (a concurrent delete). Anything else that is wrong with
// a snapshot comes back marked Broken.
func (s *Service) readSnapshot(gameID, id string) (Snapshot, bool) {
	dir := filepath.Join(s.gameDir(gameID), id)
	snap, err := inspectSnapshot(gameID, id, dir)
	if err == nil {
		return snap, true
	}
	if _, statErr := os.Stat(dir); errors.Is(statErr, fs.ErrNotExist) {
		return Snapshot{}, false
	}
	return brokenSnapshot(gameID, id, dir, err), true
}

func inspectSnapshot(gameID, id, dir string) (Snapshot, error) {
	data, err := readLimited(filepath.Join(dir, snapshotFile), maxMetadataSize)
	if err != nil {
		return Snapshot{}, fmt.Errorf("snapshot.json: %w", err)
	}
	var snap Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return Snapshot{}, fmt.Errorf("snapshot.json: %w", err)
	}
	switch {
	case snap.ID != id:
		return Snapshot{}, fmt.Errorf("идентификатор %q не совпадает с именем папки", snap.ID)
	case snap.GameID != gameID:
		return Snapshot{}, fmt.Errorf("копия принадлежит игре %q", snap.GameID)
	case !snap.Kind.valid():
		return Snapshot{}, fmt.Errorf("неизвестный вид копии %q", snap.Kind)
	}
	files := filepath.Join(dir, filesDirName)
	info, err := os.Stat(files)
	if err != nil {
		return Snapshot{}, fmt.Errorf("каталог files: %w", err)
	}
	if !info.IsDir() {
		return Snapshot{}, errors.New("files не является папкой")
	}
	snap.Path = files
	snap.Broken, snap.Problem = false, ""
	return snap, nil
}

func brokenSnapshot(gameID, id, dir string, cause error) Snapshot {
	snap := Snapshot{ID: id, GameID: gameID, Path: dir, Broken: true, Problem: cause.Error()}
	if len(id) > len(idTimeLayout) {
		if at, err := time.Parse(idTimeLayout, id[:len(idTimeLayout)]); err == nil {
			snap.CreatedAt = at.UTC()
		}
		rest := strings.TrimPrefix(id[len(idTimeLayout):], "-")
		for _, kind := range []Kind{KindPreRestore, KindManual, KindSession, KindUpdate} {
			if strings.HasPrefix(rest, string(kind)) {
				snap.Kind = kind
				break
			}
		}
	}
	return snap
}

func readLimited(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	data, readErr := io.ReadAll(io.LimitReader(f, limit+1))
	closeErr := f.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("файл больше %d байт", limit)
	}
	return data, nil
}

// rotate keeps the newest Limit valid snapshots plus every protected one and
// removes the rest. Broken snapshots are neither counted nor removed: nothing
// about them can be trusted, so the player decides.
func (s *Service) rotate(ctx context.Context, gameID string, keep ...string) error {
	limit := max(s.config().SaveBackupLimit, 1)
	list, err := s.list(ctx, gameID)
	if err != nil {
		return err
	}
	kept := 0
	var errs []error
	for _, snap := range list {
		if snap.Broken {
			continue
		}
		if slices.Contains(keep, snap.ID) || kept < limit {
			kept++
			continue
		}
		if err := removeSnapshotDir(filepath.Join(s.gameDir(gameID), snap.ID)); err != nil {
			errs = append(errs, fmt.Errorf("удаление копии %s: %w", snap.ID, err))
		}
	}
	if len(errs) > 0 {
		return uierr.Wrap(codeRotationFailed, errors.Join(errs...))
	}
	return nil
}

// removeSnapshotDir hides the snapshot with one rename before deleting it, so
// a deletion that dies halfway leaves a .partial directory (swept on the next
// operation) rather than a snapshot missing half of its files.
func removeSnapshotDir(dir string) error {
	hidden := dir + partialSuffix
	if err := os.Rename(dir, hidden); err != nil {
		return err
	}
	return os.RemoveAll(hidden)
}
