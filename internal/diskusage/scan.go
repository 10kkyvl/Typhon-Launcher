package diskusage

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"typhon/internal/diskreason"
	"typhon/internal/download"
	"typhon/internal/library"
	"typhon/internal/uierr"
)

const (
	installPartialSuffix = ".partial"
	stagingSuffix        = ".staging"
)

type scanner struct {
	ctx           context.Context
	s             *Service
	gamesRoots    []string
	downloadsPath string
	report        Report
	entries       map[string]entry
	games         []library.Game
	gameByID      map[string]library.Game
}

func (sc *scanner) warn(area string, err error) {
	sc.report.Warnings = append(sc.report.Warnings, Warning{Area: area, Message: err.Error()})
}

// Scan measures everything once and keeps the result for Cached. It is the
// only place that walks the disk; the deletions work from its report and ask
// the owners again before they touch anything.
func (s *Service) Scan(ctx context.Context) (Report, error) {
	ctx, end, err := s.begin(ctx)
	if err != nil {
		return Report{}, err
	}
	defer end()
	s.opMu.Lock()
	defer s.opMu.Unlock()

	cfg := s.deps.Settings()
	games := s.deps.Library.GetGames()
	sc := &scanner{
		ctx:           ctx,
		s:             s,
		gamesRoots:    cfg.GamesRoots(),
		downloadsPath: cfg.DownloadsPath,
		entries:       map[string]entry{},
		games:         games,
		gameByID:      make(map[string]library.Game, len(games)),
		report: Report{
			Games:       []GameUsage{},
			Downloads:   []DownloadUsage{},
			Leftovers:   []Leftover{},
			Screenshots: []GroupUsage{},
			Snapshots:   []GroupUsage{},
			Warnings:    []Warning{},
		},
	}
	for _, g := range games {
		sc.gameByID[g.ID] = g
	}

	steps := []func() error{sc.scanGames, sc.scanDownloads, sc.scanLeftovers, sc.scanScreenshots, sc.scanSnapshots}
	for _, step := range steps {
		if err := step(); err != nil {
			return Report{}, err
		}
	}
	if err := ctx.Err(); err != nil {
		return Report{}, err
	}
	sc.report.ScannedAt = time.Now()
	sc.sort()
	sc.report.recount()

	s.mu.Lock()
	stored := sc.report.clone()
	s.report = &stored
	s.entries = sc.entries
	s.mu.Unlock()
	return sc.report.clone(), nil
}

func (sc *scanner) sort() {
	r := &sc.report
	slices.SortStableFunc(r.Games, func(a, b GameUsage) int { return cmpSize(a.SizeBytes, b.SizeBytes, a.Title, b.Title) })
	slices.SortStableFunc(r.Downloads, func(a, b DownloadUsage) int { return cmpSize(a.SizeBytes, b.SizeBytes, a.Name, b.Name) })
	slices.SortStableFunc(r.Leftovers, func(a, b Leftover) int { return cmpSize(a.SizeBytes, b.SizeBytes, a.Path, b.Path) })
	slices.SortStableFunc(r.Screenshots, func(a, b GroupUsage) int { return cmpSize(a.SizeBytes, b.SizeBytes, a.Title, b.Title) })
	slices.SortStableFunc(r.Snapshots, func(a, b GroupUsage) int { return cmpSize(a.SizeBytes, b.SizeBytes, a.Title, b.Title) })
}

func cmpSize(a, b int64, nameA, nameB string) int {
	switch {
	case a > b:
		return -1
	case a < b:
		return 1
	}
	return strings.Compare(nameA, nameB)
}

type measured struct {
	size    int64
	unknown bool
	missing bool
	viaLink bool
	err     error
}

// measure sizes a file or folder. A missing path is its own answer and any
// other failure is an unknown size, never a zero; only cancellation is an error.
func (sc *scanner) measure(path string) measured {
	if path == "" {
		return measured{unknown: true}
	}
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return measured{missing: true}
	}
	if err != nil {
		return measured{unknown: true}
	}
	linked := info.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0
	if linked {
		// The size of a link is the size of the link, not of what it points
		// at: measure the target, and say the number came through a link.
		target, evalErr := filepath.EvalSymlinks(path)
		if evalErr != nil {
			return measured{unknown: true, viaLink: true}
		}
		if info, err = os.Stat(target); err != nil {
			return measured{unknown: true, viaLink: true}
		}
		path = target
	}
	if !info.IsDir() {
		return measured{size: info.Size(), viaLink: linked}
	}
	if linked {
		// A trailing separator makes the walk open the folder itself instead
		// of stopping at the junction.
		path += string(os.PathSeparator)
	}
	size, inner, err := walkSize(sc.ctx, path)
	if err != nil {
		return measured{unknown: true, viaLink: linked, err: sc.ctx.Err()}
	}
	return measured{size: size, viaLink: linked || inner}
}

