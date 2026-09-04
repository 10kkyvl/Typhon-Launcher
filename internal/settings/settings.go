package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"typhon/internal/storage"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	CleanupKeep   = "keep"
	CleanupAsk    = "ask"
	CleanupDelete = "delete"

	RefreshManual   = "manual"
	RefreshHourly   = "1h"
	RefreshSixHours = "6h"
	RefreshHalfDay  = "12h"
	RefreshDaily    = "24h"

	LanguageSystem = "system"
	LanguageRU     = "ru"
	LanguageEN     = "en"

	PresenceOnline    = "online"
	PresenceAway      = "away"
	PresenceBusy      = "busy"
	PresenceInvisible = "invisible"

	KeepPreviousOff         = "off"
	KeepPreviousFirstLaunch = "first_launch"
	KeepPreviousDay         = "24h"

	LibraryFolderName = "TyphonLibrary"

	// CurrentTelemetryConsent is the version of the consent prompt this build
	// shows. A stored version of zero means the user has never been asked,
	// which is a different state from having been asked and declined: the
	// first is worth one prompt, the second must never be re-prompted.
	CurrentTelemetryConsent = 1

	dirGames       = "Games"
	dirDownloads   = "Downloads"
	dirScreenshots = "Screenshots"
)

var (
	ErrLibraryNotConfigured = errors.New("библиотека не настроена")
	ErrLibraryPathRelative  = errors.New("путь библиотеки должен быть абсолютным")
	ErrLibraryPathRoot      = errors.New("библиотека не может быть корнем диска")
	ErrLibraryParentEmpty   = errors.New("не выбрана папка для библиотеки")
)

type Settings struct {
	Theme                  string  `json:"theme"`
	UIScale                float64 `json:"uiScale"`
	Language               string  `json:"language"`
	LibraryPath            string  `json:"libraryPath"`
	DownloadsPath          string  `json:"downloadsPath"`
	GamesPath              string  `json:"gamesPath"`
	ScreenshotsPath        string  `json:"screenshotsPath"`
	LaunchOnStartup        bool    `json:"launchOnStartup"`
	MinimizeToTray         bool    `json:"minimizeToTray"`
	DiscordRichPresence    bool    `json:"discordRichPresence"`
	HardwareAcceleration   bool    `json:"hardwareAcceleration"`
	AnimationsEnabled      bool    `json:"animationsEnabled"`
	MaxActiveDownloads     int     `json:"maxActiveDownloads"`
	DownloadRateLimit      int64   `json:"downloadRateLimit"`
	UploadRateLimit        int64   `json:"uploadRateLimit"`
	UploadWhileDownloading bool    `json:"uploadWhileDownloading"`
	SeedAfterDownload      bool    `json:"seedAfterDownload"`
	InstallCleanupPolicy   string  `json:"installCleanupPolicy"`
	AutoInstall            bool    `json:"autoInstall"`
	SourceRefreshInterval  string  `json:"sourceRefreshInterval"`
	VerifyAfterInstall     bool    `json:"verifyAfterInstall"`
	InstallSkipShortcuts   bool    `json:"installSkipShortcuts"`
	InstallSkipExtras      bool    `json:"installSkipExtras"`
	DesktopShortcuts       bool    `json:"desktopShortcuts"`

	UpdateCheckAutomatically bool   `json:"updateCheckAutomatically"`
	UpdateAutoDownload       bool   `json:"updateAutoDownload"`
	UpdateAutoInstall        bool   `json:"updateAutoInstall"`
	UpdateSaveBackup         bool   `json:"updateSaveBackup"`
	KeepPreviousVersion      string `json:"keepPreviousVersion"`
	AllowTorrentReuse        bool   `json:"allowTorrentReuse"`

	LANSharing bool `json:"lanSharing"`

	PresenceStatus string `json:"presenceStatus"`

	AccountSync           bool `json:"accountSync"`
	SourcesNoticeAccepted bool `json:"sourcesNoticeAccepted"`
	AnonymousUsageStats   bool `json:"anonymousUsageStats"`
	AnonymousDiagnostics  bool `json:"anonymousDiagnostics"`

	TelemetryConsentVersion int `json:"telemetryConsentVersion"`
}

