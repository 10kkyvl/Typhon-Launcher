package wrappers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"typhon/internal/library"
	"typhon/internal/storage"
	"typhon/internal/uierr"
)

const (
	ProgressEvent = "wrappers:progress"

	codeRecoveryFailed = "wrappers.recovery_failed"

	stateVersion  = 1
	phaseApplying = "applying"
	phaseApplied  = "applied"
	phaseRemoving = "removing"
)

type Games interface {
	Find(id string) (library.Game, error)
	IsRunning(id string) bool
}

type Applied struct {
	Wrapper   string    `json:"wrapper"`
	Version   string    `json:"version"`
	Preset    string    `json:"preset"`
	Files     []string  `json:"files"`
	AppliedAt time.Time `json:"appliedAt"`
}

type Info struct {
	Supported bool     `json:"supported"`
	Arch      string   `json:"arch"`
	Imports   []string `json:"imports"`
	Suggested string   `json:"suggested"`
	Active    *Applied `json:"active"`
	Problem   *Problem `json:"problem,omitempty"`
}

// Problem is an interrupted operation that could not be finished or rolled
// back; the entry stays in the journal until Disable succeeds.
type Problem struct {
	Code string `json:"code"`
	File string `json:"file,omitempty"`
}

type fileError struct {
	file string
	err  error
}

func (e *fileError) Error() string { return e.err.Error() + ": " + e.file }
func (e *fileError) Unwrap() error { return e.err }

func fileErr(file string, err error) error { return &fileError{file: file, err: err} }

func problemOf(err error) *Problem {
	p := &Problem{Code: uierr.Code(err)}
	if p.Code == "" {
		p.Code = codeRecoveryFailed
	}
	var fe *fileError
	if errors.As(err, &fe) {
		p.File = fe.file
	}
	return p
}

type Progress struct {
	GameID string `json:"gameId"`
	Stage  string `json:"stage"`
	Done   int64  `json:"done"`
	Total  int64  `json:"total"`
}

type fileRec struct {
	Name    string `json:"name"`
	SHA     string `json:"sha"`
	Orig    bool   `json:"orig"`
	OrigSHA string `json:"origSha,omitempty"`
}

type entry struct {
	Dir       string    `json:"dir"`
	Wrapper   string    `json:"wrapper"`
	Version   string    `json:"version"`
	Preset    string    `json:"preset"`
	Phase     string    `json:"phase"`
	Problem   *Problem  `json:"problem,omitempty"`
	AppliedAt time.Time `json:"appliedAt"`
	Files     []fileRec `json:"files"`
}

type stateFile struct {
	Version int              `json:"version"`
	Entries map[string]entry `json:"entries"`
}

type payload struct {
	name string
	data []byte
	sha  string
}

type Service struct {
	root      string
	games     Games
	releases  map[string]release
	dl        *downloader
	supported bool
	emit      func(name string, data any)

	fetchMu sync.Mutex
	mu      sync.Mutex
	entries map[string]entry
}

func NewService(configDir string, games Games) (*Service, error) {
	return newService(configDir, games, productionReleases(), newDownloader(allowedURL), runtime.GOOS == "windows")
}

func newService(configDir string, games Games, releases map[string]release, dl *downloader, supported bool) (*Service, error) {
	if configDir == "" || !filepath.IsAbs(configDir) {
		return nil, fmt.Errorf("wrappers: config dir %q is not an absolute path", configDir)
	}
	if games == nil {
		return nil, errors.New("wrappers: games provider is required")
	}
	root := filepath.Join(configDir, "wrappers")
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("create %s: %w", root, err)
	}
	entries, err := loadState(filepath.Join(root, "state.json"))
	if err != nil {
		return nil, err
	}
	return &Service{
		root: root, games: games, releases: releases, dl: dl, supported: supported,
		emit: func(name string, data any) {
			if app := application.Get(); app != nil {
				app.Event.Emit(name, data)
			}
		},
		entries: entries,
	}, nil
}

func loadState(path string) (map[string]entry, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]entry{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrBadState, err)
	}
	var st stateFile
	if err := json.Unmarshal(raw, &st); err != nil {
		return nil, fmt.Errorf("%w: %s: %w", ErrBadState, path, err)
	}
	if st.Version != stateVersion {
		return nil, fmt.Errorf("%w: %s: version %d", ErrBadState, path, st.Version)
	}
	if st.Entries == nil {
		st.Entries = map[string]entry{}
	}
	return st.Entries, nil
}

