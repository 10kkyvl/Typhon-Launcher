package wrappers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

	tmpPrefix  = ".typhon-wrap-"
	tmpSuffix  = ".tmp"
	tmpPattern = tmpPrefix + "*" + tmpSuffix
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

// Problem is a wrapper that cannot be finished or removed on its own: an
// interrupted operation, a file changed by the user, an unavailable game
// directory. The entry stays in the journal until Disable succeeds.
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
	InstallDir string    `json:"installDir"`
	RelDir     string    `json:"relDir"`
	Wrapper    string    `json:"wrapper"`
	Version    string    `json:"version"`
	Preset     string    `json:"preset"`
	Phase      string    `json:"phase"`
	Problem    *Problem  `json:"problem,omitempty"`
	AppliedAt  time.Time `json:"appliedAt"`
	Files      []fileRec `json:"files"`
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

type restoreMode int

const (
	modeStrict restoreMode = iota
	modeKeepChanged
)

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
	busy    map[string]bool
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
		busy:    map[string]bool{},
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
	for id, e := range st.Entries {
		if err := validEntry(id, e); err != nil {
			return nil, fmt.Errorf("%w: %s: %w", ErrBadState, path, err)
		}
	}
	return st.Entries, nil
}

func validEntry(id string, e entry) error {
	if err := validID(id); err != nil {
		return err
	}
	if !filepath.IsAbs(e.InstallDir) || !filepath.IsLocal(e.RelDir) {
		return fmt.Errorf("entry %s has a bad install location", id)
	}
	for _, f := range e.Files {
		if f.Name == "" || filepath.Base(f.Name) != f.Name || f.Name == "." || f.Name == ".." {
			return fmt.Errorf("entry %s has a bad file name %q", id, f.Name)
		}
	}
	return nil
}

func (s *Service) save(entries map[string]entry) error {
	raw, err := json.MarshalIndent(stateFile{Version: stateVersion, Entries: entries}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode wrappers state: %w", err)
	}
	return storage.WriteAtomic(filepath.Join(s.root, "state.json"), raw)
}

func (s *Service) get(gameID string) (entry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[gameID]
	return e, ok
}

// commit persists the change first and only then publishes it to memory, so a
// failed write leaves memory identical to disk.
func (s *Service) commit(gameID string, e *entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
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

// begin claims a game for one operation; the work itself runs outside s.mu.
func (s *Service) begin(gameID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.busy[gameID] {
		return ErrBusy
	}
	s.busy[gameID] = true
	return nil
}

func (s *Service) end(gameID string) {
	s.mu.Lock()
	delete(s.busy, gameID)
	s.mu.Unlock()
}

func (s *Service) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	if !s.supported {
		return nil
	}
	s.mu.Lock()
	ids := make([]string, 0, len(s.entries))
	for id := range s.entries {
		ids = append(ids, id)
	}
	s.mu.Unlock()
	sort.Strings(ids)
	var errs []error
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return errors.Join(append(errs, err)...)
		}
		e, ok := s.get(id)
		if !ok {
			continue
		}
		if dir, err := s.resolve(id, &e); err == nil {
			cleanLeftovers(dir)
		}
		if e.Phase == phaseApplied {
			continue
		}
		err := s.restore(id, modeStrict)
		if err == nil || errors.Is(err, ErrGameDirMissing) {
			continue
		}
		if rec := s.recordProblem(id, err); rec != nil {
			errs = append(errs, fmt.Errorf("recover wrappers of %s: %w", id, errors.Join(rec, err)))
		}
	}
	return errors.Join(errs...)
}

// A leftover temp file is junk from an interrupted write; failing to delete it
// is worth a log line but must not stop the launcher.
func cleanLeftovers(dir string) {
	items, err := os.ReadDir(dir)
	if err != nil {
		slog.Warn("wrappers: scan for leftover temp files failed", "err", err)
		return
	}
	for _, it := range items {
		if !strings.HasPrefix(it.Name(), tmpPrefix) || !strings.HasSuffix(it.Name(), tmpSuffix) || !it.Type().IsRegular() {
			continue
		}
		if err := os.Remove(filepath.Join(dir, it.Name())); err != nil && !errors.Is(err, fs.ErrNotExist) {
			slog.Warn("wrappers: remove leftover temp file failed", "name", it.Name(), "err", err)
		}
	}
}

// recordProblem keeps a failed operation visible: the entry stays in the
// journal with the reason, so the UI can offer Disable instead of the wrapper
// silently disappearing from view.
func (s *Service) recordProblem(gameID string, cause error) error {
	e, ok := s.get(gameID)
	if !ok {
		return nil
	}
	e.Problem = problemOf(cause)
	slog.Error("wrappers: operation failed", "game", gameID, "code", e.Problem.Code, "file", e.Problem.File)
	return s.commit(gameID, &e)
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

func installPaths(g library.Game) (inst, dir, exe string, err error) {
	if g.InstallDir == "" || g.Executable == "" {
		return "", "", "", ErrNoInstallDir
	}
	inst, exe = filepath.Clean(g.InstallDir), filepath.Clean(g.Executable)
	if !filepath.IsAbs(inst) || !filepath.IsAbs(exe) {
		return "", "", "", ErrNoInstallDir
	}
	rel, err := filepath.Rel(inst, exe)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", "", "", ErrOutsideDir
	}
	return inst, filepath.Dir(exe), exe, nil
}

