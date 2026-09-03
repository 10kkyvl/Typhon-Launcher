package download

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"typhon/internal/storage"

	"github.com/anacrolix/torrent/metainfo"
	tstorage "github.com/anacrolix/torrent/storage"
)

const completionFileName = "piece-completion.json"

// completionFlushInterval is a var so tests can shrink it instead of
// sleeping for it.
var completionFlushInterval = 2 * time.Second

// nonClosingCompletion wraps the manager's single piece completion db so
// that closing a per-torrent storage (every Cancel/Remove/DeleteData, plus a
// discarded ClientConfig on port retry) never closes the shared db out from
// under every other torrent still using it. Only Manager.ServiceShutdown
// closes the real completion, after the client is gone.
type nonClosingCompletion struct {
	tstorage.PieceCompletion
}

func (nonClosingCompletion) Close() error { return nil }

// pieceSet is a growable bitset, one bit per piece index.
type pieceSet struct {
	bits []byte
	n    int
}

func newPieceSet(n int) *pieceSet {
	if n < 0 {
		n = 0
	}
	return &pieceSet{bits: make([]byte, (n+7)/8), n: n}
}

func (p *pieceSet) ensure(n int) {
	if n <= p.n {
		return
	}
	need := (n + 7) / 8
	if need > len(p.bits) {
		grown := make([]byte, need)
		copy(grown, p.bits)
		p.bits = grown
	}
	p.n = n
}

func (p *pieceSet) get(i int) bool {
	if i < 0 || i >= p.n {
		return false
	}
	return p.bits[i/8]&(1<<uint(i%8)) != 0
}

func (p *pieceSet) set(i int, v bool) {
	p.ensure(i + 1)
	if v {
		p.bits[i/8] |= 1 << uint(i%8)
	} else {
		p.bits[i/8] &^= 1 << uint(i%8)
	}
}

type completionEntry struct {
	Pieces string `json:"pieces"`
	Count  int    `json:"count"`
}

type completionFile struct {
	Version  int                               `json:"version"`
	Torrents map[metainfo.Hash]completionEntry `json:"torrents"`
}

// fileCompletion is a bbolt-free storage.PieceCompletion: a bitset per
// infohash held in memory and flushed to a single JSON file through
// storage.WriteAtomic. bbolt's file lock made a fresh db fail to open on the
// Windows CI runner (timeout acquiring the lock on a file nothing else had
// open); a plain atomically-written file never holds a handle open between
// writes, so a second manager on the same dir cannot be blocked by the first.
type fileCompletion struct {
	path string

	mu       sync.Mutex
	torrents map[metainfo.Hash]*pieceSet
	dirty    bool
	lastErr  error

	stop     chan struct{}
	stopOnce sync.Once
	done     chan struct{}
}

var _ tstorage.PieceCompletion = (*fileCompletion)(nil)

// openPieceCompletion opens the launcher's own file-backed piece completion
// db at <dir>/piece-completion.json. A missing file starts empty; a file
// that fails to parse is renamed aside (once) so a fresh one can be created
// instead of refusing to start. The one-time write below both creates the
// file and probes that dir is actually writable, so an unwritable dir still
// surfaces as a constructor error.
func openPieceCompletion(dir string) (tstorage.PieceCompletion, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("create piece completion dir: %w", err)
	}
	fc := &fileCompletion{
		path:     filepath.Join(dir, completionFileName),
		torrents: map[metainfo.Hash]*pieceSet{},
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
	if err := fc.load(); err != nil {
		return nil, fmt.Errorf("open piece completion db: %w", err)
	}
	fc.dirty = true
	if err := fc.persist(); err != nil {
		return nil, fmt.Errorf("open piece completion db: %w", err)
	}
	go fc.flushLoop()
	return fc, nil
}

func (fc *fileCompletion) load() error {
	data, err := os.ReadFile(fc.path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read piece completion db: %w", err)
	}
	torrents, decodeErr := decodeCompletionFile(data)
	if decodeErr != nil {
		return fc.quarantine(decodeErr)
	}
	fc.torrents = torrents
	return nil
}

