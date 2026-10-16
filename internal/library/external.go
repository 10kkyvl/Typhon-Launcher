package library

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"typhon/internal/platform"
	"typhon/internal/uierr"
)

var (
	ErrExternalProvider  = errors.New("неизвестный внешний лаунчер")
	ErrExternalAppID     = errors.New("у внешней игры не задан идентификатор")
	ErrExternalTitle     = errors.New("у внешней игры не задано название")
	ErrExternalDir       = errors.New("каталог внешней игры не найден")
	ErrExternalNoLaunch  = errors.New("у внешней игры нет ни файла запуска, ни ссылки запуска")
	ErrExternalURIScheme = errors.New("ссылка запуска имеет недопустимый вид")
)

const maxLaunchURILen = 512

var (
	steamLaunchURI = regexp.MustCompile(`^steam://rungameid/[0-9]{1,12}$`)
	epicLaunchURI  = regexp.MustCompile(`^com\.epicgames\.launcher://apps/[A-Za-z0-9._-]+(%3A[A-Za-z0-9._-]+){2}\?action=launch&silent=true$`)
)

type ExternalGame struct {
	Provider        string
	AppID           string
	Title           string
	InstallDir      string
	Executable      string
	LaunchURI       string
	CanonicalGameID string
}

type ExternalSkip struct {
	Provider string
	AppID    string
	Title    string
	Reason   string
}

type ExternalOutcome struct {
	Provider string
	AppID    string
	Added    bool
	Updated  bool
}

type ExternalSummary struct {
	Added    int
	Updated  int
	Marked   int
	Outcomes []ExternalOutcome
	Skipped  []ExternalSkip
}

func validProvider(p string) bool {
	return p == ProviderSteam || p == ProviderGOG || p == ProviderEpic
}

func ValidLaunchURI(raw string) error {
	if len(raw) > maxLaunchURILen {
		return ErrExternalURIScheme
	}
	for _, r := range raw {
		if r <= ' ' || r == 0x7f {
			return ErrExternalURIScheme
		}
	}
	if steamLaunchURI.MatchString(raw) || epicLaunchURI.MatchString(raw) {
		return nil
	}
	return ErrExternalURIScheme
}

func validateExternal(g ExternalGame) (ExternalGame, error) {
	g.Provider = strings.TrimSpace(g.Provider)
	g.AppID = strings.TrimSpace(g.AppID)
	g.Title = strings.TrimSpace(g.Title)
	g.InstallDir = strings.TrimSpace(g.InstallDir)
	g.Executable = strings.TrimSpace(g.Executable)
	g.LaunchURI = strings.TrimSpace(g.LaunchURI)
	if !validProvider(g.Provider) {
		return g, ErrExternalProvider
	}
	if g.AppID == "" {
		return g, ErrExternalAppID
	}
	if g.Title == "" {
		return g, ErrExternalTitle
	}
	if g.InstallDir == "" {
		return g, errEmptyInstallDir
	}
	if !filepath.IsAbs(g.InstallDir) {
		return g, fmt.Errorf("%w: %s", ErrExternalDir, g.InstallDir)
	}
	info, err := os.Stat(g.InstallDir)
	if err != nil {
		return g, fmt.Errorf("%w: %w", ErrExternalDir, err)
	}
	if !info.IsDir() {
		return g, fmt.Errorf("%w: %s", ErrExternalDir, g.InstallDir)
	}
	if g.Executable != "" {
		info, err := os.Stat(g.Executable)
		if err != nil {
			return g, fmt.Errorf("%w: %w", errNoExecutable, err)
		}
		if info.IsDir() {
			return g, errNoExecutable
		}
		if !platform.Inside(g.InstallDir, g.Executable) {
			return g, errOutsideInstall
		}
	}
	if g.LaunchURI != "" {
		if err := ValidLaunchURI(g.LaunchURI); err != nil {
			return g, err
		}
	}
	if g.Executable == "" && g.LaunchURI == "" {
		return g, ErrExternalNoLaunch
	}
	return g, nil
}

func (s *Service) externalIndexLocked(provider, appID string) int {
	for i := range s.games {
		ext := s.games[i].External
		if ext != nil && ext.Provider == provider && ext.AppID == appID {
			return i
		}
	}
	return -1
}

type measured struct {
	size    int64
	unknown bool
}

// ApplyExternal выполняет upsert по (Provider, AppID). Записи, которых сканер
// больше не видит, не трогает: пометить их пропавшими умеет только SyncExternal.
//
//wails:ignore
func (s *Service) ApplyExternal(ctx context.Context, items []ExternalGame) (ExternalSummary, error) {
	summary, dropped, libCtx, err := s.applyExternal(ctx, items, nil)
	if err != nil {
		return ExternalSummary{}, err
	}
	for _, id := range dropped {
		s.dropSteam(libCtx, id)
	}
	return summary, nil
}

