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
	// history is the titles the library keeps of removed games, read once
	// the first group of a game without a card needs one.
	history map[string]string
	sets    map[groupKey][]string
	// downloads is the download list as the downloads step read it; the
	// leftover steps judge against the same list.
	downloads []download.Download
}

func (sc *scanner) warn(code string, err error) {
	sc.report.Warnings = append(sc.report.Warnings, Warning{Area: warningArea[code], Code: code, Message: err.Error()})
}

// walkDisk measures everything once. It is the only place that walks the
// disk; the deletions work from its report and ask the owners again before
// they touch anything. The report is dated when the walk starts: it shows the
// disk as it was no earlier than that.
func (s *Service) walkDisk(ctx context.Context) (*scanner, error) {
	startedAt := time.Now()
	cfg := s.deps.Settings()
	games := s.deps.Library.GetGames()
	sc := &scanner{
		ctx:           ctx,
		s:             s,
		gamesRoots:    cfg.GamesRoots(),
		downloadsPath: cfg.DownloadsPath,
		entries:       map[string]entry{},
		sets:          map[groupKey][]string{},
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
			return nil, err
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	sc.report.ScannedAt = startedAt
	sc.sort()
	sc.report.recount()
	return sc, nil
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
	// rootLink: the path itself is a link, measured through its target, so
	// no walk of a folder around it counted these bytes.
	rootLink bool
	err      error
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
			return measured{unknown: true, viaLink: true, rootLink: true}
		}
		if info, err = os.Stat(target); err != nil {
			return measured{unknown: true, viaLink: true, rootLink: true}
		}
		path = target
	}
	if !info.IsDir() {
		return measured{size: info.Size(), viaLink: linked, rootLink: linked}
	}
	if linked {
		// A trailing separator makes the walk open the folder itself instead
		// of stopping at the junction.
		path += string(os.PathSeparator)
	}
	size, inner, err := walkSize(sc.ctx, path)
	if err != nil {
		return measured{unknown: true, viaLink: linked, rootLink: linked, err: sc.ctx.Err()}
	}
	return measured{size: size, viaLink: linked || inner, rootLink: linked}
}

// gameFolder is where a game's measured bytes lie: its folder, or the image
// of an emulated game.
func gameFolder(g library.Game) string {
	if g.Emulation != nil {
		return g.Executable
	}
	return g.InstallDir
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
	return measured{size: g.SizeBytes, unknown: g.SizeUnknown || image.unknown, rootLink: image.rootLink}
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
			counted:     countedAt(gameFolder(g), m),
		})
		if m.unknown || m.missing || m.viaLink {
			continue
		}
		if _, err := sc.s.deps.Library.UpdateMeasuredSize(g.ID, g.InstallDir, m.size, false); err != nil {
			sc.warn(WarnSizeNotSaved, fmt.Errorf("%s: %w", g.Title, err))
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
	sc.downloads = sc.s.deps.Downloads.List()
	for _, d := range sc.downloads {
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
			counted:     countedAt(root, m),
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
	case "install.no_game_list":
		return reasonGamesUnchecked
	case "updates.download_in_use":
		return diskreason.UpdateOwned
	}
	return diskreason.Unknown
}

func (sc *scanner) scanScreenshots() error {
	usage, err := sc.s.deps.Screenshots.Usage(sc.ctx)
	if ctxErr := sc.ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	if err != nil {
		if uierr.Code(err) == screenshotsNoLibrary {
			sc.warn(WarnScreenshotsNoFolder, err)
		} else {
			sc.warn(WarnScreenshotsUnreadable, err)
		}
		return nil
	}
	for _, u := range usage {
		if err := sc.ctx.Err(); err != nil {
			return err
		}
		shots, listErr := sc.s.deps.Screenshots.List(u.GameID)
		sc.addGroup(groupKey{groupShots, u.GameID}, shotItems(shots), listErr,
			GroupUsage{Count: u.Count, SizeBytes: u.SizeBytes, SizeUnknown: u.SizeUnknown})
	}
	return nil
}

func (sc *scanner) scanSnapshots() error {
	usage, err := sc.s.deps.Snapshots.Usage(sc.ctx)
	if ctxErr := sc.ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	if err != nil {
		if uierr.Code(err) == snapshotsDisabled {
			sc.warn(WarnSnapshotsDisabled, err)
		} else {
			sc.warn(WarnSnapshotsUnreadable, err)
		}
		return nil
	}
	for _, u := range usage {
		snaps, listErr := sc.s.deps.Snapshots.List(sc.ctx, u.GameID)
		if err := sc.ctx.Err(); err != nil {
			return err
		}
		sc.addGroup(groupKey{groupSnaps, u.GameID}, snapItems(snaps), listErr,
			GroupUsage{Count: u.Count, SizeBytes: u.SizeBytes, SizeUnknown: u.SizeUnknown})
	}
	return nil
}

func (sc *scanner) group(gameID string, count int, size int64, unknown bool) GroupUsage {
	g := GroupUsage{GameID: gameID, Count: count, SizeBytes: size, SizeUnknown: unknown}
	if game, ok := sc.gameByID[gameID]; ok {
		g.Title, g.InLibrary = game.Title, true
		return g
	}
	if sc.history == nil {
		sc.history = map[string]string{}
		for _, game := range sc.s.deps.Library.GetHistoryGames() {
			if game.Title != "" {
				sc.history[game.ID] = game.Title
			}
		}
	}
	g.Title = sc.history[gameID]
	g.TitleUnknown = g.Title == ""
	return g
}
