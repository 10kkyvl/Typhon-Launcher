package download

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

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

func (s *store) load() []record {
	if s.dir == "" {
		return nil
	}
	data, err := os.ReadFile(s.listPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		slog.Error("read downloads", "path", s.listPath(), "error", err)
		return nil
	}
	var records []record
	if err := json.Unmarshal(data, &records); err != nil {
		slog.Error("parse downloads", "path", s.listPath(), "error", err)
		return nil
	}
	return records
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
	return writeAtomic(s.listPath(), data)
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
	return writeAtomic(path, buf.Bytes())
}

func (s *store) loadMetainfo(infoHash string) (*metainfo.MetaInfo, error) {
	if s.dir == "" {
		return nil, errors.New("downloads path unavailable")
	}
	return metainfo.LoadFromFile(s.metainfoPath(infoHash))
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
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".torrent") {
			continue
		}
		if known[strings.ToLower(strings.TrimSuffix(name, ".torrent"))] {
			continue
		}
		if err := os.Remove(filepath.Join(dir, name)); err != nil {
			slog.Warn("remove orphaned torrent file", "name", name, "error", err)
		} else {
			slog.Info("removed orphaned torrent file", "name", name)
		}
	}
}

func (s *store) removeMetainfo(infoHash string) {
	if s.dir == "" {
		return
	}
	if err := os.Remove(s.metainfoPath(infoHash)); err != nil && !errors.Is(err, os.ErrNotExist) {
		slog.Warn("remove torrent file", "infoHash", infoHash, "error", err)
	}
}

func writeAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		slog.Error("write file", "path", tmp, "error", err)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		slog.Error("replace file", "path", path, "error", err)
		return err
	}
	return nil
}
