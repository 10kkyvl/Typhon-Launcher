package library

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"typhon/internal/platform"
	"typhon/internal/uierr"
)

const (
	maxProfiles    = 8
	maxProfileName = 40
)

var (
	errExecutableMissing   = uierr.New("library.executable_missing", "исполняемый файл больше не существует")
	errProfileNotFound     = uierr.New("library.profile_not_found", "профиль запуска не найден")
	errProfileName         = uierr.New("library.profile_name_invalid", "название профиля должно быть от 1 до 40 символов без управляющих")
	errProfileNameTaken    = uierr.New("library.profile_name_taken", "профиль с таким названием уже есть")
	errProfileLimit        = uierr.New("library.profile_limit", "слишком много профилей запуска")
	errProfileNoInstallDir = uierr.New("library.profile_no_install_dir", "у игры не задана папка установки")
	errProfileOutside      = uierr.New("library.profile_executable_outside", "исполняемый файл профиля должен лежать в папке игры")
	errProfileNotFile      = uierr.New("library.profile_executable_invalid", "исполняемый файл профиля не найден или не является файлом")
	errProfileExternal     = uierr.New("library.profile_unsupported", "у этой игры нельзя завести профили запуска")
	errProfileBadID        = uierr.New("library.profile_bad_id", "идентификатор профиля непригоден для командной строки")
)

type LaunchProfile struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Executable   string   `json:"executable"`
	Args         []string `json:"args,omitempty"`
	RunAsAdmin   bool     `json:"runAsAdmin,omitempty"`
	ShortcutPath string   `json:"shortcutPath,omitempty"`
}

type ProfileInput struct {
	Name       string `json:"name"`
	Executable string `json:"executable"`
	ArgsLine   string `json:"argsLine"`
	RunAsAdmin bool   `json:"runAsAdmin"`
}

// ProfileView with an empty ID is the game's own Executable/LaunchArgs/RunAsAdmin.
type ProfileView struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Executable   string `json:"executable"`
	ArgsLine     string `json:"argsLine"`
	RunAsAdmin   bool   `json:"runAsAdmin"`
	IsDefault    bool   `json:"isDefault"`
	IsLast       bool   `json:"isLast"`
	ShortcutPath string `json:"shortcutPath,omitempty"`
}

const EventProfileFallback = "library:profile-fallback"

// ProfileFallback tells the UI that Play used the default profile because the
// remembered one could not be launched.
type ProfileFallback struct {
	GameID      string `json:"gameId"`
	ProfileName string `json:"profileName"`
	Code        string `json:"code"`
}

type chosenProfile struct {
	id         string
	executable string
	args       []string
	admin      bool
}

// SafeProfileID reports whether id may travel in a shortcut command line.
func SafeProfileID(id string) bool { return safeCommandLineID(id) }

func newProfileID() (string, error) {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("profile id: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}

func findProfile(g *Game, profileID string) *LaunchProfile {
	for i := range g.Profiles {
		if g.Profiles[i].ID == profileID {
			return &g.Profiles[i]
		}
	}
	return nil
}

func cloneProfiles(in []LaunchProfile) []LaunchProfile {
	out := make([]LaunchProfile, len(in))
	for i, p := range in {
		p.Args = slices.Clone(p.Args)
		out[i] = p
	}
	return out
}

// resolveProfileExe turns the stored relative path into an absolute one and
// proves it is still a regular file inside installDir, symlinks included.
func resolveProfileExe(installDir, rel string) (string, error) {
	if installDir == "" {
		return "", errProfileNoInstallDir
	}
	if rel == "" || filepath.IsAbs(rel) || filepath.VolumeName(rel) != "" {
		return "", errProfileOutside
	}
	abs := filepath.Join(installDir, filepath.FromSlash(rel))
	if !platform.Inside(installDir, abs) {
		return "", errProfileOutside
	}
	if err := checkProfileFile(installDir, abs); err != nil {
		return "", err
	}
	return abs, nil
}

func checkProfileFile(installDir, abs string) error {
	info, err := os.Stat(abs)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("%w: %w", errProfileNotFile, err)
		}
		return fmt.Errorf("исполняемый файл профиля: %w", err)
	}
	if !info.Mode().IsRegular() {
		return errProfileNotFile
	}
	realDir, err := filepath.EvalSymlinks(installDir)
	if err != nil {
		return fmt.Errorf("папка установки: %w", err)
	}
	realExe, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return fmt.Errorf("исполняемый файл профиля: %w", err)
	}
	if !platform.Inside(realDir, realExe) {
		return errProfileOutside
	}
	return nil
}

func relativeProfileExe(installDir, abs string) (string, error) {
	if installDir == "" {
		return "", errProfileNoInstallDir
	}
	abs = strings.TrimSpace(abs)
	if abs == "" || !filepath.IsAbs(abs) {
		return "", errProfileNotFile
	}
	if !platform.Inside(installDir, abs) {
		return "", errProfileOutside
	}
	if err := checkProfileFile(installDir, abs); err != nil {
		return "", err
	}
	rel, err := filepath.Rel(installDir, abs)
	if err != nil {
		return "", fmt.Errorf("путь профиля: %w", err)
	}
	return filepath.ToSlash(rel), nil
}