func (s *Service) applyExternal(ctx context.Context, items []ExternalGame, present map[string]map[string]bool) (ExternalSummary, []string, context.Context, error) {
	checked := make([]ExternalGame, 0, len(items))
	for _, item := range items {
		g, err := validateExternal(item)
		if err != nil {
			return ExternalSummary{}, nil, nil, fmt.Errorf("%s: %w", item.Title, err)
		}
		checked = append(checked, g)
	}

	s.mu.Lock()
	needsSize := make(map[int]bool, len(checked))
	for i, g := range checked {
		pos := s.externalIndexLocked(g.Provider, g.AppID)
		needsSize[i] = pos < 0 || s.games[pos].Uninstalled
	}
	s.mu.Unlock()

	sizes := make(map[int]measured, len(checked))
	for i, g := range checked {
		if !needsSize[i] {
			continue
		}
		if err := ctx.Err(); err != nil {
			return ExternalSummary{}, nil, nil, err
		}
		size, unknown := measureInstall("", g.InstallDir)
		sizes[i] = measured{size: size, unknown: unknown}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ExternalSummary{}, nil, nil, errSessionNotRunning
	}
	previous := append([]Game(nil), s.games...)
	var summary ExternalSummary
	now := s.now()
	for i, g := range checked {
		pos := s.externalIndexLocked(g.Provider, g.AppID)
		if pos >= 0 {
			m, ok := sizes[i]
			if !ok {
				m = measured{unknown: true}
			}
			changed := s.updateExternalLocked(pos, g, m)
			if changed {
				summary.Updated++
			}
			summary.Outcomes = append(summary.Outcomes, ExternalOutcome{Provider: g.Provider, AppID: g.AppID, Updated: changed})
			continue
		}
		ignored, err := s.excludedLocked(g.InstallDir)
		if err != nil {
			s.games = previous
			return ExternalSummary{}, nil, nil, err
		}
		if ignored {
			summary.Skipped = append(summary.Skipped, ExternalSkip{Provider: g.Provider, AppID: g.AppID, Title: g.Title, Reason: "каталог исключён из библиотеки"})
			continue
		}
		if dup := s.sameDirLocked(g.InstallDir); dup != "" {
			summary.Skipped = append(summary.Skipped, ExternalSkip{Provider: g.Provider, AppID: g.AppID, Title: g.Title, Reason: "игра уже есть в библиотеке: " + dup})
			continue
		}
		m, ok := sizes[i]
		if !ok {
			m = measured{unknown: true}
		}
		s.games = append(s.games, Game{
			ID:              newID(),
			Title:           g.Title,
			Executable:      g.Executable,
			InstallDir:      g.InstallDir,
			SizeBytes:       m.size,
			SizeUnknown:     m.unknown,
			InstalledAt:     now,
			CanonicalGameID: g.CanonicalGameID,
			Source:          SourceExternal,
			External:        &External{Provider: g.Provider, AppID: g.AppID, LaunchURI: g.LaunchURI},
		})
		summary.Added++
		summary.Outcomes = append(summary.Outcomes, ExternalOutcome{Provider: g.Provider, AppID: g.AppID, Added: true})
	}
	gone := s.markGoneLocked(present)
	summary.Marked = len(gone)
	if summary.Added == 0 && summary.Updated == 0 && summary.Marked == 0 {
		return summary, nil, nil, nil
	}
	if err := s.persist(); err != nil {
		s.games = previous
		return ExternalSummary{}, nil, nil, fmt.Errorf("save library: %w", err)
	}
	if s.dropGoneShortcutsLocked(gone) {
		if err := s.persist(); err != nil {
			return ExternalSummary{}, nil, nil, fmt.Errorf("save library after removing shortcuts: %w", err)
		}
	}
	slog.Info("external games applied", "added", summary.Added, "updated", summary.Updated, "marked", summary.Marked)
	s.emitUpdated()
	dropped := make([]string, 0, len(gone))
	for _, g := range gone {
		dropped = append(dropped, g.ID)
		s.recordUninstalledLocked(g)
	}
	return summary, dropped, s.ctx, nil
}

func (s *Service) updateExternalLocked(pos int, g ExternalGame, m measured) bool {
	cur := &s.games[pos]
	next := *cur
	ext := *cur.External
	ext.LaunchURI = g.LaunchURI
	next.External = &ext
	next.Title = g.Title
	next.InstallDir = g.InstallDir
	next.Executable = g.Executable
	if next.CanonicalGameID == "" {
		next.CanonicalGameID = g.CanonicalGameID
	}
	if cur.Uninstalled {
		next.SizeBytes = m.size
		next.SizeUnknown = m.unknown
	}
	next.Uninstalled = false
	if next.Title == cur.Title && next.InstallDir == cur.InstallDir && next.Executable == cur.Executable &&
		next.CanonicalGameID == cur.CanonicalGameID && next.Uninstalled == cur.Uninstalled && ext == *cur.External {
		return false
	}
	*cur = next
	return true
}

func (s *Service) sameDirLocked(dir string) string {
	for i := range s.games {
		if s.games[i].InstallDir != "" && platform.SamePath(s.games[i].InstallDir, dir) {
			return s.games[i].Title
		}
	}
	return ""
}

func (s *Service) playExternalLocked(game *Game) error {
	uri := game.External.LaunchURI
	if err := ValidLaunchURI(uri); err != nil {
		s.noteLaunchFailureLocked(game.ID, "library.launch_failed", err.Error())
		return uierr.Wrap("library.launch_failed", err)
	}
	open := s.openURI
	if open == nil {
		open = platform.OpenURI
	}
	id, title, provider := game.ID, game.Title, game.External.Provider
	if s.starting == nil {
		s.starting = map[string]context.CancelFunc{}
	}
	s.starting[id] = func() {}
	s.mu.Unlock()
	err := open(uri)
	s.mu.Lock()
	delete(s.starting, id)
	if err != nil {
		s.noteLaunchFailureLocked(id, "library.launch_failed", err.Error())
		return uierr.Wrap("library.launch_failed", fmt.Errorf("не удалось запустить игру через %s: %w", provider, err))
	}
	slog.Info("external game launch requested", "id", id, "title", title, "provider", provider)
	return nil
}