func (s *Service) save(entries map[string]entry) error {
	raw, err := json.MarshalIndent(stateFile{Version: stateVersion, Entries: entries}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode wrappers state: %w", err)
	}
	return storage.WriteAtomic(filepath.Join(s.root, "state.json"), raw)
}

// commit persists the change first and only then publishes it to memory, so a
// failed write leaves memory identical to disk.
func (s *Service) commit(gameID string, e *entry) error {
	next := make(map[string]entry, len(s.entries)+1)
	for k, v := range s.entries {
		next[k] = v
	}
	if e == nil {
		delete(next, gameID)
	} else {
		next[gameID] = *e
	}
	if err := s.save(next); err != nil {
		return err
	}
	s.entries = next
	return nil
}

func (s *Service) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	if !s.supported {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := make([]string, 0, len(s.entries))
	for id, e := range s.entries {
		if e.Phase != phaseApplied {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	var errs []error
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return errors.Join(append(errs, err)...)
		}
		err := s.restore(id)
		if err == nil {
			continue
		}
		e, ok := s.entries[id]
		if !ok {
			errs = append(errs, fmt.Errorf("recover wrappers of %s: %w", id, err))
			continue
		}
		e.Problem = problemOf(err)
		slog.Error("wrappers: recovery of an interrupted operation failed", "game", id, "code", e.Problem.Code, "file", e.Problem.File)
		if cerr := s.commit(id, &e); cerr != nil {
			errs = append(errs, fmt.Errorf("record wrappers problem of %s: %w", id, errors.Join(cerr, err)))
		}
	}
	return errors.Join(errs...)
}

func validID(id string) error {
	if id == "" || len(id) > 128 || id == "." || id == ".." {
		return fmt.Errorf("%w: %q", ErrBadGameID, id)
	}
	for _, r := range id {
		ok := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.'
		if !ok {
			return fmt.Errorf("%w: %q", ErrBadGameID, id)
		}
	}
	return nil
}

func installPaths(g library.Game) (dir, exe string, err error) {
	if g.InstallDir == "" || g.Executable == "" {
		return "", "", ErrNoInstallDir
	}
	inst, exe := filepath.Clean(g.InstallDir), filepath.Clean(g.Executable)
	if !filepath.IsAbs(inst) || !filepath.IsAbs(exe) {
		return "", "", ErrNoInstallDir
	}
	rel, err := filepath.Rel(inst, exe)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", "", ErrOutsideDir
	}
	return filepath.Dir(exe), exe, nil
}

func (s *Service) Inspect(ctx context.Context, gameID string) (Info, error) {
	if err := ctx.Err(); err != nil {
		return Info{}, err
	}
	if !s.supported {
		return Info{Imports: []string{}}, nil
	}
	if err := validID(gameID); err != nil {
		return Info{}, err
	}
	g, err := s.games.Find(gameID)
	if err != nil {
		return Info{}, fmt.Errorf("find game %s: %w", gameID, err)
	}
	_, exe, err := installPaths(g)
	if err != nil {
		return Info{}, err
	}
	info := Info{Imports: []string{}, Active: s.active(gameID), Problem: s.problem(gameID)}
	arch, libs, err := parseImports(exe)
	if errors.Is(err, ErrNotPE) {
		return info, nil
	}
	if err != nil {
		return Info{}, err
	}
	info.Supported = true
	info.Arch = arch
	info.Imports = libs
	if libs == nil {
		info.Imports = []string{}
	}
	info.Suggested = suggest(libs)
	return info, nil
}

func (s *Service) problem(gameID string) *Problem {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p := s.entries[gameID].Problem; p != nil {
		c := *p
		return &c
	}
	return nil
}

func (s *Service) active(gameID string) *Applied {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[gameID]
	if !ok || e.Phase != phaseApplied {
		return nil
	}
	names := make([]string, len(e.Files))
	for i, f := range e.Files {
		names[i] = f.Name
	}
	return &Applied{Wrapper: e.Wrapper, Version: e.Version, Preset: e.Preset, Files: names, AppliedAt: e.AppliedAt}
}

