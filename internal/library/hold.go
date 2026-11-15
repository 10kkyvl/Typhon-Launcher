package library

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"typhon/internal/platform"
	"typhon/internal/uierr"
)

var (
	ErrFolderShared = uierr.New("library.folder_shared", "в папке игры лежит другая игра библиотеки")
	ErrFolderBusy   = uierr.New("library.folder_busy", "эту папку сейчас удаляют или переносят вместе с другой игрой")
	ErrGameRemoving = uierr.New("library.removing", "игра удаляется")
	ErrGameMoving   = uierr.New("library.moving", "игра переносится на другой диск, дождитесь конца переноса")
	// ErrFolderChanged: the card names another folder than the one the caller
	// decided on, and no move holds it now (a move ended, or an install or a
	// scan gave the card another folder). Asking again sees the new folder.
	ErrFolderChanged = fmt.Errorf("%w: the card names another folder", errInstallationChanged)
)

// FolderNeighbour is another library game whose files lie in, around or at a
// game's folder: deleting or moving that folder would take them along.
type FolderNeighbour struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type holdKind int

const (
	holdRemove holdKind = iota
	holdMove
	holdAdd
)

// folderHold bars new cards from a folder while the files in it are deleted or
// moved (holdRemove, holdMove), and bars the deletion or move of a folder a card
// is being added in (holdAdd).
type folderHold struct {
	kind  holdKind
	game  string
	title string
	dir   string
}

// overlaps says whether dir is one of roots, lies inside one or holds one. The
// paths are compared by key, without touching the disk: it runs under s.mu. A
// path whose key cannot be made counts as overlapping, because the answer
// decides whether files may be deleted or moved. A link is not followed: a
// card reached through a symlinked parent or a junction is compared by the
// path it was added under.
func overlaps(roots []string, dir string) bool {
	if dir == "" {
		return false
	}
	key, err := platform.PathKey(dir)
	if err != nil {
		return true
	}
	sep := string(filepath.Separator)
	for _, root := range roots {
		if root == "" {
			continue
		}
		rootKey, err := platform.PathKey(root)
		if err != nil {
			return true
		}
		if rootKey == key || strings.HasPrefix(key, rootKey+sep) || strings.HasPrefix(rootKey, key+sep) {
			return true
		}
	}
	return false
}

// neighboursLocked counts the cards that own files on disk: an uninstalled card
// keeps its old folder only as a hint for the next install, and an archived one
// is no longer in s.games and claims no folder at all.
func (s *Service) neighboursLocked(id, dir string) []FolderNeighbour {
	var out []FolderNeighbour
	for _, g := range s.games {
		if g.ID == id || g.Uninstalled {
			continue
		}
		if overlaps(g.Footprint(), dir) {
			out = append(out, FolderNeighbour{ID: g.ID, Title: g.Title})
		}
	}
	for _, h := range s.holds {
		if h.kind == holdAdd && h.game != id && overlaps([]string{h.dir}, dir) {
			out = append(out, FolderNeighbour{ID: h.game, Title: h.title})
		}
	}
	return out
}

func sharedError(neighbours []FolderNeighbour) error {
	titles := make([]string, 0, len(neighbours))
	for _, n := range neighbours {
		titles = append(titles, n.Title)
	}
	return fmt.Errorf("%w: %s", ErrFolderShared, strings.Join(titles, ", "))
}

// FolderNeighbours lists the other games whose files deleting or moving this
// game's folder would take along, for the dialogs to name them.
func (s *Service) FolderNeighbours(id string) ([]FolderNeighbour, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	game := s.findLocked(id)
	if game == nil {
		return nil, errNotFound
	}
	return s.neighboursLocked(id, game.InstallDir), nil
}

func (s *Service) reservedLocked(id string) error {
	if _, ok := s.relocating[id]; ok {
		return ErrGameRelocating
	}
	if _, ok := s.removing[id]; ok {
		return ErrGameRemoving
	}
	return nil
}

// BeginRemoval reserves a game for an uninstall: it fails while the game runs,
// starts, moves or is already being removed. dir is the folder the removal acts
// on, by deleting it or by running an uninstaller in it, and empty when it
// touches no file; with a dir it also fails while another card's files share
// that folder, and with ErrFolderChanged when the card no longer names dir (a
// move ended, or an install or a scan rebound the card, since the caller
// looked); a move in progress is ErrGameRelocating. Until release, PlayGame
// and the move reservations refuse the game, and no card may be added inside
// dir. The checks and the reservation are one critical section of s.mu.
//
//wails:ignore
func (s *Service) BeginRemoval(id, dir string) (func(), error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	game := s.findLocked(id)
	if game == nil {
		return nil, errNotFound
	}
	if err := s.reservedLocked(id); err != nil {
		return nil, err
	}
	if _, ok := s.running[id]; ok || s.starting[id] != nil {
		return nil, ErrRelocateGameRunning
	}
	if dir != "" {
		if !platform.SamePath(game.InstallDir, dir) {
			return nil, fmt.Errorf("%w: %s", ErrFolderChanged, game.InstallDir)
		}
		if neighbours := s.neighboursLocked(id, dir); len(neighbours) > 0 {
			return nil, sharedError(neighbours)
		}
	}
	if s.removing == nil {
		s.removing = map[string]struct{}{}
	}
	s.removing[id] = struct{}{}
	hold := s.addHoldLocked(folderHold{kind: holdRemove, game: id, dir: dir})
	var once sync.Once
	return func() {
		once.Do(func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			delete(s.removing, id)
			s.dropHoldLocked(hold)
		})
	}, nil
}

