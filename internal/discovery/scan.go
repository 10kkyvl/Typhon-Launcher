package discovery

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"typhon/internal/catalog"
	"typhon/internal/install"
	"typhon/internal/library"
	"typhon/internal/platform"
	"typhon/internal/titles"
)

const progressInterval = 250 * time.Millisecond

type candidate struct {
	path string
	name string
}

type stepKind int

const (
	stepKnown stepKind = iota
	stepAdded
	stepUpdated
	stepSkipped
)

type step struct {
	kind      stepKind
	canonical string
	reason    string
}

type knownIndex struct {
	byPath map[string]library.Game
	byID   map[string]library.Game
}

func (s *Service) run(ctx context.Context) (Result, error) {
	roots, err := s.roots()
	if err != nil {
		return Result{}, err
	}
	idx, err := s.index()
	if err != nil {
		return Result{}, err
	}

	result := Result{}
	candidates := make([]candidate, 0, 16)
	for _, root := range roots {
		found, err := listCandidates(root, &result)
		if err != nil {
			result.RootsSkipped++
			if errors.Is(err, fs.ErrNotExist) {
				result.note(root, "папка не найдена")
				continue
			}
			result.fail(root, err.Error())
			continue
		}
		result.Roots++
		candidates = append(candidates, found...)
	}
	result.Candidates = len(candidates)

	slog.Info("discovery started", "roots", result.Roots, "candidates", result.Candidates)
	emit(eventStarted, Progress{Total: result.Candidates})

	provisioned := make([]string, 0, len(candidates))
	last := time.Now()
	for i, c := range candidates {
		if ctx.Err() != nil {
			result.Cancelled = true
			break
		}
		outcome, err := s.process(ctx, c, idx)
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			result.Cancelled = true
			break
		}
		if err != nil {
			result.fail(c.path, err.Error())
			slog.Warn("discovery candidate", "path", c.path, "error", err)
		} else {
			apply(&result, c, outcome)
			if outcome.canonical != "" {
				provisioned = append(provisioned, outcome.canonical)
			}
		}
		if now := time.Now(); now.Sub(last) >= progressInterval || i+1 == len(candidates) {
			last = now
			emit(eventProgress, Progress{Processed: i + 1, Total: len(candidates)})
		}
	}

	s.resolveMetadata(provisioned, &result)
	slog.Info("discovery finished", "added", result.Added, "updated", result.Updated,
		"known", result.Known, "skipped", result.Skipped, "errors", result.Errors, "cancelled", result.Cancelled)
	emit(eventCompleted, result)
	return result, nil
}

func apply(result *Result, c candidate, outcome step) {
	switch outcome.kind {
	case stepAdded:
		result.Added++
	case stepUpdated:
		result.Updated++
	case stepSkipped:
		result.Skipped++
		result.note(c.path, outcome.reason)
	default:
		result.Known++
	}
}

func (s *Service) index() (knownIndex, error) {
	games := s.library.GetInstalledGames()
	idx := knownIndex{
		byPath: make(map[string]library.Game, len(games)),
		byID:   make(map[string]library.Game, len(games)),
	}
	for _, game := range games {
		idx.byID[game.ID] = game
		if game.InstallDir == "" {
			continue
		}
		key, err := platform.PathKey(game.InstallDir)
		if err != nil {
			return knownIndex{}, fmt.Errorf("normalize %s: %w", game.InstallDir, err)
		}
		idx.byPath[key] = game
	}
	return idx, nil
}

func listCandidates(root string, result *Result) ([]candidate, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("read dir %s: %w", root, err)
	}
	out := make([]candidate, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		path := filepath.Join(root, name)
		if strings.HasPrefix(name, ".") {
			continue
		}
		// Симлинк или junction ведёт куда угодно, в том числе на другой диск;
		// обходим только то, что физически лежит внутри настроенного корня.
		// Windows отдаёт junction как irregular-запись, а не как ссылку.
		if entry.Type()&(fs.ModeSymlink|fs.ModeIrregular) != 0 {
			result.Skipped++
			result.note(path, "ссылка ведёт за пределы папки игр")
			continue
		}
		if !entry.IsDir() {
			continue
		}
		if strings.HasSuffix(strings.ToLower(name), ".partial") {
			continue
		}
		out = append(out, candidate{path: path, name: name})
	}
	return out, nil
}

func (s *Service) process(ctx context.Context, c candidate, idx knownIndex) (step, error) {
	marker, err := library.ReadMarker(c.path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		// Испорченная метка не отменяет каталог: дальше работает обычный поиск.
		slog.Warn("read install marker", "path", c.path, "error", err)
		marker = library.Marker{}
	}

	key, err := platform.PathKey(c.path)
	if err != nil {
		return step{}, fmt.Errorf("normalize %s: %w", c.path, err)
	}
	if game, ok := idx.byPath[key]; ok {
		return s.refresh(ctx, c, game, marker)
	}
	if marker.GameID != "" {
		if game, ok := idx.byID[marker.GameID]; ok {
			return s.refresh(ctx, c, game, marker)
		}
		return s.adopt(ctx, c, marker)
	}
	return s.discover(ctx, c)
}