// TelemetryConsentRecorded reports whether the user has answered the consent
// prompt. Until they have, the switches hold defaults nobody agreed to.
func (s Settings) TelemetryConsentRecorded() bool {
	return s.TelemetryConsentVersion > 0
}

// UsageStatsAllowed and DiagnosticsAllowed are the only two questions callers
// may ask before sending anything. Reading the switch on its own would send
// the default answer of a question that has not been put to the user yet.
func (s Settings) UsageStatsAllowed() bool {
	return s.TelemetryConsentRecorded() && s.AnonymousUsageStats
}

func (s Settings) DiagnosticsAllowed() bool {
	return s.TelemetryConsentRecorded() && s.AnonymousDiagnostics
}

func Defaults() Settings {
	return Settings{
		Theme:                  "dark",
		UIScale:                1,
		Language:               LanguageSystem,
		LaunchOnStartup:        false,
		MinimizeToTray:         true,
		DiscordRichPresence:    false,
		HardwareAcceleration:   true,
		AnimationsEnabled:      true,
		MaxActiveDownloads:     2,
		DownloadRateLimit:      0,
		UploadRateLimit:        0,
		UploadWhileDownloading: false,
		SeedAfterDownload:      false,
		InstallCleanupPolicy:   CleanupDelete,
		AutoInstall:            false,
		SourceRefreshInterval:  RefreshSixHours,
		VerifyAfterInstall:     true,
		InstallSkipShortcuts:   true,
		InstallSkipExtras:      true,
		DesktopShortcuts:       true,

		UpdateCheckAutomatically: true,
		UpdateAutoDownload:       false,
		UpdateAutoInstall:        false,
		UpdateSaveBackup:         true,
		KeepPreviousVersion:      KeepPreviousFirstLaunch,
		AllowTorrentReuse:        true,

		LANSharing: false,

		PresenceStatus: PresenceOnline,

		AccountSync:           false,
		SourcesNoticeAccepted: false,

		// These are the values the consent prompt starts from on a fresh
		// install, not values anything may act on: DiagnosticsAllowed stays
		// false until the prompt is answered. Crash reports carry no game and
		// no behaviour, only what broke, so the prompt starts with them
		// selected; usage statistics describe what the user does and start
		// unselected.
		AnonymousUsageStats:     false,
		AnonymousDiagnostics:    true,
		TelemetryConsentVersion: 0,
	}
}

// consentProbe re-reads the consent keys with pointers so an absent key can be
// told from a stored false. Unmarshalling into Defaults() cannot make that
// distinction, and the two mean opposite things here.
type consentProbe struct {
	UsageStats     *bool `json:"anonymousUsageStats"`
	Diagnostics    *bool `json:"anonymousDiagnostics"`
	ConsentVersion *int  `json:"telemetryConsentVersion"`
}

// applyStoredConsent decides what an existing settings file means. It runs for
// every file that has one, and its whole job is to make sure a launcher update
// never turns telemetry on for somebody who was already installed: the new
// default applies to installations with no settings file at all, and to
// nothing else.
func applyStoredConsent(s Settings, p consentProbe) Settings {
	if p.ConsentVersion != nil {
		// A build that knows about the prompt wrote this file, so the stored
		// version is the whole answer: above zero it was answered, at zero it
		// was not. Nothing may be inferred from the switches here — at zero
		// they hold the prompt's preselection, and reading a preselection as
		// an answer is how a settings save made before the prompt was ever
		// shown would turn into consent.
		return s
	}

	// No version key at all: the file predates the prompt. Defaults() is the
	// wrong fallback for a missing switch here, because before the prompt
	// both switches shipped off — an absent key means the user never touched
	// one, not that they wanted the value this build now starts from.
	s.AnonymousUsageStats = p.UsageStats != nil && *p.UsageStats
	s.AnonymousDiagnostics = p.Diagnostics != nil && *p.Diagnostics

	// A switch found on in a file this old can only have been turned on by
	// hand, in settings, on purpose. That is an answer, and asking again would
	// be asking someone to repeat themselves. Both off is genuinely ambiguous
	// — it is equally the shipped default nobody ever touched — so those
	// installs see the prompt once.
	if s.AnonymousUsageStats || s.AnonymousDiagnostics {
		s.TelemetryConsentVersion = CurrentTelemetryConsent
	} else {
		s.TelemetryConsentVersion = 0
	}
	return s
}

