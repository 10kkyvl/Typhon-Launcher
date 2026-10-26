package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"typhon/internal/storage"
)

const (
	replacedDirName    = "replaced"
	replacedJournal    = "journal.json"
	replacedJournalVer = 1
)

// replacedFile is one live state file as it stood before a rollback wrote the
// saved copy over it.
type replacedFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

// replacedRecord lists the files a rollback is about to overwrite. From is the
// version the rollback leaves and To the one it goes to: which of them the
// launcher is running decides, after a crash, whether the files go back.
type replacedRecord struct {
	From  string         `json:"from"`
	To    string         `json:"to"`
	Files []replacedFile `json:"files"`
}

func replacedPath(configDir string) (string, error) {
	dir, err := PreviousDir(configDir)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, replacedDirName), nil
}

func (r replacedRecord) validate() error {
	if r.From == "" || r.To == "" {
		return errors.New("versions are missing")
	}
	for _, f := range r.Files {
		native := filepath.FromSlash(f.Path)
		if f.Path == "" || !filepath.IsLocal(native) || filepath.ToSlash(filepath.Clean(native)) != f.Path || !strings.HasSuffix(f.Path, ".json") {
			return fmt.Errorf("file path %q", f.Path)
		}
		if err := validateHash(f.SHA256); err != nil {
			return fmt.Errorf("file %q: %w", f.Path, err)
		}
	}
	return nil
}

// saveReplaced copies what the live files hold now, before the first of them
// is overwritten, and lists them in a journal that is written after the copies.
// A journal on disk therefore promises every copy it names; copies without one
// belong to a rollback that had not yet changed anything.
func saveReplaced(ctx context.Context, configDir string, items []restoreItem, from, to string) error {
	dir, err := replacedPath(configDir)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("drop earlier replaced files: %w", err)
	}
	if err := os.MkdirAll(dir, cacheDirMode); err != nil {
		return err
	}
	rec := replacedRecord{From: from, To: to}
	for _, it := range items {
		if err := ctx.Err(); err != nil {
			return err
		}
		dst := filepath.Join(dir, filepath.FromSlash(it.rel))
		if err := os.MkdirAll(filepath.Dir(dst), cacheDirMode); err != nil {
			return err
		}
		if err := storage.WriteAtomic(dst, it.old); err != nil {
			return fmt.Errorf("keep %s: %w", it.rel, err)
		}
		sum := sha256.Sum256(it.old)
		rec.Files = append(rec.Files, replacedFile{Path: it.rel, SHA256: hex.EncodeToString(sum[:]), Size: int64(len(it.old))})
	}
	if err := storage.Save(filepath.Join(dir, replacedJournal), replacedJournalVer, rec); err != nil {
		return fmt.Errorf("write the replaced files journal: %w", err)
	}
	return nil
}

// undoReplaced puts every file the journal names back and, only if all of them
// went back, removes the copies. A failure leaves the journal in place for the
// next start. It does not take a context: an undo that is cancelled half-way
// is the damage it exists to repair.
func undoReplaced(configDir string) error {
	dir, err := replacedPath(configDir)
	if err != nil {
		return err
	}
	var rec replacedRecord
	if err := storage.Load(filepath.Join(dir, replacedJournal), replacedJournalVer, nil, &rec); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return os.RemoveAll(dir)
		}
		return fmt.Errorf("read the replaced files journal: %w", err)
	}
	if err := rec.validate(); err != nil {
		return fmt.Errorf("replaced files journal: %w", err)
	}
	var errs []error
	for _, f := range rec.Files {
		native := filepath.FromSlash(f.Path)
		data, err := os.ReadFile(filepath.Join(dir, native))
		if err != nil {
			errs = append(errs, fmt.Errorf("read the kept copy of %s: %w", f.Path, err))
			continue
		}
		sum := sha256.Sum256(data)
		if int64(len(data)) != f.Size || hex.EncodeToString(sum[:]) != f.SHA256 {
			errs = append(errs, fmt.Errorf("%w: kept copy of %s", ErrPreviousStateHash, f.Path))
			continue
		}
		if err := storage.WriteAtomic(filepath.Join(configDir, native), data); err != nil {
			errs = append(errs, fmt.Errorf("put back %s: %w", f.Path, err))
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return os.RemoveAll(dir)
}

// recoverReplaced settles a rollback that died. Running the version the
// rollback was leaving means it never got as far as the swap: the files it had
// already overwritten go back. Running the version it was heading for means the
// swap happened and the copies are only waste.
func recoverReplaced(configDir, current string) error {
	dir, err := replacedPath(configDir)
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(dir, replacedJournal)); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			if _, derr := os.Stat(dir); derr == nil {
				return os.RemoveAll(dir)
			}
			return nil
		}
		return err
	}
	var rec replacedRecord
	if err := storage.Load(filepath.Join(dir, replacedJournal), replacedJournalVer, nil, &rec); err != nil {
		return fmt.Errorf("read the replaced files journal: %w", err)
	}
	if err := rec.validate(); err != nil {
		return fmt.Errorf("replaced files journal: %w", err)
	}
	switch {
	case sameVersion(current, rec.To):
		slog.Info("selfupdate: the interrupted rollback had finished, dropping the kept copies")
		return os.RemoveAll(dir)
	case sameVersion(current, rec.From):
		slog.Warn("selfupdate: a rollback died before the swap, putting the overwritten state files back", "files", len(rec.Files))
		return undoReplaced(configDir)
	}
	slog.Warn("selfupdate: kept copies of an interrupted rollback belong to other versions, dropping them", "from", rec.From, "to", rec.To, "running", current)
	return os.RemoveAll(dir)
}
