package metadata

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"typhon/internal/storage"
)

const (
	assetsVersion     = 1
	assetsFileName    = "media_assets.json"
	mediaDirName      = "media"
	gamesDirName      = "games"
	candidatesDirName = "candidates"
)

var errStorePath = errors.New("каталог медиаданных недоступен")

type assetStore struct {
	mu     sync.Mutex
	path   string
	root   string
	assets []MediaAsset
}

func newAssetStore(dir string) (*assetStore, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, errStorePath
	}
	s := &assetStore{
		path: filepath.Join(dir, assetsFileName),
		root: filepath.Join(dir, mediaDirName),
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *assetStore) load() error {
	var stored []MediaAsset
	err := storage.Load(s.path, assetsVersion, nil, &stored)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load media assets: %w", err)
	}
	s.assets = stored
	return nil
}

func (s *assetStore) mediaRoot() string {
	return s.root
}

func (s *assetStore) candidatesRoot() string {
	return filepath.Join(s.root, candidatesDirName)
}

func (s *assetStore) list(gameID string) []MediaAsset {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.listLocked(gameID)
}

func (s *assetStore) listLocked(gameID string) []MediaAsset {
	var out []MediaAsset
	for _, a := range s.assets {
		if a.GameID != gameID {
			continue
		}
		out = append(out, withURL(a))
	}
	return out
}

func (s *assetStore) find(assetID string) (MediaAsset, bool) {
	if assetID == "" {
		return MediaAsset{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, a := range s.assets {
		if a.ID == assetID {
			return withURL(a), true
		}
	}
	return MediaAsset{}, false
}

func (s *assetStore) replace(gameID string, next []MediaAsset) ([]MediaAsset, error) {
	if gameID == "" {
		return nil, errStorePath
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	previous := s.listLocked(gameID)
	before := s.assets
	kept := make([]MediaAsset, 0, len(s.assets)+len(next))
	for _, a := range s.assets {
		if a.GameID == gameID {
			continue
		}
		kept = append(kept, a)
	}
	for _, a := range next {
		a.URL = ""
		kept = append(kept, a)
	}
	s.assets = kept
	if err := storage.Save(s.path, assetsVersion, s.assets); err != nil {
		s.assets = before
		return nil, fmt.Errorf("save media assets: %w", err)
	}
	return previous, nil
}

func (s *assetStore) removeFiles(assets []MediaAsset) {
	for _, a := range assets {
		full, err := assetPath(s.root, a.Path)
		if err != nil {
			slog.Warn("resolve media asset path", "asset", a.ID, "path", a.Path, "error", err)
			continue
		}
		if err := os.Remove(full); err != nil && !errors.Is(err, fs.ErrNotExist) {
			slog.Warn("remove media asset", "asset", a.ID, "path", full, "error", err)
		}
	}
}

func (s *assetStore) sweep(ctx context.Context) error {
	root := filepath.Join(s.root, gamesDirName)
	if _, err := os.Stat(root); errors.Is(err, fs.ErrNotExist) {
		return nil
	} else if err != nil {
		return fmt.Errorf("stat media root: %w", err)
	}

	known := s.knownPaths()
	var orphans []string
	walkErr := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(s.root, p)
		if err != nil {
			return err
		}
		if known[filepath.ToSlash(rel)] {
			return nil
		}
		orphans = append(orphans, p)
		return nil
	})
	if walkErr != nil {
		return fmt.Errorf("scan media root: %w", walkErr)
	}
	for _, p := range orphans {
		if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("remove orphan media file %s: %w", p, err)
		}
	}
	return nil
}

func (s *assetStore) knownPaths() map[string]bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	known := make(map[string]bool, len(s.assets))
	for _, a := range s.assets {
		known[path.Clean(a.Path)] = true
	}
	return known
}

func (s *assetStore) clearCandidates() error {
	if err := os.RemoveAll(s.candidatesRoot()); err != nil {
		return fmt.Errorf("clear candidate cache: %w", err)
	}
	return nil
}

func withURL(a MediaAsset) MediaAsset {
	a.URL = "/" + path.Join(mediaDirName, a.Path)
	return a
}

func newAssetID() (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate asset id: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