func ValidLanguage(lang string) bool {
	switch lang {
	case LanguageSystem, LanguageRU, LanguageEN:
		return true
	default:
		return false
	}
}

func ValidPresenceStatus(status string) bool {
	switch status {
	case PresenceOnline, PresenceAway, PresenceBusy, PresenceInvisible:
		return true
	default:
		return false
	}
}

func normalizeLibraryPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", nil
	}
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("%w: %s", ErrLibraryPathRelative, path)
	}
	path = filepath.Clean(path)
	if filepath.Dir(path) == path {
		return "", fmt.Errorf("%w: %s", ErrLibraryPathRoot, path)
	}
	return path, nil
}

func derivePaths(s Settings) Settings {
	if s.LibraryPath == "" {
		s.GamesPath = ""
		s.DownloadsPath = ""
		s.ScreenshotsPath = ""
		return s
	}
	s.GamesPath = filepath.Join(s.LibraryPath, dirGames)
	s.DownloadsPath = filepath.Join(s.LibraryPath, dirDownloads)
	s.ScreenshotsPath = filepath.Join(s.LibraryPath, dirScreenshots)
	return s
}

func legacyLibraryPath(gamesPath string) string {
	gamesPath = strings.TrimSpace(gamesPath)
	if gamesPath == "" || !filepath.IsAbs(gamesPath) {
		return ""
	}
	root := filepath.Clean(gamesPath)
	if filepath.Base(root) == dirGames {
		root = filepath.Dir(root)
	}
	if filepath.Dir(root) == root {
		return ""
	}
	return root
}

func sanitize(s Settings) (Settings, error) {
	library, err := normalizeLibraryPath(s.LibraryPath)
	if err != nil {
		return Settings{}, err
	}
	s.LibraryPath = library
	s = derivePaths(s)
	if s.UIScale < 0.9 || s.UIScale > 1.25 {
		s.UIScale = 1
	}
	if s.MaxActiveDownloads < 1 {
		s.MaxActiveDownloads = 1
	}
	if s.MaxActiveDownloads > 10 {
		s.MaxActiveDownloads = 10
	}
	if s.DownloadRateLimit < 0 {
		s.DownloadRateLimit = 0
	}
	if s.UploadRateLimit < 0 {
		s.UploadRateLimit = 0
	}
	if s.TelemetryConsentVersion < 0 {
		s.TelemetryConsentVersion = 0
	}
	switch s.InstallCleanupPolicy {
	case CleanupKeep, CleanupAsk, CleanupDelete:
	default:
		s.InstallCleanupPolicy = CleanupDelete
	}
	switch s.SourceRefreshInterval {
	case RefreshManual, RefreshHourly, RefreshSixHours, RefreshHalfDay, RefreshDaily:
	default:
		s.SourceRefreshInterval = RefreshSixHours
	}
	if !ValidLanguage(s.Language) {
		s.Language = LanguageSystem
	}
	if !ValidPresenceStatus(s.PresenceStatus) {
		s.PresenceStatus = PresenceOnline
	}
	switch s.KeepPreviousVersion {
	case KeepPreviousOff, KeepPreviousFirstLaunch, KeepPreviousDay:
	default:
		s.KeepPreviousVersion = KeepPreviousFirstLaunch
	}
	return s, nil
}

