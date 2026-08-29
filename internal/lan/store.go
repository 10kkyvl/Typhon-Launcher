package lan

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"typhon/internal/storage"
)

const shareStoreVersion = 1

var errEmptyConfigDir = errors.New("lan: config dir unavailable")

// shareStore persists the set of locally shared games (lanshares.json) and
// the .torrent files built for them (lan/<infohash>.torrent), both through
// storage's atomic writer. It is not safe for concurrent use; callers hold
// Service.mu while touching it.
type shareStore struct {
	path       string
	torrentDir string
	shares     map[string]Share
}

func newShareStore(configDir string) (*shareStore, error) {
	if configDir == "" {
		return nil, errEmptyConfigDir
	}
	return &shareStore{
		path:       filepath.Join(configDir, "lanshares.json"),
		torrentDir: filepath.Join(configDir, "lan"),
		shares:     map[string]Share{},
	}, nil
}

func (s *shareStore) load() error {
	var list []Share
	if err := storage.Load(s.path, shareStoreVersion, nil, &list); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			s.shares = map[string]Share{}
			return nil
		}
		return fmt.Errorf("load lan shares: %w", err)
	}
	shares := make(map[string]Share, len(list))
	for _, sh := range list {
		shares[sh.GameID] = sh
	}
	s.shares = shares
	return nil
}

func (s *shareStore) save() error {
	list := make([]Share, 0, len(s.shares))
	for _, sh := range s.shares {
		list = append(list, sh)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].GameID < list[j].GameID })
	if err := storage.Save(s.path, shareStoreVersion, list); err != nil {
		return fmt.Errorf("save lan shares: %w", err)
	}
	return nil
}

func (s *shareStore) put(sh Share) error {
	previous, had := s.shares[sh.GameID]
	s.shares[sh.GameID] = sh
	if err := s.save(); err != nil {
		if had {
			s.shares[sh.GameID] = previous
		} else {
			delete(s.shares, sh.GameID)
		}
		return err
	}
	return nil
}

func (s *shareStore) list() []Share {
	list := make([]Share, 0, len(s.shares))
	for _, sh := range s.shares {
		list = append(list, sh)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].GameID < list[j].GameID })
	return list
}

func (s *shareStore) torrentPath(infoHash string) string {
	return filepath.Join(s.torrentDir, infoHash+".torrent")
}

func (s *shareStore) writeTorrent(infoHash string, data []byte) error {
	if err := os.MkdirAll(s.torrentDir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", s.torrentDir, err)
	}
	if err := storage.WriteAtomic(s.torrentPath(infoHash), data); err != nil {
		return fmt.Errorf("write torrent %s: %w", infoHash, err)
	}
	return nil
}

func (s *shareStore) readTorrent(infoHash string) ([]byte, error) {
	data, err := os.ReadFile(filepath.Clean(s.torrentPath(infoHash)))
	if err != nil {
		return nil, fmt.Errorf("read torrent %s: %w", infoHash, err)
	}
	return data, nil
}
