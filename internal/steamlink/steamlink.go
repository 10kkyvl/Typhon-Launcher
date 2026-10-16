// Package steamlink adds games to Steam as non-Steam shortcuts: it edits
// userdata/<id>/config/shortcuts.vdf of every Steam user and copies artwork
// into their grid folders.
package steamlink

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"typhon/internal/procs"
	"typhon/internal/storage"
	"typhon/internal/uierr"
)

var (
	ErrNotInstalled = uierr.New("steam.not_installed", "Steam не установлен")
	ErrNoUsers      = uierr.New("steam.no_users", "в Steam не выполнен вход ни под одним пользователем")
	ErrSteamRunning = uierr.New("steam.running", "закройте Steam, иначе он перезапишет список ярлыков")
	ErrUnsupported  = uierr.Wrap("steam.unsupported", fmt.Errorf("добавление в Steam поддерживается только в Windows: %w", errors.ErrUnsupported))
)

const (
	backupSuffix = ".typhon-backup"
	maxArtBytes  = 32 << 20
)

var artExts = []string{".png", ".jpg", ".jpeg"}

// Art holds absolute paths of local image files; either may be empty.
type Art struct {
	Cover string
	Hero  string
}

// manager serialises every operation: two callers reading the same
// shortcuts.vdf and writing back would drop each other's entry.
type manager struct {
	mu      sync.Mutex
	root    func() (string, error)
	running func(ctx context.Context) (bool, error)
	write   func(path string, data []byte) error
}

var system = &manager{root: Root, running: steamRunning, write: storage.WriteAtomic}

// Sync is the part of the library state that rides along with another
// operation inside the same transaction: shortcuts of games that went away are
// dropped and shortcuts that already exist get their current target.
type Sync struct {
	Update []Shortcut
	Drop   []uint32
}

func Add(ctx context.Context, sc Shortcut, art Art, plan Sync) error {
	if !Supported() {
		return ErrUnsupported
	}
	return system.add(ctx, sc, art, plan)
}

func Remove(ctx context.Context, appID uint32, plan Sync) error {
	if !Supported() {
		return ErrUnsupported
	}
	return system.remove(ctx, appID, plan)
}

// Reconcile drops the shortcuts of the listed app ids and refreshes the ones
// in update that are already present; it never adds. With nothing to change it
// writes nothing and does not care whether Steam is running.
func Reconcile(ctx context.Context, update []Shortcut, drop []uint32) error {
	if !Supported() {
		return ErrUnsupported
	}
	return system.reconcile(ctx, Sync{Update: update, Drop: drop})
}

func Contains(ctx context.Context, appID uint32) (bool, error) {
	if !Supported() {
		return false, ErrUnsupported
	}
	return system.contains(ctx, appID)
}

type user struct {
	id          string
	configDir   string
	vdfPath     string
	original    []byte
	hadOriginal bool
	doc         *document
	changed     bool
	written     bool
}

type artFile struct {
	name string
	ext  string
	data []byte
}