func normalizeProfileName(name string) (string, error) {
	name = strings.TrimSpace(name)
	n := utf8.RuneCountInString(name)
	if n < 1 || n > maxProfileName {
		return "", errProfileName
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return "", errProfileName
		}
	}
	return name, nil
}

type profileDraft struct {
	name  string
	rel   string
	args  []string
	admin bool
}

func draftProfile(installDir string, in ProfileInput) (profileDraft, error) {
	name, err := normalizeProfileName(in.Name)
	if err != nil {
		return profileDraft{}, err
	}
	args, err := ParseLaunchArgs(in.ArgsLine)
	if err != nil {
		return profileDraft{}, err
	}
	args, err = normalizeLaunchArgs(args)
	if err != nil {
		return profileDraft{}, err
	}
	rel, err := relativeProfileExe(installDir, in.Executable)
	if err != nil {
		return profileDraft{}, err
	}
	return profileDraft{name: name, rel: rel, args: args, admin: in.RunAsAdmin}, nil
}

func nameTaken(g *Game, name, exceptID string) bool {
	for _, p := range g.Profiles {
		if p.ID != exceptID && strings.EqualFold(p.Name, name) {
			return true
		}
	}
	return false
}

func chooseProfile(g *Game, profileID string, explicit bool) (chosenProfile, error) {
	if !explicit {
		profileID = g.LastProfile
	}
	if profileID != "" {
		if p := findProfile(g, profileID); p != nil {
			exe, err := resolveProfileExe(g.InstallDir, p.Executable)
			if err != nil {
				return chosenProfile{}, fmt.Errorf("профиль «%s»: %w", p.Name, err)
			}
			return chosenProfile{id: p.ID, executable: exe, args: p.Args, admin: p.RunAsAdmin}, nil
		}
		if explicit {
			return chosenProfile{}, errProfileNotFound
		}
	}
	return chosenProfile{executable: g.Executable, args: g.LaunchArgs, admin: g.RunAsAdmin}, nil
}

// fallbackProfileLocked is the way out of a remembered profile whose exe is
// gone: Play must not be locked out, so the choice is reset to the default and
// the UI is told. If the reset cannot be saved the launch is aborted, otherwise
// the same fallback would silently repeat.
func (s *Service) fallbackProfileLocked(game *Game, cause error) (chosenProfile, error) {
	name := ""
	if p := findProfile(game, game.LastProfile); p != nil {
		name = p.Name
	}
	previous := game.LastProfile
	game.LastProfile = ""
	if err := s.persist(); err != nil {
		game.LastProfile = previous
		return chosenProfile{}, fmt.Errorf("save library: %w", err)
	}
	s.emitUpdated()
	emit(EventProfileFallback, ProfileFallback{GameID: game.ID, ProfileName: name, Code: uierr.Code(cause)})
	return chosenProfile{executable: game.Executable, args: game.LaunchArgs, admin: game.RunAsAdmin}, nil
}

// rememberProfileLocked records the choice before the process starts, so a
// save failure aborts the launch instead of failing after the game is running.
func (s *Service) rememberProfileLocked(game *Game, profileID string) error {
	if game.LastProfile == profileID {
		return nil
	}
	previous := game.LastProfile
	game.LastProfile = profileID
	if err := s.persist(); err != nil {
		game.LastProfile = previous
		return fmt.Errorf("save library: %w", err)
	}
	s.emitUpdated()
	return nil
}

func profileView(g *Game, p LaunchProfile) ProfileView {
	exe := p.Executable
	if g.InstallDir != "" {
		exe = filepath.Join(g.InstallDir, filepath.FromSlash(p.Executable))
	}
	return ProfileView{
		ID:           p.ID,
		Name:         p.Name,
		Executable:   exe,
		ArgsLine:     LaunchArgsLine(p.Args),
		RunAsAdmin:   p.RunAsAdmin,
		IsLast:       g.LastProfile == p.ID,
		ShortcutPath: p.ShortcutPath,
	}
}

func (s *Service) Profiles(id string) ([]ProfileView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	g := s.findLocked(id)
	if g == nil {
		return nil, errNotFound
	}
	last := g.LastProfile
	if findProfile(g, last) == nil {
		last = ""
	}
	views := []ProfileView{{
		Name:         "Основной",
		Executable:   g.Executable,
		ArgsLine:     LaunchArgsLine(g.LaunchArgs),
		RunAsAdmin:   g.RunAsAdmin,
		IsDefault:    true,
		IsLast:       last == "",
		ShortcutPath: g.ShortcutPath,
	}}
	for _, p := range g.Profiles {
		v := profileView(g, p)
		v.IsLast = last == p.ID
		views = append(views, v)
	}
	return views, nil
}

func (s *Service) installDirOf(id string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	game := s.findLocked(id)
	if game == nil {
		return "", errNotFound
	}
	if game.External != nil {
		return "", errProfileExternal
	}
	return game.InstallDir, nil
}