func (s *Service) Enable(ctx context.Context, gameID, wrapper, preset string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !s.supported {
		return ErrUnsupported
	}
	rel, ok := s.releases[wrapper]
	if !ok {
		return fmt.Errorf("%w: %q", ErrBadWrapper, wrapper)
	}
	if !rel.hasPreset(preset) {
		return fmt.Errorf("%w: %q for %s", ErrBadPreset, preset, wrapper)
	}
	if err := validID(gameID); err != nil {
		return err
	}
	g, err := s.games.Find(gameID)
	if err != nil {
		return fmt.Errorf("find game %s: %w", gameID, err)
	}
	dir, exe, err := installPaths(g)
	if err != nil {
		return err
	}
	if s.games.IsRunning(gameID) {
		return ErrRunning
	}
	arch, libs, err := parseImports(exe)
	if err != nil {
		return err
	}
	tg := targets(wrapper, arch, libs)
	if len(tg) == 0 {
		return fmt.Errorf("%w: %s on %s", ErrNoTarget, wrapper, arch)
	}
	cache, err := s.ensure(ctx, gameID, rel)
	if err != nil {
		return err
	}
	pls, err := buildPayloads(cache, preset, tg)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.games.IsRunning(gameID) {
		return ErrRunning
	}
	if busy, ok := s.entries[gameID]; ok {
		if busy.Problem != nil {
			return fmt.Errorf("%w: %s", ErrNeedsRepair, busy.Problem.Code)
		}
		return ErrActive
	}
	recs := make([]fileRec, len(pls))
	for i, p := range pls {
		exists, sha, err := probe(filepath.Join(dir, p.name))
		if err != nil {
			return fmt.Errorf("%s: %w", p.name, err)
		}
		recs[i] = fileRec{Name: p.name, SHA: p.sha, Orig: exists, OrigSHA: sha}
	}
	e := entry{Dir: dir, Wrapper: wrapper, Version: rel.Version, Preset: preset, Phase: phaseApplying, Files: recs}
	if err := s.commit(gameID, &e); err != nil {
		return err
	}
	if err := s.place(ctx, gameID, e, pls); err != nil {
		return errors.Join(err, s.restore(gameID))
	}
	e.Phase = phaseApplied
	e.AppliedAt = time.Now().UTC()
	if err := s.commit(gameID, &e); err != nil {
		return errors.Join(err, s.restore(gameID))
	}
	return nil
}

func buildPayloads(cache, preset string, tg []target) ([]payload, error) {
	out := make([]payload, 0, len(tg))
	for _, t := range tg {
		data, err := os.ReadFile(filepath.Join(cache, filepath.FromSlash(t.Src)))
		if err != nil {
			return nil, fmt.Errorf("read cached %s: %w", t.Src, err)
		}
		if t.Name == confName {
			if data, err = patchConf(data, preset); err != nil {
				return nil, err
			}
		}
		out = append(out, payload{name: t.Name, data: data, sha: hashBytes(data)})
	}
	return out, nil
}

func probe(p string) (exists bool, sha string, err error) {
	fi, err := os.Lstat(p)
	if errors.Is(err, fs.ErrNotExist) {
		return false, "", nil
	}
	if err != nil {
		return false, "", err
	}
	if !fi.Mode().IsRegular() {
		return false, "", ErrSymlink
	}
	sha, err = hashFile(p)
	if err != nil {
		return false, "", err
	}
	return true, sha, nil
}

func (s *Service) backupDir(gameID string) string {
	return filepath.Join(s.root, "backup", gameID)
}

