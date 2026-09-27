//go:build devmock && !windows

package devmock

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"

	"typhon/internal/settings"
	"typhon/internal/storage"
)

const (
	stateFileName      = "devmock-processes.json"
	startPID           = 100000
	gameSecondsEnv     = "TYPHON_DEVMOCK_GAME_SECONDS"
	defaultGameSeconds = 60
	stateDirEnv        = "TYPHON_DEVMOCK_STATE_DIR"
)

// StateDirEnv lets other packages' tests point the default registry at an
// isolated directory (t.Setenv) without a test-only API on this package.
const StateDirEnv = stateDirEnv

var ErrNotRunning = errors.New("devmock: process not running")

type Entry struct {
	PID       uint32    `json:"pid"`
	Path      string    `json:"path"`
	Args      []string  `json:"args"`
	Dir       string    `json:"dir"`
	CreatedAt time.Time `json:"createdAt"`
	EndsAt    time.Time `json:"endsAt"`
}

type stateFile struct {
	NextPID uint32  `json:"nextPid"`
	Entries []Entry `json:"entries"`
}

type entryRecord struct {
	Entry
	done  chan struct{}
	timer *time.Timer
}

type registry struct {
	mu      sync.Mutex
	pathFn  func() (string, error)
	now     func() time.Time
	nextPID uint32
	entries map[uint32]*entryRecord
}

func newRegistry(pathFn func() (string, error), now func() time.Time) *registry {
	return &registry{pathFn: pathFn, now: now}
}

type Process struct {
	reg  *registry
	pid  uint32
	path string
}

func (p *Process) PID() uint32 { return p.pid }

func (p *Process) Path() string { return p.path }

func (p *Process) Wait() error { return p.reg.wait(p.pid) }

func (p *Process) Kill() error { return p.reg.kill(p.pid) }

func (r *registry) loadStateFile(path string) (stateFile, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return stateFile{NextPID: startPID}, nil
	}
	if err != nil {
		return stateFile{}, fmt.Errorf("devmock: read state %s: %w", path, err)
	}
	var sf stateFile
	if err := json.Unmarshal(data, &sf); err != nil {
		return stateFile{}, fmt.Errorf("devmock: parse state %s: %w", path, err)
	}
	if sf.NextPID < startPID {
		sf.NextPID = startPID
	}
	return sf, nil
}

// loadAndPruneLocked reads the state file and merges it with this
// registry's own owned entries so that a launcher restarted with the same
// state directory still sees fake processes another launcher started, and
// so that two registries sharing a directory (as happens across launcher
// restarts, and in tests) stay in sync. In-memory wins per PID because it
// reflects exactly what this instance persisted for its own processes.
// Anything whose EndsAt is no longer in the future is dropped from both the
// returned set and, if this instance owns it, from its own live table.
func (r *registry) loadAndPruneLocked(path string, now time.Time) (live map[uint32]Entry, nextPID uint32, changed bool, err error) {
	sf, err := r.loadStateFile(path)
	if err != nil {
		return nil, 0, false, err
	}

	onDisk := make(map[uint32]struct{}, len(sf.Entries))
	live = make(map[uint32]Entry, len(sf.Entries)+len(r.entries))
	for _, e := range sf.Entries {
		onDisk[e.PID] = struct{}{}
		live[e.PID] = e
	}
	for pid, rec := range r.entries {
		if _, ok := onDisk[pid]; !ok {
			changed = true
		}
		live[pid] = rec.Entry
	}

	for pid, e := range live {
		if !e.EndsAt.After(now) {
			delete(live, pid)
			r.dropOwnedLocked(pid)
			changed = true
		}
	}

	nextPID = sf.NextPID
	if r.nextPID > nextPID {
		nextPID = r.nextPID
	}
	if nextPID != sf.NextPID {
		changed = true
	}

	return live, nextPID, changed, nil
}

func (r *registry) dropOwnedLocked(pid uint32) {
	rec, ok := r.entries[pid]
	if !ok {
		return
	}
	if rec.timer != nil {
		rec.timer.Stop()
	}
	if rec.done != nil {
		close(rec.done)
	}
	delete(r.entries, pid)
}

func entriesSlice(m map[uint32]Entry) []Entry {
	out := make([]Entry, 0, len(m))
	for _, e := range m {
		out = append(out, e)
	}
	return out
}

