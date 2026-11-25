package diskusage

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"typhon/internal/diskreason"
	"typhon/internal/download"
	"typhon/internal/platform"
)

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
		if slices.ContainsFunc(g.Footprint(), func(root string) bool { return overlaps(root, path) }) {
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