func (s *Service) place(ctx context.Context, gameID string, e entry, pls []payload) error {
	bdir := s.backupDir(gameID)
	for _, f := range e.Files {
		if !f.Orig {
			continue
		}
		dst := filepath.Join(e.Dir, f.Name)
		exists, sha, err := probe(dst)
		if err != nil {
			return fmt.Errorf("%s: %w", f.Name, err)
		}
		if !exists || sha != f.OrigSHA {
			return fmt.Errorf("%w: %s", ErrTampered, f.Name)
		}
		data, err := os.ReadFile(dst)
		if err != nil {
			return fmt.Errorf("read %s: %w", dst, err)
		}
		if hashBytes(data) != f.OrigSHA {
			return fmt.Errorf("%w: %s", ErrTampered, f.Name)
		}
		if err := os.MkdirAll(bdir, 0o700); err != nil {
			return fmt.Errorf("create %s: %w", bdir, err)
		}
		if err := storage.WriteAtomic(filepath.Join(bdir, f.Name), data); err != nil {
			return err
		}
		if got, err := hashFile(filepath.Join(bdir, f.Name)); err != nil || got != f.OrigSHA {
			return errors.Join(fmt.Errorf("%w: %s", ErrBackupCorrupt, f.Name), err)
		}
	}
	for i, p := range pls {
		if err := ctx.Err(); err != nil {
			return err
		}
		s.emit(ProgressEvent, Progress{GameID: gameID, Stage: "apply", Done: int64(i), Total: int64(len(pls))})
		dst := filepath.Join(e.Dir, p.name)
		exists, sha, err := probe(dst)
		if err != nil {
			return fmt.Errorf("%s: %w", p.name, err)
		}
		f := e.Files[i]
		if exists != f.Orig || sha != f.OrigSHA {
			return fmt.Errorf("%w: %s", ErrTampered, p.name)
		}
		if err := storage.WriteAtomic(dst, p.data); err != nil {
			return err
		}
	}
	s.emit(ProgressEvent, Progress{GameID: gameID, Stage: "apply", Done: int64(len(pls)), Total: int64(len(pls))})
	return nil
}

func (s *Service) Disable(ctx context.Context, gameID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !s.supported {
		return ErrUnsupported
	}
	if err := validID(gameID); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.games.IsRunning(gameID) {
		return ErrRunning
	}
	if _, ok := s.entries[gameID]; !ok {
		return ErrNotActive
	}
	return s.restore(gameID)
}

// restore serves Disable, the rollback of a failed Enable and recovery at
// startup: from any phase it ends with the original files back and no entry.
// Every file is checked before the first one is touched, so a file the user
// replaced aborts the whole operation untouched.
func (s *Service) restore(gameID string) error {
	e, ok := s.entries[gameID]
	if !ok {
		return ErrNotActive
	}
	if _, err := os.Stat(e.Dir); errors.Is(err, fs.ErrNotExist) {
		return s.finish(gameID)
	} else if err != nil {
		return fmt.Errorf("stat %s: %w", e.Dir, err)
	}
	cur := make([]string, len(e.Files))
	for i, f := range e.Files {
		_, sha, err := probe(filepath.Join(e.Dir, f.Name))
		if err != nil {
			return fileErr(f.Name, err)
		}
		untouched := f.Orig && sha == f.OrigSHA
		if sha != "" && sha != f.SHA && !untouched {
			return fileErr(f.Name, ErrTampered)
		}
		cur[i] = sha
	}
	if e.Phase != phaseRemoving {
		e.Phase = phaseRemoving
		if err := s.commit(gameID, &e); err != nil {
			return err
		}
	}
	for i, f := range e.Files {
		dst := filepath.Join(e.Dir, f.Name)
		switch {
		case f.Orig && cur[i] == f.OrigSHA:
		case f.Orig:
			if err := s.putBack(gameID, dst, f); err != nil {
				return err
			}
		case cur[i] == f.SHA:
			if err := os.Remove(dst); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return fmt.Errorf("remove %s: %w", dst, err)
			}
		}
	}
	return s.finish(gameID)
}

func (s *Service) putBack(gameID, dst string, f fileRec) error {
	bak := filepath.Join(s.backupDir(gameID), f.Name)
	data, err := os.ReadFile(bak)
	if err != nil {
		return fmt.Errorf("read backup %s: %w", bak, err)
	}
	if hashBytes(data) != f.OrigSHA {
		return fileErr(f.Name, ErrBackupCorrupt)
	}
	if err := storage.WriteAtomic(dst, data); err != nil {
		return err
	}
	got, err := hashFile(dst)
	if err != nil {
		return fmt.Errorf("verify %s: %w", dst, err)
	}
	if got != f.OrigSHA {
		return fileErr(f.Name, ErrTampered)
	}
	return nil
}

func (s *Service) finish(gameID string) error {
	bdir := s.backupDir(gameID)
	if err := os.RemoveAll(bdir); err != nil {
		return fmt.Errorf("remove %s: %w", bdir, err)
	}
	return s.commit(gameID, nil)
}