func decodeCompletionFile(data []byte) (map[metainfo.Hash]*pieceSet, error) {
	var doc completionFile
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	torrents := make(map[metainfo.Hash]*pieceSet, len(doc.Torrents))
	for hash, entry := range doc.Torrents {
		bits, err := base64.StdEncoding.DecodeString(entry.Pieces)
		if err != nil {
			return nil, fmt.Errorf("decode pieces for %s: %w", hash.HexString(), err)
		}
		torrents[hash] = &pieceSet{bits: bits, n: entry.Count}
	}
	return torrents, nil
}

func (fc *fileCompletion) quarantine(cause error) error {
	broken := fmt.Sprintf("%s.broken-%d", fc.path, time.Now().Unix())
	if err := os.Rename(fc.path, broken); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("move corrupt piece completion db aside: %w", err)
	}
	slog.Warn("corrupt piece completion db moved aside", "path", fc.path, "broken", broken, "error", cause)
	fc.torrents = map[metainfo.Hash]*pieceSet{}
	return nil
}

func (fc *fileCompletion) snapshotLocked() completionFile {
	doc := completionFile{Version: 1, Torrents: make(map[metainfo.Hash]completionEntry, len(fc.torrents))}
	for hash, ps := range fc.torrents {
		doc.Torrents[hash] = completionEntry{
			Pieces: base64.StdEncoding.EncodeToString(ps.bits),
			Count:  ps.n,
		}
	}
	return doc
}

// persist writes the current state if it is dirty. dirty is cleared before
// the lock is released, so a Set arriving mid-write re-dirties it instead of
// being clobbered by the unconditional clear a write-then-clear order would
// need; a failed write puts dirty back so the next tick retries.
func (fc *fileCompletion) persist() error {
	fc.mu.Lock()
	if !fc.dirty {
		fc.mu.Unlock()
		return nil
	}
	fc.dirty = false
	doc := fc.snapshotLocked()
	fc.mu.Unlock()

	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		fc.mu.Lock()
		fc.dirty = true
		fc.mu.Unlock()
		return fmt.Errorf("marshal piece completion db: %w", err)
	}
	if err := storage.WriteAtomic(fc.path, data); err != nil {
		fc.mu.Lock()
		fc.dirty = true
		fc.mu.Unlock()
		return fmt.Errorf("write piece completion db: %w", err)
	}
	return nil
}

func (fc *fileCompletion) flushLoop() {
	defer close(fc.done)
	ticker := time.NewTicker(completionFlushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-fc.stop:
			return
		case <-ticker.C:
			if err := fc.persist(); err != nil {
				slog.Warn("flush piece completion db", "error", err)
				fc.mu.Lock()
				fc.lastErr = err
				fc.mu.Unlock()
			}
		}
	}
}

func (fc *fileCompletion) takeErrLocked() error {
	err := fc.lastErr
	fc.lastErr = nil
	return err
}

func (fc *fileCompletion) Get(pk metainfo.PieceKey) (tstorage.Completion, error) {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	ps := fc.torrents[pk.InfoHash]
	if ps == nil || pk.Index < 0 || pk.Index >= ps.n {
		return tstorage.Completion{}, nil
	}
	return tstorage.Completion{Ok: true, Complete: ps.get(pk.Index)}, nil
}

func (fc *fileCompletion) Set(pk metainfo.PieceKey, complete bool) error {
	fc.mu.Lock()
	ps := fc.torrents[pk.InfoHash]
	if ps == nil {
		ps = newPieceSet(pk.Index + 1)
		fc.torrents[pk.InfoHash] = ps
	}
	ps.set(pk.Index, complete)
	fc.dirty = true
	err := fc.takeErrLocked()
	fc.mu.Unlock()
	return err
}

func (fc *fileCompletion) Persistent() bool { return true }

func (fc *fileCompletion) Close() error {
	fc.stopOnce.Do(func() { close(fc.stop) })
	<-fc.done
	if err := fc.persist(); err != nil {
		fc.mu.Lock()
		fc.lastErr = err
		fc.mu.Unlock()
	}
	fc.mu.Lock()
	defer fc.mu.Unlock()
	return fc.takeErrLocked()
}