// BeginMove is BeginRelocate for a move that has not touched any file yet: it
// also fails while another card's files share the game's folder, and holds the
// folder against new cards until release.
//
//wails:ignore
func (s *Service) BeginMove(id string) (func(), error) {
	return s.reserveMove(id, true, nil)
}

// reserveMove reserves a game for a move that acts on dirs, and on the card's
// own folder when own is set: it fails while the game runs, starts, moves or
// is being removed, or while another card's files share one of those folders,
// and holds them against new cards until release. The checks and the
// reservation are one critical section of s.mu.
func (s *Service) reserveMove(id string, own bool, dirs []string) (func(), error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	game := s.findLocked(id)
	if game == nil {
		return nil, errNotFound
	}
	if err := s.reservedLocked(id); err != nil {
		return nil, err
	}
	if _, ok := s.running[id]; ok || s.starting[id] != nil {
		return nil, ErrRelocateGameRunning
	}
	if own {
		dirs = append([]string{game.InstallDir}, dirs...)
	}
	for _, dir := range dirs {
		if neighbours := s.neighboursLocked(id, dir); len(neighbours) > 0 {
			return nil, sharedError(neighbours)
		}
	}
	if s.relocating == nil {
		s.relocating = map[string]struct{}{}
	}
	s.relocating[id] = struct{}{}
	held := make([]*folderHold, 0, len(dirs))
	for _, dir := range dirs {
		if dir != "" {
			held = append(held, s.addHoldLocked(folderHold{kind: holdMove, game: id, dir: dir}))
		}
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			delete(s.relocating, id)
			for _, hold := range held {
				s.dropHoldLocked(hold)
			}
		})
	}, nil
}

// holdForAdding keeps a removal or a move from deciding on dir while a card is
// being added there or given that folder; it refuses a dir whose files are
// being deleted or moved. Every path that looks at files before it saves a card
// for them (AddGame, RegisterInstalled, the image paths with SetExecutable of an
// image, discovery) holds dir
// from before the look until after the save, so the files it saw are the files
// the card names. id is the card's when the path knows it, empty otherwise.
func (s *Service) holdForAdding(id, title, dir string) (func(), error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, h := range s.holds {
		if h.kind != holdAdd && overlaps([]string{h.dir}, dir) {
			return nil, ErrFolderBusy
		}
	}
	hold := s.addHoldLocked(folderHold{kind: holdAdd, game: id, title: title, dir: dir})
	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.dropHoldLocked(hold)
	}, nil
}

// removingLocked refuses to give the card at pos (-1: none) files again while
// a removal holds it: the removal settles the card last, and an install or a
// scan that rebound it in between would be marked uninstalled or removed. Every
// path that binds a card to files asks it under the lock it saves the card in:
// RegisterInstalled, discovery, RegisterEmulated, and reviveLocked and
// bindImageLocked for AddGame, AddEmulatedGame and SetExecutable of an image.
func (s *Service) removingLocked(pos int) error {
	if pos < 0 {
		return nil
	}
	if _, ok := s.removing[s.games[pos].ID]; ok {
		return ErrGameRemoving
	}
	return nil
}

func (s *Service) addHoldLocked(h folderHold) *folderHold {
	held := &h
	s.holds = append(s.holds, held)
	return held
}

func (s *Service) dropHoldLocked(held *folderHold) {
	for i, h := range s.holds {
		if h == held {
			s.holds = append(s.holds[:i:i], s.holds[i+1:]...)
			return
		}
	}
}

// checkHoldsLocked is the last line for every way a card gains a folder
// (adding, discovery, installs, sync, relocation): persist refuses a state in
// which a card other than the holder lies in a folder being deleted or moved.
// A hold is only granted with no such card, so a refusal here is always the
// mutation being persisted, and its caller rolls it back.
func (s *Service) checkHoldsLocked() error {
	for _, h := range s.holds {
		if h.kind == holdAdd || h.dir == "" {
			continue
		}
		for _, g := range s.games {
			if g.ID != h.game && !g.Uninstalled && overlaps(g.Footprint(), h.dir) {
				return fmt.Errorf("%w: %s", ErrFolderBusy, g.Title)
			}
		}
	}
	return nil
}