func (s *Service) refresh(ctx context.Context, c candidate, game library.Game, marker library.Marker) (step, error) {
	executable, err := s.executableFor(ctx, c, game.Executable, marker, game.Title)
	if err != nil {
		return step{}, err
	}
	if platform.SamePath(game.InstallDir, c.path) && strings.EqualFold(game.Executable, executable) {
		return step{kind: stepKnown}, nil
	}
	return s.store(library.Discovered{
		GameID:          game.ID,
		Title:           game.Title,
		Executable:      executable,
		InstallDir:      c.path,
		Version:         game.Version,
		VersionSource:   game.VersionSource,
		CanonicalGameID: game.CanonicalGameID,
		SizeBytes:       game.SizeBytes,
		SizeUnknown:     game.SizeUnknown,
	}, "")
}

func (s *Service) adopt(ctx context.Context, c candidate, marker library.Marker) (step, error) {
	title := strings.TrimSpace(marker.Title)
	if title == "" {
		title = titleOf(c.name)
	}
	executable, err := s.executableFor(ctx, c, "", marker, title)
	if err != nil {
		return step{}, err
	}
	size, err := install.DirSize(ctx, c.path)
	if err != nil {
		return step{}, err
	}
	canonical := marker.CanonicalGameID
	provisioned := ""
	if canonical == "" {
		canonical, provisioned = s.canonical(title, titles.Parse(c.name).Year)
	}
	return s.store(library.Discovered{
		Title:           title,
		Executable:      executable,
		InstallDir:      c.path,
		Version:         marker.Version,
		VersionSource:   marker.VersionSource,
		CanonicalGameID: canonical,
		SizeBytes:       size,
	}, provisioned)
}

func (s *Service) discover(ctx context.Context, c candidate) (step, error) {
	parsed := titles.Parse(c.name)
	title := parsed.Base
	if title == "" {
		title = titleOf(c.name)
	}
	candidates, installed, err := install.LooksInstalled(ctx, c.path, title)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return step{kind: stepSkipped, reason: "папка исчезла во время поиска"}, nil
		}
		return step{}, err
	}
	if !installed {
		return step{kind: stepSkipped, reason: "не похоже на установленную игру"}, nil
	}
	executable := ""
	if install.HighConfidence(candidates) {
		executable = candidates[0].Path
	}
	size, err := install.DirSize(ctx, c.path)
	if err != nil {
		return step{}, err
	}
	canonical, provisioned := s.canonical(title, parsed.Year)
	return s.store(library.Discovered{
		Title:           title,
		Executable:      executable,
		InstallDir:      c.path,
		CanonicalGameID: canonical,
		SizeBytes:       size,
	}, provisioned)
}

func (s *Service) store(d library.Discovered, provisioned string) (step, error) {
	_, outcome, err := s.library.ApplyDiscovered(d)
	if err != nil {
		return step{}, err
	}
	switch outcome {
	case library.OutcomeCreated:
		return step{kind: stepAdded, canonical: provisioned}, nil
	case library.OutcomeUpdated:
		return step{kind: stepUpdated, canonical: provisioned}, nil
	default:
		return step{kind: stepKnown}, nil
	}
}

func (s *Service) executableFor(ctx context.Context, c candidate, current string, marker library.Marker, title string) (string, error) {
	present, err := fileExists(current)
	if err != nil {
		return "", err
	}
	if present {
		return current, nil
	}
	fromMarker := marker.MarkerExecutable(c.path)
	present, err = fileExists(fromMarker)
	if err != nil {
		return "", err
	}
	if present {
		return fromMarker, nil
	}
	return s.detect(ctx, c.path, title)
}

func (s *Service) detect(ctx context.Context, dir, title string) (string, error) {
	candidates, err := install.FindExecutables(ctx, dir, title)
	if err != nil {
		return "", err
	}
	if !install.HighConfidence(candidates) {
		return "", nil
	}
	return candidates[0].Path, nil
}

func (s *Service) canonical(title string, year int) (string, string) {
	normalized := titles.Normalize(title)
	if normalized == "" {
		return "", ""
	}
	query := catalog.Query{Title: title, Normalized: normalized, Year: year}
	match := s.catalog.Resolve(query)
	if match.Status == catalog.StatusMatched && match.GameID != "" {
		return match.GameID, ""
	}
	games := s.catalog.Provision([]catalog.Query{query})
	game, ok := games[normalized]
	if !ok {
		return "", ""
	}
	return game.ID, game.ID
}

func fileExists(path string) (bool, error) {
	if path == "" {
		return false, nil
	}
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("stat %s: %w", path, err)
	}
	return !info.IsDir(), nil
}

func titleOf(name string) string {
	cleaned := strings.NewReplacer("_", " ", ".", " ").Replace(name)
	return strings.TrimSpace(cleaned)
}
