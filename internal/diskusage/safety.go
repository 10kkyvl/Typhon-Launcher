package diskusage

import (
	"io/fs"
	"path/filepath"
	"regexp"
	"strings"

	"typhon/internal/diskreason"
	"typhon/internal/relocate"
)

// removalName is the name a removal gives a game folder before it deletes it:
// <dir>.removing or <dir>.removing-N (install's freeRemovalPath).
var removalName = regexp.MustCompile(`\.removing(-\d+)?$`)

// updateStagingDir is the folder the updates keep their staging folders in,
// one per game: it is not a move's staging folder.
const updateStagingDir = ".staging"

// copyKinds names the leftover kind of each folder a move may leave.
var copyKinds = map[string]string{
	relocate.CopyStaging: KindRelocateCopyStaging,
	relocate.CopyTarget:  KindRelocateCopyTarget,
	relocate.CopySource:  KindRelocateCopySource,
}

// scanOrphanedCopies lists the copies of moves that are over. Only the move
// service may delete one, and it offers no reservation for that, so none is
// deletable here; the reason says what the copy may be. onlyCopy false means
// the move service cannot tell, never that another copy exists, so a copy
// whose game's card names no folder on disk (movedOn false) is treated as
// possibly the only one.
func (sc *scanner) scanOrphanedCopies() error {
	copies, err := sc.s.deps.Relocations.OrphanedCopies()
	if err != nil {
		sc.warn(WarnMovesUnchecked, err)
		return nil
	}
	for _, c := range copies {
		if err := sc.ctx.Err(); err != nil {
			return err
		}
		kind, ok := copyKinds[c.Kind]
		if !ok {
			kind = "relocate_copy_" + c.Kind
		}
		code, reason := copyVerdict(c)
		o := entry{
			root:   filepath.Dir(c.Path),
			gameID: c.GameID,
			title:  c.Title,
			check:  func() (bool, string, string) { return false, code, reason },
		}
		if err := sc.addLeftover(c.Path, kind, o); err != nil {
			return err
		}
	}
	return nil
}

func copyVerdict(c relocate.OrphanedCopy) (code, reason string) {
	switch {
	case c.HoldsGame:
		return diskreason.HoldsGame, "в этой папке лежит игра из библиотеки"
	case c.OnlyCopy || !c.MovedOn:
		return diskreason.OnlyCopy, "это может быть единственная копия игры"
	}
	return reasonRelocateCopy, "копию, оставленную переносом, удаляет только перенос"
}

// scanRemovals lists the folders removals renamed before deleting them. The
// launcher finishes the ones its journal names at its next start; the others
// nothing finishes. Install offers no way to delete either from here.
func (sc *scanner) scanRemovals(roots []string) error {
	pending, journalErr := sc.s.deps.Installs.PendingRemovals()
	if journalErr != nil {
		sc.warn(WarnRemovalsUnreadable, journalErr)
	}
	journaled := newPathSet()
	for _, p := range pending {
		journaled.add(p)
	}
	for _, root := range roots {
		err := sc.scanRoot(root, KindRemoval,
			func(e fs.DirEntry) bool { return e.IsDir() && removalName.MatchString(e.Name()) },
			func(path string) entry {
				return entry{check: func() (bool, string, string) {
					switch {
					case sc.holdsGame(path):
						return false, diskreason.HoldsGame, "в этой папке лежит игра из библиотеки"
					case journalErr != nil:
						return false, reasonRemovalsUnreadable, "журнал удалений не читается"
					case journaled.has(path):
						return false, reasonRemovalPending, "удаление завершится при следующем запуске"
					}
					return false, reasonRemovalUnjournaled, "удаление не записано в журнал, его никто не завершит"
				}}
			})
		if err != nil {
			return err
		}
	}
	return nil
}

// scanStrayStagings lists the move staging folders in the games folders that
// no move journal names: the journal was lost. Relocate gives no verdict on
// such a folder, so it is not deletable.
func (sc *scanner) scanStrayStagings(roots []string) error {
	journaled := newPathSet()
	for _, job := range sc.s.deps.Relocations.List() {
		journaled.add(job.Staging)
	}
	for _, root := range roots {
		err := sc.scanRoot(root, KindRelocateStaging,
			func(e fs.DirEntry) bool {
				name := e.Name()
				return e.IsDir() && name != updateStagingDir && strings.HasSuffix(name, stagingSuffix) &&
					!journaled.has(filepath.Join(root, name))
			},
			func(path string) entry {
				return entry{check: func() (bool, string, string) {
					if sc.holdsGame(path) {
						return false, diskreason.HoldsGame, "в этой папке лежит игра из библиотеки"
					}
					return false, reasonRelocateUnjournaled, "ни один перенос не ссылается на эту папку"
				}}
			})
		if err != nil {
			return err
		}
	}
	return nil
}

// holdsGame says whether a card's files lie at, in or around path.
func (sc *scanner) holdsGame(path string) bool {
	for _, g := range sc.games {
		if g.Uninstalled {
			continue
		}
		for _, root := range g.Footprint() {
			if overlaps(root, path) {
				return true
			}
		}
	}
	return false
}
