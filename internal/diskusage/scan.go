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
	"typhon/internal/platform"
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

func (sc *scanner) scanGames() error {
	for _, g := range sc.games {
		if err := sc.ctx.Err(); err != nil {
			return err
		}
		if g.Uninstalled {
			continue
		}
		m := sc.measure(g.InstallDir)
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
		case d.Seeding:
			usage.Referenced, usage.Reason, usage.ReasonCode = true, "файлы раздаются", diskreason.Seeding
		case checkErr != nil:
			usage.Referenced, usage.Reason, usage.ReasonCode = true, checkErr.Error(), downloadReasonCode(checkErr)
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

// scanLeftovers looks only in folders the launcher itself writes to: the
// games and downloads folders, the parents of known installs and the exact
// paths the update flow derives from a game.
func (sc *scanner) scanLeftovers() error {
	partialRoots := newPathSet()
	partRoots := newPathSet()
	for _, root := range sc.gamesRoots {
		partialRoots.add(root)
	}
	partRoots.add(sc.downloadsPath)
	for _, item := range sc.s.deps.Installs.List() {
		if item.Destination != "" {
			partialRoots.add(filepath.Dir(item.Destination))
		}
	}
	for _, g := range sc.games {
		if g.InstallDir != "" && g.External == nil {
			partialRoots.add(filepath.Dir(g.InstallDir))
		}
	}
	for _, d := range sc.s.deps.Downloads.List() {
		partRoots.add(d.Destination)
	}

	for _, root := range partialRoots.out {
		err := sc.scanRoot(root, KindInstallPartial,
			func(e fs.DirEntry) bool { return e.IsDir() && strings.HasSuffix(e.Name(), installPartialSuffix) },
			func(path string) entry {
				return entry{
					check:   func() (bool, string, string) { return sc.s.deps.Installs.PartialVerdict(path) },
					reserve: func() (func(), error) { return sc.s.deps.Installs.ReservePartial(path) },
				}
			})
		if err != nil {
			return err
		}
	}
	for _, root := range partRoots.out {
		err := sc.scanRoot(root, KindDownloadPart,
			func(e fs.DirEntry) bool { return download.IsPartFile(e.Name()) },
			func(path string) entry {
				return entry{check: func() (bool, string, string) { return sc.s.deps.Downloads.PartFileVerdict(path) }}
			})
		if err != nil {
			return err
		}
	}
	if err := sc.scanStagings(); err != nil {
		return err
	}
	if err := sc.scanOrphans(); err != nil {
		return err
	}
	return sc.scanUpdateDirs()
}

func (sc *scanner) scanRoot(root, kind string, match func(fs.DirEntry) bool, owner func(path string) entry) error {
	dirEntries, err := os.ReadDir(root)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		sc.warn("leftovers", err)
		return nil
	}
	for _, e := range dirEntries {
		if err := sc.ctx.Err(); err != nil {
			return err
		}
		if !match(e) {
			continue
		}
		path := filepath.Join(root, e.Name())
		o := owner(path)
		o.root = root
		if err := sc.addLeftover(path, kind, o); err != nil {
			return err
		}
	}
	return nil
}

func (sc *scanner) scanStagings() error {
	stagings, err := sc.s.deps.Relocations.Stagings()
	if err != nil {
		sc.warn("leftovers", err)
		return nil
	}
	for _, st := range stagings {
		path := st.Path
		check := func() (bool, string, string) {
			if !strings.HasSuffix(filepath.Base(path), stagingSuffix) {
				return false, diskreason.RelocateJournal, "имя папки не похоже на временную папку переноса"
			}
			current, err := sc.s.deps.Relocations.Stagings()
			if err != nil {
				return false, diskreason.Unreadable, err.Error()
			}
			for _, c := range current {
				if platform.SamePath(c.Path, path) {
					return c.Deletable, c.Code, c.Reason
				}
			}
			return false, diskreason.RelocateJournal, "перенос больше не ссылается на эту папку"
		}
		o := entry{
			root:  filepath.Dir(path),
			check: check,
			reserve: func() (func(), error) {
				if ok, _, reason := check(); !ok {
					return nil, fmt.Errorf("%w: %s", errNotDeletable, reason)
				}
				return sc.s.deps.Relocations.ReserveStaging(path)
			},
		}
		if err := sc.addLeftover(path, KindRelocateStaging, o); err != nil {
			return err
		}
	}
	return nil
}

// scanOrphans lists the files that a deleted download left on disk; the
// download manager wrote them down before it dropped the record.
func (sc *scanner) scanOrphans() error {
	for _, c := range sc.s.deps.Downloads.PendingCleanups() {
		if err := sc.ctx.Err(); err != nil {
			return err
		}
		path := c.Path
		check := func() (bool, string, string) { return sc.orphanVerdict(path) }
		o := entry{
			root:  filepath.Dir(path),
			check: check,
			// The claim bars new downloads, the second check closes the gap
			// between the first one and the claim for everything else.
			reserve: func() (func(), error) {
				if ok, _, reason := check(); !ok {
					return nil, fmt.Errorf("%w: %s", errNotDeletable, reason)
				}
				release, err := sc.s.deps.Downloads.ReserveCleanup(path)
				if err != nil {
					return nil, err
				}
				if ok, _, reason := check(); !ok {
					release()
					return nil, fmt.Errorf("%w: %s", errNotDeletable, reason)
				}
				return release, nil
			},
			remove: func() error { return sc.s.deps.Downloads.RemoveCleanup(path) },
		}
		if err := sc.addLeftover(path, KindDownloadOrphan, o); err != nil {
			return err
		}
	}
	return nil
}

// orphanVerdict is the full ownership check for files a deleted download left
// behind. The list only says they were once a download's; since then a game
// may have been installed there, or an install or update started to use them.
func (sc *scanner) orphanVerdict(path string) (bool, string, string) {
	if ok, code, reason := sc.s.deps.Downloads.CleanupVerdict(path); !ok {
		return false, code, reason
	}
	for _, g := range sc.s.deps.Library.GetGames() {
		if g.InstallDir != "" && overlaps(g.InstallDir, path) {
			return false, diskreason.HoldsGame, "в этой папке лежит игра из библиотеки"
		}
	}
	if sc.s.deps.Installs.PathInUse(path) {
		return false, diskreason.OwnedByInstall, "в этой папке работает установка"
	}
	if sc.s.deps.Updates.PathInUse(path) {
		return false, diskreason.UpdateOwned, "в этой папке работает обновление"
	}
	return true, "", ""
}

func overlaps(a, b string) bool {
	return platform.SamePath(a, b) || platform.Inside(a, b) || platform.Inside(b, a)
}

// scanUpdateDirs reports the folders an update leaves next to a game. They
// belong to the update and rollback machinery, which is not asked here, so
// none of them is offered for deletion.
func (sc *scanner) scanUpdateDirs() error {
	for _, g := range sc.games {
		if g.InstallDir == "" || g.External != nil {
			continue
		}
		paths := []struct{ path, kind string }{
			{filepath.Join(filepath.Dir(g.InstallDir), ".staging", g.ID), KindUpdateStaging},
			{g.InstallDir + ".previous", KindUpdatePrevious},
			{g.InstallDir + ".replaced", KindUpdateReplaced},
		}
		for _, p := range paths {
			if _, err := os.Lstat(p.path); err != nil {
				if !errors.Is(err, fs.ErrNotExist) {
					sc.warn("leftovers", err)
				}
				continue
			}
			o := entry{
				root: filepath.Dir(p.path),
				check: func() (bool, string, string) {
					return false, diskreason.UpdateOwned, "управляется обновлением игры"
				},
			}
			if err := sc.addLeftover(p.path, p.kind, o); err != nil {
				return err
			}
		}
	}
	return nil
}

func (sc *scanner) addLeftover(path, kind string, o entry) error {
	key, err := platform.PathKey(path)
	if err != nil {
		sc.warn("leftovers", err)
		return nil
	}
	if _, dup := sc.entries[key]; dup {
		return nil
	}
	l := Leftover{Path: path, Kind: kind}
	info, err := os.Lstat(path)
	switch {
	case err != nil && errors.Is(err, fs.ErrNotExist):
		return nil
	case err != nil:
		l.SizeUnknown = true
		l.Reason, l.ReasonCode = "не удалось прочитать: "+err.Error(), diskreason.Unreadable
	case info.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0:
		l.Reason, l.ReasonCode = "ссылка, а не папка с данными", diskreason.Symlink
	default:
		m := sc.measure(path)
		if m.err != nil {
			return m.err
		}
		l.SizeBytes, l.SizeUnknown = m.size, m.unknown || m.viaLink
		l.Deletable, l.ReasonCode, l.Reason = o.check()
		if l.Deletable {
			l.ReasonCode, l.Reason = "", ""
		} else if l.ReasonCode == "" {
			l.ReasonCode = diskreason.Unknown
		}
	}
	sc.entries[key] = o
	sc.report.Leftovers = append(sc.report.Leftovers, l)
	return nil
}

type pathSet struct {
	seen map[string]bool
	out  []string
}

func newPathSet() *pathSet { return &pathSet{seen: map[string]bool{}} }

func (p *pathSet) add(path string) {
	if path == "" || !filepath.IsAbs(path) {
		return
	}
	key, err := platform.PathKey(path)
	if err != nil || p.seen[key] {
		return
	}
	p.seen[key] = true
	p.out = append(p.out, filepath.Clean(path))
}