func (s *Service) AddProfile(id string, in ProfileInput) (ProfileView, error) {
	installDir, err := s.installDirOf(id)
	if err != nil {
		return ProfileView{}, err
	}
	draft, err := draftProfile(installDir, in)
	if err != nil {
		return ProfileView{}, err
	}
	profileID, err := newProfileID()
	if err != nil {
		return ProfileView{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	game := s.findLocked(id)
	if game == nil {
		return ProfileView{}, errNotFound
	}
	if game.InstallDir != installDir {
		return ProfileView{}, errInstallationChanged
	}
	if len(game.Profiles) >= maxProfiles {
		return ProfileView{}, errProfileLimit
	}
	if nameTaken(game, draft.name, "") {
		return ProfileView{}, errProfileNameTaken
	}
	previous := *game
	profile := LaunchProfile{ID: profileID, Name: draft.name, Executable: draft.rel, Args: draft.args, RunAsAdmin: draft.admin}
	game.Profiles = append(cloneProfiles(previous.Profiles), profile)
	if err := s.persist(); err != nil {
		*game = previous
		return ProfileView{}, fmt.Errorf("save library: %w", err)
	}
	s.emitUpdated()
	return profileView(game, profile), nil
}

func (s *Service) UpdateProfile(id, profileID string, in ProfileInput) error {
	if profileID == "" {
		return errProfileNotFound
	}
	installDir, err := s.installDirOf(id)
	if err != nil {
		return err
	}
	draft, err := draftProfile(installDir, in)
	if err != nil {
		return err
	}

	s.mu.Lock()
	game := s.findLocked(id)
	if game == nil {
		s.mu.Unlock()
		return errNotFound
	}
	if game.InstallDir != installDir {
		s.mu.Unlock()
		return errInstallationChanged
	}
	current := findProfile(game, profileID)
	if current == nil {
		s.mu.Unlock()
		return errProfileNotFound
	}
	if nameTaken(game, draft.name, profileID) {
		s.mu.Unlock()
		return errProfileNameTaken
	}
	oldLink := current.ShortcutPath
	oldTitle := game.Title + " (" + current.Name + ")"
	if oldLink != "" && draft.name != current.Name {
		newName, nameErr := s.shortcuts.FileName(game.Title + " (" + draft.name + ")")
		dir, dirErr := s.shortcuts.DesktopDir()
		if err := errors.Join(nameErr, dirErr); err != nil {
			s.mu.Unlock()
			return err
		}
		if !strings.EqualFold(filepath.Join(dir, newName), oldLink) {
			if err := s.shortcuts.Remove(oldLink); err != nil {
				s.mu.Unlock()
				return fmt.Errorf("ярлык профиля «%s»: %w", oldTitle, err)
			}
		}
	}
	previous := *game
	game.Profiles = cloneProfiles(previous.Profiles)
	p := findProfile(game, profileID)
	p.Name, p.Executable, p.Args, p.RunAsAdmin = draft.name, draft.rel, draft.args, draft.admin
	if err := s.persist(); err != nil {
		*game = previous
		s.mu.Unlock()
		return fmt.Errorf("save library: %w", err)
	}
	s.emitUpdated()
	ctx := s.ctx
	s.mu.Unlock()

	var errs []error
	if oldLink != "" {
		errs = append(errs, s.CreateProfileShortcut(id, profileID))
	}
	s.reconcileSteam(ctx, "profile-update")
	return errors.Join(errs...)
}

// RemoveProfile takes the shortcut and the Steam entry away first and refuses
// if either cannot go: a profile deleted from the library must not leave a
// link that launches an unknown profile.
func (s *Service) RemoveProfile(id, profileID string) error {
	if profileID == "" {
		return errProfileNotFound
	}
	s.mu.Lock()
	game := s.findLocked(id)
	if game == nil {
		s.mu.Unlock()
		return errNotFound
	}
	p := findProfile(game, profileID)
	if p == nil {
		s.mu.Unlock()
		return errProfileNotFound
	}
	link := p.ShortcutPath
	ctx := s.ctx
	s.mu.Unlock()

	if err := s.removeSteamEntry(ctx, id, profileID); err != nil {
		return fmt.Errorf("запись в Steam: %w", err)
	}
	if link != "" {
		if err := s.shortcuts.Remove(link); err != nil {
			return fmt.Errorf("ярлык профиля: %w", err)
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	game = s.findLocked(id)
	if game == nil {
		return errNotFound
	}
	if findProfile(game, profileID) == nil {
		return errProfileNotFound
	}
	previous := *game
	game.Profiles = slices.DeleteFunc(cloneProfiles(previous.Profiles), func(x LaunchProfile) bool { return x.ID == profileID })
	if game.LastProfile == profileID {
		game.LastProfile = ""
	}
	if err := s.persist(); err != nil {
		*game = previous
		return fmt.Errorf("save library: %w", err)
	}
	s.emitUpdated()
	return nil
}