var migrateConfigDirOnce sync.Once

func ConfigDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	newDir := filepath.Join(dir, "Typhon")
	migrateConfigDirOnce.Do(func() {
		oldDir := filepath.Join(dir, "Aurora")
		if _, err := os.Stat(newDir); !errors.Is(err, os.ErrNotExist) {
			return
		}
		if _, err := os.Stat(oldDir); errors.Is(err, os.ErrNotExist) {
			return
		}
		if err := os.Rename(oldDir, newDir); err != nil {
			slog.Warn("migrate config dir", "from", oldDir, "to", newDir, "error", err)
		}
	})
	return newDir, nil
}

type Service struct {
	mu       sync.Mutex
	path     string
	current  Settings
	subs     map[int]func(Settings)
	nextSub  int
	appliers []func(prev, next Settings) error
}

func NewService() (*Service, error) {
	dir, err := ConfigDir()
	if err != nil {
		return nil, fmt.Errorf("resolve config dir: %w", err)
	}
	return NewServiceAt(filepath.Join(dir, "settings.json"))
}

func NewServiceAt(path string) (*Service, error) {
	if path == "" {
		return nil, errors.New("settings path unavailable")
	}
	s := &Service{current: Defaults(), path: path}
	current, err := s.load()
	if err != nil {
		return nil, err
	}
	s.current = current
	return s, nil
}

func (s *Service) load() (Settings, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return Defaults(), nil
	}
	if err != nil {
		return Settings{}, fmt.Errorf("read settings %s: %w", s.path, err)
	}
	loaded := Defaults()
	if err := json.Unmarshal(data, &loaded); err != nil {
		return Settings{}, fmt.Errorf("parse settings %s: %w", s.path, err)
	}
	var probe consentProbe
	if err := json.Unmarshal(data, &probe); err != nil {
		return Settings{}, fmt.Errorf("parse settings consent %s: %w", s.path, err)
	}
	loaded = applyStoredConsent(loaded, probe)
	if loaded.LibraryPath == "" {
		loaded.LibraryPath = legacyLibraryPath(loaded.GamesPath)
	}
	current, err := sanitize(loaded)
	if err != nil {
		return Settings{}, fmt.Errorf("settings %s: %w", s.path, err)
	}
	return current, nil
}

// An applier is handed the stored settings alongside the incoming ones so it
// can sit out saves that do not touch its own field: a registry or tray failure
// must not block saving an unrelated setting such as the theme.
//
//wails:ignore
func (s *Service) AddApplier(fn func(prev, next Settings) error) error {
	if fn == nil {
		return errors.New("settings: nil applier")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.appliers = append(s.appliers, fn)
	return nil
}

//wails:ignore
func (s *Service) Subscribe(fn func(Settings)) func() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.subs == nil {
		s.subs = map[int]func(Settings){}
	}
	id := s.nextSub
	s.nextSub++
	s.subs[id] = fn
	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		delete(s.subs, id)
	}
}

func (s *Service) GetSettings() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.current
}

func (s *Service) SaveSettings(next Settings) error {
	next, err := sanitize(next)
	if err != nil {
		return err
	}

	s.mu.Lock()
	prev := s.current
	appliers := make([]func(prev, next Settings) error, len(s.appliers))
	copy(appliers, s.appliers)
	s.mu.Unlock()

	// The consent version only ever moves forward. Every other field here
	// comes straight from a caller that may have assembled the struct without
	// knowing this field exists, and a zero from such a caller would erase the
	// record that the user was asked — after which the prompt reappears and
	// the defaults apply again to somebody who already answered.
	if next.TelemetryConsentVersion < prev.TelemetryConsentVersion {
		next.TelemetryConsentVersion = prev.TelemetryConsentVersion
	}

	for _, apply := range appliers {
		if err := apply(prev, next); err != nil {
			return fmt.Errorf("apply settings: %w", err)
		}
	}

	next, subs, err := s.persist(next)
	if err != nil {
		return err
	}
	if app := application.Get(); app != nil {
		app.Event.Emit("settings:updated", next)
	}
	for _, notify := range subs {
		notify(next)
	}
	return nil
}