// measureGame takes the stored size of an emulated game: its folder is whatever
// the image lies in, and only the library knows the tracks of a .cue. The image
// itself is still looked at, so a moved file shows as missing.
func (sc *scanner) measureGame(g library.Game) measured {
	if g.Emulation == nil {
		return sc.measure(g.InstallDir)
	}
	image := sc.measure(g.Executable)
	if image.missing || image.err != nil {
		return image
	}
	return measured{size: g.SizeBytes, unknown: g.SizeUnknown || image.unknown}
}

func (sc *scanner) scanGames() error {
	for _, g := range sc.games {
		if err := sc.ctx.Err(); err != nil {
			return err
		}
		if g.Uninstalled {
			continue
		}
		m := sc.measureGame(g)
		if m.err != nil {
			return m.err
		}
		sc.report.Games = append(sc.report.Games, GameUsage{
			ID:          g.ID,
			Title:       g.Title,
			InstallDir:  g.InstallDir,
			SizeBytes:   m.size,
			SizeUnknown: m.unknown,
			Missing:     m.missing,
			Linked:      m.viaLink,
			External:    g.External != nil,
			LastPlayed:  g.LastPlayed,
			InstalledAt: g.InstalledAt,
			Running:     sc.s.deps.Library.IsRunning(g.ID),
			Busy:        sc.s.deps.Installs.GameBusy(g),
		})
		if m.unknown || m.missing || m.viaLink {
			continue
		}
		if _, err := sc.s.deps.Library.UpdateMeasuredSize(g.ID, g.InstallDir, m.size, false); err != nil {
			sc.warn("games", fmt.Errorf("%s: %w", g.Title, err))
		}
	}
	return nil
}

func (sc *scanner) scanDownloads() error {
	owners := map[string]string{}
	for _, g := range sc.games {
		if g.SourceDownloadID != "" && !g.Uninstalled {
			owners[g.SourceDownloadID] = g.ID
		}
	}
	for _, d := range sc.s.deps.Downloads.List() {
		if err := sc.ctx.Err(); err != nil {
			return err
		}
		root, ok := d.DataRoot()
		if !ok {
			continue
		}
		m := sc.measure(root)
		if m.err != nil {
			return m.err
		}
		usage := DownloadUsage{
			ID:          d.ID,
			Name:        d.Name,
			Root:        root,
			SizeBytes:   m.size,
			SizeUnknown: m.unknown || m.viaLink,
			Missing:     m.missing,
			Seeding:     d.Seeding,
			GameID:      owners[d.ID],
			Installed:   owners[d.ID] != "",
		}
		switch checkErr := sc.s.deps.Installs.CheckDownloadDeletable(d); {
		case d.Status != download.StatusCompleted:
			usage.Referenced, usage.Reason, usage.ReasonCode = true, "загрузка не завершена", diskreason.Incomplete
		case checkErr != nil:
			usage.Referenced, usage.Reason, usage.ReasonCode = true, checkErr.Error(), downloadReasonCode(checkErr)
		case d.Seeding:
			usage.Reason, usage.ReasonCode = "файлы раздаются, при удалении раздача остановится", diskreason.Seeding
		}
		sc.report.Downloads = append(sc.report.Downloads, usage)
	}
	return nil
}

func downloadReasonCode(err error) string {
	switch uierr.Code(err) {
	case "install.download_in_use":
		return diskreason.OwnedByInstall
	case "install.download_is_install":
		return diskreason.HoldsGame
	}
	return diskreason.Unknown
}

func (sc *scanner) scanScreenshots() error {
	usage, err := sc.s.deps.Screenshots.Usage(sc.ctx)
	if ctxErr := sc.ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	if err != nil {
		sc.warn("screenshots", err)
		return nil
	}
	for _, u := range usage {
		sc.report.Screenshots = append(sc.report.Screenshots, sc.group(u.GameID, u.Count, u.SizeBytes, u.SizeUnknown))
	}
	return nil
}

func (sc *scanner) scanSnapshots() error {
	usage, err := sc.s.deps.Snapshots.Usage(sc.ctx)
	if ctxErr := sc.ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	if err != nil {
		sc.warn("snapshots", err)
		return nil
	}
	for _, u := range usage {
		sc.report.Snapshots = append(sc.report.Snapshots, sc.group(u.GameID, u.Count, u.SizeBytes, u.SizeUnknown))
	}
	return nil
}

func (sc *scanner) group(gameID string, count int, size int64, unknown bool) GroupUsage {
	g := GroupUsage{GameID: gameID, Count: count, SizeBytes: size, SizeUnknown: unknown}
	if game, ok := sc.gameByID[gameID]; ok {
		g.Title, g.InLibrary = game.Title, true
	}
	return g
}