func (r *registry) persistLocked(path string, nextPID uint32, entries []Entry) error {
	sort.Slice(entries, func(i, j int) bool { return entries[i].PID < entries[j].PID })
	data, err := json.MarshalIndent(stateFile{NextPID: nextPID, Entries: entries}, "", "  ")
	if err != nil {
		return fmt.Errorf("devmock: encode state: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("devmock: create state dir: %w", err)
	}
	if err := storage.WriteAtomic(path, data); err != nil {
		return fmt.Errorf("devmock: persist state: %w", err)
	}
	return nil
}

func (r *registry) start(execPath string, args []string, dir string, lifetime time.Duration) (*Process, error) {
	if execPath == "" {
		return nil, errors.New("devmock: empty executable path")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	path, err := r.pathFn()
	if err != nil {
		return nil, err
	}
	now := r.now()
	live, nextPID, _, err := r.loadAndPruneLocked(path, now)
	if err != nil {
		return nil, err
	}

	pid := nextPID
	entry := Entry{
		PID:       pid,
		Path:      execPath,
		Args:      append([]string(nil), args...),
		Dir:       dir,
		CreatedAt: now,
		EndsAt:    now.Add(lifetime),
	}
	live[pid] = entry

	if err := r.persistLocked(path, pid+1, entriesSlice(live)); err != nil {
		return nil, err
	}

	if r.entries == nil {
		r.entries = map[uint32]*entryRecord{}
	}
	rec := &entryRecord{Entry: entry, done: make(chan struct{})}
	rec.timer = time.AfterFunc(lifetime, func() { r.expire(pid) })
	r.entries[pid] = rec
	r.nextPID = pid + 1

	return &Process{reg: r, pid: pid, path: execPath}, nil
}

func (r *registry) expire(pid uint32) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.dropOwnedLocked(pid)
}

func (r *registry) kill(pid uint32) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	rec, ok := r.entries[pid]
	if !ok || rec.done == nil {
		return ErrNotRunning
	}

	path, err := r.pathFn()
	if err != nil {
		return err
	}
	now := r.now()
	live, nextPID, _, err := r.loadAndPruneLocked(path, now)
	if err != nil {
		return err
	}
	if _, stillOwned := r.entries[pid]; !stillOwned {
		return ErrNotRunning
	}
	delete(live, pid)

	if err := r.persistLocked(path, nextPID, entriesSlice(live)); err != nil {
		return err
	}

	r.nextPID = nextPID
	rec.timer.Stop()
	delete(r.entries, pid)
	close(rec.done)
	return nil
}

func (r *registry) wait(pid uint32) error {
	r.mu.Lock()
	rec, ok := r.entries[pid]
	if !ok || rec.done == nil {
		r.mu.Unlock()
		return nil
	}
	done := rec.done
	r.mu.Unlock()
	<-done
	return nil
}

func (r *registry) list() ([]Entry, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	path, err := r.pathFn()
	if err != nil {
		return nil, err
	}
	now := r.now()
	live, nextPID, changed, err := r.loadAndPruneLocked(path, now)
	if err != nil {
		return nil, err
	}
	r.nextPID = nextPID

	if changed {
		if err := r.persistLocked(path, nextPID, entriesSlice(live)); err != nil {
			return nil, err
		}
	}

	out := entriesSlice(live)
	sort.Slice(out, func(i, j int) bool { return out[i].PID < out[j].PID })
	return out, nil
}

func gameLifetime() (time.Duration, error) {
	raw, ok := os.LookupEnv(gameSecondsEnv)
	if !ok {
		return defaultGameSeconds * time.Second, nil
	}
	seconds, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("devmock: invalid %s %q: %w", gameSecondsEnv, raw, err)
	}
	if seconds <= 0 {
		return 0, fmt.Errorf("devmock: %s must be positive, got %d", gameSecondsEnv, seconds)
	}
	return time.Duration(seconds) * time.Second, nil
}

func defaultStateDir() (string, error) {
	if dir, ok := os.LookupEnv(stateDirEnv); ok {
		if dir == "" {
			return "", fmt.Errorf("devmock: %s must not be empty", stateDirEnv)
		}
		if !filepath.IsAbs(dir) {
			return "", fmt.Errorf("devmock: %s must be an absolute path, got %q", stateDirEnv, dir)
		}
		return dir, nil
	}
	return settings.ConfigDir()
}

var stateDir = defaultStateDir

func defaultPath() (string, error) {
	dir, err := stateDir()
	if err != nil {
		return "", fmt.Errorf("devmock: resolve state dir: %w", err)
	}
	return filepath.Join(dir, stateFileName), nil
}

// StateDir resolves the directory devmock keeps its fake filesystem state
// in, creating it if absent, so other mocks (the install runner's fake
// system32) can place files alongside the process table.
func StateDir() (string, error) {
	dir, err := stateDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("devmock: create state dir %s: %w", dir, err)
	}
	return dir, nil
}

var (
	defaultRegOnce sync.Once
	defaultReg     *registry
)

func getDefaultRegistry() *registry {
	defaultRegOnce.Do(func() {
		defaultReg = newRegistry(defaultPath, time.Now)
	})
	return defaultReg
}

func Start(path string, args []string, dir string) (*Process, error) {
	lifetime, err := gameLifetime()
	if err != nil {
		return nil, err
	}
	return getDefaultRegistry().start(path, args, dir, lifetime)
}

func List() ([]Entry, error) {
	return getDefaultRegistry().list()
}