func (plan Sync) validate() error {
	for _, sc := range plan.Update {
		if err := sc.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (plan Sync) apply(d *document) bool {
	changed := false
	for _, id := range plan.Drop {
		if d.remove(id) {
			changed = true
		}
	}
	for _, sc := range plan.Update {
		if d.update(sc) {
			changed = true
		}
	}
	return changed
}

func (m *manager) add(ctx context.Context, sc Shortcut, art Art, plan Sync) error {
	if err := sc.Validate(); err != nil {
		return err
	}
	if err := plan.validate(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.ensureClosed(ctx); err != nil {
		return err
	}
	users, err := m.load(ctx, func(d *document) bool {
		changed := plan.apply(d)
		if d.upsert(sc) {
			changed = true
		}
		return changed
	})
	if err != nil {
		return err
	}
	files, err := readArt(sc.AppID, art)
	if err != nil {
		return err
	}
	for _, u := range users {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := m.writeArt(u, files); err != nil {
			return err
		}
	}
	if err := m.commit(ctx, users); err != nil {
		return err
	}
	return removeArt(users, plan.Drop)
}

func (m *manager) remove(ctx context.Context, appID uint32, plan Sync) error {
	if err := plan.validate(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	users, err := m.load(ctx, func(d *document) bool {
		changed := d.remove(appID)
		if plan.apply(d) {
			changed = true
		}
		return changed
	})
	if err != nil {
		return err
	}
	if err := m.commitIfChanged(ctx, users); err != nil {
		return err
	}
	return removeArt(users, append([]uint32{appID}, plan.Drop...))
}

func (m *manager) reconcile(ctx context.Context, plan Sync) error {
	if err := plan.validate(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	users, err := m.load(ctx, plan.apply)
	if errors.Is(err, ErrNoUsers) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := m.commitIfChanged(ctx, users); err != nil {
		return err
	}
	return removeArt(users, plan.Drop)
}

// commitIfChanged asks about a running Steam only when something has to be
// written: a client that is open is not an obstacle to a no-op.
func (m *manager) commitIfChanged(ctx context.Context, users []*user) error {
	for _, u := range users {
		if u.changed {
			if err := m.ensureClosed(ctx); err != nil {
				return err
			}
			return m.commit(ctx, users)
		}
	}
	return nil
}

func (m *manager) contains(ctx context.Context, appID uint32) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	users, err := m.load(ctx, func(d *document) bool { return d.has(appID) })
	if errors.Is(err, ErrNoUsers) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	for _, u := range users {
		if u.changed {
			return true, nil
		}
	}
	return false, nil
}

func (m *manager) ensureClosed(ctx context.Context) error {
	running, err := m.running(ctx)
	if err != nil {
		return fmt.Errorf("проверка, запущен ли Steam: %w", err)
	}
	if running {
		return ErrSteamRunning
	}
	return nil
}

// load reads and parses shortcuts.vdf of every user and applies edit in memory
// only, so a damaged file of one user stops the operation before any other user
// is touched. For contains, "changed" carries the answer of the lookup.
func (m *manager) load(ctx context.Context, edit func(*document) bool) ([]*user, error) {
	root, err := m.root()
	if err != nil {
		return nil, err
	}
	ids, err := listUsers(root)
	if err != nil {
		return nil, err
	}
	users := make([]*user, 0, len(ids))
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		cfg := filepath.Join(root, "userdata", id, "config")
		u := &user{id: id, configDir: cfg, vdfPath: filepath.Join(cfg, "shortcuts.vdf")}
		data, err := readLimited(u.vdfPath, maxVDFBytes)
		switch {
		case errors.Is(err, fs.ErrNotExist):
		case err != nil:
			return nil, fmt.Errorf("steam %s: %w", id, err)
		default:
			u.original = data
			u.hadOriginal = true
		}
		u.doc, err = parseShortcuts(u.original)
		if err != nil {
			return nil, fmt.Errorf("steam %s: разбор %s: %w", id, u.vdfPath, err)
		}
		u.changed = edit(u.doc)
		users = append(users, u)
	}
	return users, nil
}

func (m *manager) commit(ctx context.Context, users []*user) error {
	for _, u := range users {
		if !u.changed {
			continue
		}
		if err := ctx.Err(); err != nil {
			return errors.Join(err, m.rollback(users))
		}
		if err := m.writeUser(u); err != nil {
			return errors.Join(err, m.rollback(users))
		}
		u.written = true
	}
	return nil
}

func (m *manager) writeUser(u *user) error {
	if err := ensureDir(u.configDir); err != nil {
		return err
	}
	if u.hadOriginal {
		if err := m.write(u.vdfPath+backupSuffix, u.original); err != nil {
			return fmt.Errorf("steam %s: копия shortcuts.vdf: %w", u.id, err)
		}
	}
	if err := m.write(u.vdfPath, u.doc.bytes()); err != nil {
		return fmt.Errorf("steam %s: запись shortcuts.vdf: %w", u.id, err)
	}
	return nil
}

func (m *manager) rollback(users []*user) error {
	var errs []error
	for _, u := range users {
		if !u.written {
			continue
		}
		if u.hadOriginal {
			if err := m.write(u.vdfPath, u.original); err != nil {
				errs = append(errs, fmt.Errorf("steam %s: откат shortcuts.vdf: %w", u.id, err))
			}
			continue
		}
		if err := os.Remove(u.vdfPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
			errs = append(errs, fmt.Errorf("steam %s: откат shortcuts.vdf: %w", u.id, err))
		}
	}
	return errors.Join(errs...)
}

func listUsers(root string) ([]string, error) {
	if root == "" {
		return nil, ErrNotInstalled
	}
	entries, err := os.ReadDir(filepath.Join(root, "userdata"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrNoUsers
	}
	if err != nil {
		return nil, fmt.Errorf("steam: список пользователей: %w", err)
	}
	var ids []string
	for _, e := range entries {
		name := e.Name()
		if !e.IsDir() || name == "0" || !digitsOnly(name) {
			continue
		}
		ids = append(ids, name)
	}
	if len(ids) == 0 {
		return nil, ErrNoUsers
	}
	sort.Strings(ids)
	return ids, nil
}

func digitsOnly(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func readLimited(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path) //nolint:gosec // G304: путь собран из каталога Steam и числового имени пользователя, размер ограничен limit (инварианты 32, 34)
	if err != nil {
		return nil, err
	}
	data, readErr := io.ReadAll(io.LimitReader(f, limit+1))
	closeErr := f.Close()
	if readErr != nil {
		return nil, fmt.Errorf("read %s: %w", path, readErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close %s: %w", path, closeErr)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%s: %w", path, errVDFTooLarge)
	}
	return data, nil
}

// ensureDir creates one missing directory with the mode of its parent; the
// chmod makes the result independent of the umask.
func ensureDir(dir string) error {
	info, err := os.Stat(dir)
	if err == nil {
		if !info.IsDir() {
			return fmt.Errorf("%s: не каталог", dir)
		}
		return nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("stat %s: %w", dir, err)
	}
	parent, err := os.Stat(filepath.Dir(dir))
	if err != nil {
		return fmt.Errorf("stat %s: %w", filepath.Dir(dir), err)
	}
	mode := parent.Mode().Perm()
	if err := os.Mkdir(dir, mode); err != nil && !errors.Is(err, fs.ErrExist) {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	if err := os.Chmod(dir, mode); err != nil {
		return fmt.Errorf("chmod %s: %w", dir, err)
	}
	return nil
}

type artSource struct {
	path  string
	names []string
}

func readArt(appID uint32, art Art) ([]artFile, error) {
	id := strconv.FormatUint(uint64(appID), 10)
	sources := []artSource{
		{art.Cover, []string{id + "p"}},
		{art.Hero, []string{id + "_hero", id}},
	}
	var files []artFile
	for _, src := range sources {
		if src.path == "" {
			continue
		}
		ext := strings.ToLower(filepath.Ext(src.path))
		if !supportedArtExt(ext) {
			slog.Warn("steam art skipped: unsupported format", "path", src.path)
			continue
		}
		data, err := readLimited(src.path, maxArtBytes)
		if err != nil {
			return nil, fmt.Errorf("steam: обложка %s: %w", src.path, err)
		}
		for _, name := range src.names {
			files = append(files, artFile{name: name, ext: ext, data: data})
		}
	}
	return files, nil
}

func supportedArtExt(ext string) bool {
	for _, e := range artExts {
		if e == ext {
			return true
		}
	}
	return false
}

// writeArt puts the images into the grid folder of one user. A file of the same
// image under another extension would shadow the new one, so it goes first.
func (m *manager) writeArt(u *user, files []artFile) error {
	if len(files) == 0 {
		return nil
	}
	if err := ensureDir(u.configDir); err != nil {
		return err
	}
	grid := filepath.Join(u.configDir, "grid")
	if err := ensureDir(grid); err != nil {
		return err
	}
	for _, f := range files {
		for _, other := range artExts {
			if other == f.ext {
				continue
			}
			if err := os.Remove(filepath.Join(grid, f.name+other)); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return fmt.Errorf("steam %s: удаление старой обложки: %w", u.id, err)
			}
		}
		target := filepath.Join(grid, f.name+f.ext)
		if err := m.write(target, f.data); err != nil {
			return fmt.Errorf("steam %s: запись %s: %w", u.id, target, err)
		}
	}
	return nil
}

// removeArt deletes the grid images of the app ids for every user. One
// directory listing per user instead of a stat per possible file name.
func removeArt(users []*user, appIDs []uint32) error {
	if len(appIDs) == 0 {
		return nil
	}
	names := make(map[string]struct{}, len(appIDs)*3)
	for _, appID := range appIDs {
		id := strconv.FormatUint(uint64(appID), 10)
		names[id] = struct{}{}
		names[id+"p"] = struct{}{}
		names[id+"_hero"] = struct{}{}
	}
	var errs []error
	for _, u := range users {
		grid := filepath.Join(u.configDir, "grid")
		entries, err := os.ReadDir(grid)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("steam %s: %w", u.id, err))
			continue
		}
		for _, e := range entries {
			ext := filepath.Ext(e.Name())
			if e.IsDir() || !supportedArtExt(strings.ToLower(ext)) {
				continue
			}
			if _, ok := names[strings.TrimSuffix(e.Name(), ext)]; !ok {
				continue
			}
			target := filepath.Join(grid, e.Name())
			if err := os.Remove(target); err != nil && !errors.Is(err, fs.ErrNotExist) {
				errs = append(errs, fmt.Errorf("steam %s: удаление %s: %w", u.id, target, err))
			}
		}
	}
	return errors.Join(errs...)
}

// runningFromList decides whether the Steam client owns the pid recorded in
// the registry. An unreadable or partial process table is an error: "could not
// look" must not read as "Steam is closed".
func runningFromList(ctx context.Context, pid uint32, list func(context.Context) ([]procs.Process, bool, error)) (bool, error) {
	if pid == 0 {
		return false, nil
	}
	all, complete, err := list(ctx)
	if err != nil {
		return false, fmt.Errorf("список процессов: %w", err)
	}
	if !complete {
		return false, errors.New("список процессов неполон")
	}
	for _, p := range all {
		if p.PID != pid {
			continue
		}
		if p.PathUnknown {
			return true, nil
		}
		return strings.EqualFold(path.Base(strings.ReplaceAll(p.Path, `\`, "/")), "steam.exe"), nil
	}
	return false, nil
}