var errGameGone = errors.New("game is gone")

// resolve finds where the wrapper files are now. The library is asked first:
// a relocated game takes its wrapper along, so the current install directory
// wins over the recorded one. A missing directory is only "the game is gone"
// when the library no longer lists it; otherwise the disk may just be away.
func (s *Service) resolve(gameID string, e *entry) (string, error) {
	inst := e.InstallDir
	g, findErr := s.games.Find(gameID)
	inLibrary := findErr == nil
	if inLibrary && g.InstallDir != "" {
		if cur := filepath.Clean(g.InstallDir); filepath.IsAbs(cur) {
			inst = cur
		}
	}
	dir := filepath.Join(inst, e.RelDir)
	_, err := os.Stat(dir)
	switch {
	case err == nil:
		e.InstallDir = inst
		return dir, nil
	case errors.Is(err, fs.ErrNotExist):
		if !inLibrary {
			return "", errGameGone
		}
		return "", ErrGameDirMissing
	}
	return "", fmt.Errorf("stat %s: %w", dir, err)
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
	_, _, exe, err := installPaths(g)
	if err != nil {
		return Info{}, err
	}
	info := Info{Imports: []string{}, Active: s.active(gameID), Problem: s.problem(gameID)}
	arch, libs, err := parseImports(exe)
	if errors.Is(err, ErrNotPE) || (errors.Is(err, fs.ErrNotExist) && info.Problem != nil) {
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
	e, ok := s.get(gameID)
	if !ok {
		return nil
	}
	if _, err := s.resolve(gameID, &e); errors.Is(err, ErrGameDirMissing) {
		return &Problem{Code: uierr.Code(ErrGameDirMissing)}
	}
	if e.Problem != nil {
		c := *e.Problem
		return &c
	}
	if e.Phase != phaseApplied {
		return &Problem{Code: codeRecoveryFailed}
	}
	return nil
}

func (s *Service) active(gameID string) *Applied {
	e, ok := s.get(gameID)
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
	inst, dir, exe, err := installPaths(g)
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
	if err := s.begin(gameID); err != nil {
		return err
	}
	defer s.end(gameID)
	if cur, exists := s.get(gameID); exists {
		if cur.Problem != nil || cur.Phase != phaseApplied {
			return ErrNeedsRepair
		}
		return ErrActive
	}
	cache, err := s.ensure(ctx, gameID, rel)
	if err != nil {
		return err
	}
	pls, err := buildPayloads(cache, preset, tg)
	if err != nil {
		return err
	}
	if s.games.IsRunning(gameID) {
		return ErrRunning
	}
	relDir, err := filepath.Rel(inst, dir)
	if err != nil {
		return fmt.Errorf("locate %s in %s: %w", dir, inst, err)
	}
	recs := make([]fileRec, len(pls))
	for i, p := range pls {
		exists, sha, err := probe(filepath.Join(dir, p.name))
		if err != nil {
			return fmt.Errorf("%s: %w", p.name, err)
		}
		recs[i] = fileRec{Name: p.name, SHA: p.sha, Orig: exists, OrigSHA: sha}
	}
	e := entry{InstallDir: inst, RelDir: relDir, Wrapper: wrapper, Version: rel.Version, Preset: preset, Phase: phaseApplying, Files: recs}
	if err := s.commit(gameID, &e); err != nil {
		return err
	}
	if err := s.place(ctx, gameID, dir, e, pls); err != nil {
		return errors.Join(err, s.rollback(gameID))
	}
	e.Phase = phaseApplied
	e.AppliedAt = time.Now().UTC()
	if err := s.commit(gameID, &e); err != nil {
		return errors.Join(err, s.rollback(gameID))
	}
	return nil
}

func (s *Service) rollback(gameID string) error {
	err := s.restore(gameID, modeStrict)
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrGameDirMissing) {
		return err
	}
	return errors.Join(err, s.recordProblem(gameID, err))
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

// writeInto replaces a file in the game directory with a temp name this
// package owns, so leftovers of a killed process can be told apart and swept.
func writeInto(path string, data []byte) error {
	mode := fs.FileMode(0o644)
	if fi, err := os.Lstat(path); err == nil {
		mode = fi.Mode().Perm()
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return storage.WriteAtomicStreamTemp(path, mode, tmpPattern, func(w io.Writer) error {
		_, err := w.Write(data)
		return err
	})
}

func (s *Service) backupDir(gameID string) string {
	return filepath.Join(s.root, "backup", gameID)
}

func (s *Service) place(ctx context.Context, gameID, dir string, e entry, pls []payload) error {
	bdir := s.backupDir(gameID)
	for _, f := range e.Files {
		if !f.Orig {
			continue
		}
		dst := filepath.Join(dir, f.Name)
		exists, sha, err := probe(dst)
		if err != nil {
			return fmt.Errorf("%s: %w", f.Name, err)
		}
		if !exists || sha != f.OrigSHA {
			return fileErr(f.Name, ErrTampered)
		}
		data, err := os.ReadFile(dst)
		if err != nil {
			return fmt.Errorf("read %s: %w", dst, err)
		}
		if hashBytes(data) != f.OrigSHA {
			return fileErr(f.Name, ErrTampered)
		}
		if err := os.MkdirAll(bdir, 0o700); err != nil {
			return fmt.Errorf("create %s: %w", bdir, err)
		}
		if err := storage.WriteAtomic(filepath.Join(bdir, f.Name), data); err != nil {
			return err
		}
		if got, err := hashFile(filepath.Join(bdir, f.Name)); err != nil || got != f.OrigSHA {
			return errors.Join(fileErr(f.Name, ErrBackupCorrupt), err)
		}
	}
	for i, p := range pls {
		if err := ctx.Err(); err != nil {
			return err
		}
		s.emit(ProgressEvent, Progress{GameID: gameID, Stage: "apply", Done: int64(i), Total: int64(len(pls))})
		dst := filepath.Join(dir, p.name)
		exists, sha, err := probe(dst)
		if err != nil {
			return fmt.Errorf("%s: %w", p.name, err)
		}
		f := e.Files[i]
		if exists != f.Orig || sha != f.OrigSHA {
			return fileErr(p.name, ErrTampered)
		}
		if err := writeInto(dst, p.data); err != nil {
			return err
		}
	}
	s.emit(ProgressEvent, Progress{GameID: gameID, Stage: "apply", Done: int64(len(pls)), Total: int64(len(pls))})
	return nil
}

// Disable refuses when a file other than the wrapper's config was changed
// after the wrapper was installed.
func (s *Service) Disable(ctx context.Context, gameID string) error {
	return s.disable(ctx, gameID, modeStrict)
}

// DisableKeepChanged removes the wrapper but leaves files that were changed
// after it was installed (a game update or repair replaced them) untouched.
func (s *Service) DisableKeepChanged(ctx context.Context, gameID string) error {
	return s.disable(ctx, gameID, modeKeepChanged)
}

func (s *Service) disable(ctx context.Context, gameID string, mode restoreMode) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !s.supported {
		return ErrUnsupported
	}
	if err := validID(gameID); err != nil {
		return err
	}
	if err := s.begin(gameID); err != nil {
		return err
	}
	defer s.end(gameID)
	if s.games.IsRunning(gameID) {
		return ErrRunning
	}
	if _, ok := s.get(gameID); !ok {
		return ErrNotActive
	}
	err := s.restore(gameID, mode)
	if err == nil || errors.Is(err, ErrGameDirMissing) {
		return err
	}
	return errors.Join(err, s.recordProblem(gameID, err))
}

// restore serves Disable, the rollback of a failed Enable and recovery at
// startup: from any phase it ends with the original files back and no entry.
// In strict mode every file is checked before the first one is touched, so a
// file the user replaced aborts the whole operation untouched. The wrapper's
// own config (dgVoodoo.conf) is the exception: the user edits it on purpose,
// so it is removed, or replaced by the original, whatever it contains.
func (s *Service) restore(gameID string, mode restoreMode) error {
	e, ok := s.get(gameID)
	if !ok {
		return ErrNotActive
	}
	dir, err := s.resolve(gameID, &e)
	if errors.Is(err, errGameGone) {
		return s.finish(gameID)
	}
	if err != nil {
		return err
	}
	cur := make([]string, len(e.Files))
	changed := make([]bool, len(e.Files))
	for i, f := range e.Files {
		_, sha, err := probe(filepath.Join(dir, f.Name))
		if err != nil {
			return fileErr(f.Name, err)
		}
		untouched := f.Orig && sha == f.OrigSHA
		cur[i] = sha
		changed[i] = sha != "" && sha != f.SHA && !untouched && f.Name != confName
		if changed[i] && mode == modeStrict {
			return fileErr(f.Name, ErrTampered)
		}
	}
	if e.Phase != phaseRemoving {
		e.Phase = phaseRemoving
		if err := s.commit(gameID, &e); err != nil {
			return err
		}
	}
	for i, f := range e.Files {
		dst := filepath.Join(dir, f.Name)
		switch {
		case changed[i]:
		case f.Orig && cur[i] == f.OrigSHA:
		case f.Orig:
			if err := s.putBack(gameID, dst, f); err != nil {
				return err
			}
		case cur[i] != "":
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
	if err := writeInto(dst, data); err != nil {
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
