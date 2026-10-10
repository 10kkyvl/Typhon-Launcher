package download

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"typhon/internal/storage"

	"github.com/anacrolix/torrent/metainfo"
)

type record struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Type        Type       `json:"type"`
	Source      string     `json:"source"`
	InfoHash    string     `json:"infoHash"`
	Destination string     `json:"destination"`
	Status      Status     `json:"status"`
	Selected    []int      `json:"selected"`
	Downloaded  int64      `json:"downloaded"`
	Total       int64      `json:"total"`
	Seeding     bool       `json:"seeding"`
	Flat        bool       `json:"flat,omitempty"`
	InPlace     bool       `json:"inPlace,omitempty"`
	Root        string     `json:"root,omitempty"`
	Origin      Origin     `json:"origin,omitempty"`
	AddedAt     time.Time  `json:"addedAt"`
	CompletedAt *time.Time `json:"completedAt"`
	Error       string     `json:"error"`
}

type store struct {
	dir string
}

func newStore(dir string) *store {
	return &store{dir: dir}
}

func (s *store) listPath() string {
	return filepath.Join(s.dir, "downloads.json")
}

func (s *store) metainfoPath(infoHash string) string {
	return filepath.Join(s.dir, "torrents", infoHash+".torrent")
}

func (s *store) load() ([]record, error) {
	if s.dir == "" {
		return nil, errors.New("downloads path unavailable")
	}
	data, err := os.ReadFile(s.listPath())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read downloads %s: %w", s.listPath(), err)
	}
	var records []record
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, fmt.Errorf("parse downloads %s: %w", s.listPath(), err)
	}
	return records, nil
}

func (s *store) save(records []record) error {
	if s.dir == "" {
		return errors.New("downloads path unavailable")
	}
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return err
	}
	return storage.WriteAtomic(s.listPath(), data)
}

func (s *store) saveMetainfo(infoHash string, mi *metainfo.MetaInfo) error {
	if s.dir == "" {
		return errors.New("downloads path unavailable")
	}
	path := s.metainfoPath(infoHash)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := mi.Write(&buf); err != nil {
		return err
	}
	return storage.WriteAtomic(path, buf.Bytes())
}

func (s *store) loadMetainfo(infoHash string) (*metainfo.MetaInfo, error) {
	if s.dir == "" {
		return nil, errors.New("downloads path unavailable")
	}
	return loadMetainfoFile(s.metainfoPath(infoHash))
}

func (s *store) hasMetainfo(infoHash string) bool {
	if s.dir == "" {
		return false
	}
	info, err := os.Stat(s.metainfoPath(infoHash))
	return err == nil && !info.IsDir()
}

func (s *store) sweepMetainfo(known map[string]bool) {
	if s.dir == "" {
		return
	}
	dir := filepath.Join(s.dir, "torrents")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	removed := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".torrent") {
			continue
		}
		if known[strings.ToLower(strings.TrimSuffix(name, ".torrent"))] {
			continue
		}
		if err := os.Remove(filepath.Join(dir, name)); err != nil {
			slog.Warn("remove orphaned torrent file", "operation", "sweep_metainfo", "error", err)
			continue
		}
		removed++
	}
	if removed > 0 {
		slog.Info("removed orphaned torrent files", "operation", "sweep_metainfo", "count", removed)
	}
}

func (s *store) removeMetainfo(infoHash string) {
	if s.dir == "" {
		return
	}
	if err := os.Remove(s.metainfoPath(infoHash)); err != nil && !errors.Is(err, os.ErrNotExist) {
		slog.Warn("remove torrent file", "operation", "sweep_metainfo", "error", err)
	}
}

// maxMetainfoSize bounds every torrent file the launcher parses from disk, the
// ones a user points at and the cached ones alike. The engine takes at most
// 16 MiB of info from the swarm, so a real torrent with its trackers and web
// seeds stays far below this.
const maxMetainfoSize = 32 << 20

var errMetainfoTooLarge = errors.New("torrent-файл больше допустимого размера")

// loadMetainfoFile is the one way the package reads a torrent file. The
// decoder allocates what the file declares, so the size is capped before it
// sees a byte.
func loadMetainfoFile(path string) (*metainfo.MetaInfo, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	limited := &io.LimitedReader{R: f, N: maxMetainfoSize + 1}
	mi, loadErr := metainfo.Load(bufio.NewReader(limited))
	closeErr := f.Close()
	if limited.N <= 0 {
		return nil, fmt.Errorf("%s: %w", path, errMetainfoTooLarge)
	}
	if loadErr != nil {
		return nil, loadErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	return mi, nil
}