// SaveConsent records the answer to the consent prompt together with the
// version of the prompt that was answered, in a single write. Recording them
// separately would leave a window where the switches are set and nothing
// remembers that anyone was asked; a launcher killed inside that window comes
// back, asks again, and shows the user a question they have already answered.
// Callers get the stored settings back so the prompt closes on what was
// written rather than on what it sent.
func (s *Service) SaveConsent(usageStats, diagnostics bool) (Settings, error) {
	next := s.GetSettings()
	next.AnonymousUsageStats = usageStats
	next.AnonymousDiagnostics = diagnostics
	next.TelemetryConsentVersion = CurrentTelemetryConsent
	if err := s.SaveSettings(next); err != nil {
		return Settings{}, fmt.Errorf("save telemetry consent: %w", err)
	}
	return s.GetSettings(), nil
}

func (s *Service) persist(next Settings) (Settings, []func(Settings), error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.path == "" {
		return next, nil, errors.New("settings path unavailable")
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return next, nil, fmt.Errorf("create config dir: %w", err)
	}
	data, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return next, nil, err
	}
	if err := storage.WriteAtomic(s.path, data); err != nil {
		return next, nil, fmt.Errorf("write settings: %w", err)
	}
	s.current = next

	subs := make([]func(Settings), 0, len(s.subs))
	for _, fn := range s.subs {
		subs = append(subs, fn)
	}
	return next, subs, nil
}

func libraryRootFor(parent string) (string, error) {
	parent = strings.TrimSpace(parent)
	if parent == "" {
		return "", ErrLibraryParentEmpty
	}
	if !filepath.IsAbs(parent) {
		return "", fmt.Errorf("%w: %s", ErrLibraryPathRelative, parent)
	}
	parent = filepath.Clean(parent)
	root := parent
	if !strings.EqualFold(filepath.Base(parent), LibraryFolderName) {
		root = filepath.Join(parent, LibraryFolderName)
	}
	return normalizeLibraryPath(root)
}

func (s *Service) ProposeLibraryPath(parent string) (string, error) {
	return libraryRootFor(parent)
}

func (s *Service) SetupLibrary(parent string) (Settings, error) {
	root, err := libraryRootFor(parent)
	if err != nil {
		return Settings{}, err
	}
	if err := createLibrary(root); err != nil {
		return Settings{}, err
	}
	next := s.GetSettings()
	next.LibraryPath = root
	if err := s.SaveSettings(next); err != nil {
		return Settings{}, err
	}
	return s.GetSettings(), nil
}

func createLibrary(root string) error {
	dirs := []string{
		root,
		filepath.Join(root, dirGames),
		filepath.Join(root, dirDownloads),
		filepath.Join(root, dirScreenshots),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("создать папку %s: %w", dir, err)
		}
		if err := checkWritable(dir); err != nil {
			return err
		}
	}
	return nil
}

func checkWritable(dir string) error {
	f, err := os.CreateTemp(dir, ".typhon-probe-*")
	if err != nil {
		return fmt.Errorf("нет доступа на запись в %s: %w", dir, err)
	}
	name := f.Name()
	writeErr := writeProbe(f)
	closeErr := f.Close()
	removeErr := os.Remove(name)
	switch {
	case writeErr != nil:
		return fmt.Errorf("нет доступа на запись в %s: %w", dir, writeErr)
	case closeErr != nil:
		return fmt.Errorf("нет доступа на запись в %s: %w", dir, closeErr)
	case removeErr != nil:
		return fmt.Errorf("нет доступа на запись в %s: %w", dir, removeErr)
	}
	return nil
}

func writeProbe(f *os.File) error {
	if _, err := f.Write([]byte("typhon")); err != nil {
		return err
	}
	return f.Sync()
}
