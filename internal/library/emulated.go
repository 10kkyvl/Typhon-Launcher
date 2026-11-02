package library

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"typhon/internal/emulators"
	"typhon/internal/platform"
	"typhon/internal/storage"
	"typhon/internal/uierr"
)

const (
	emulatedFileName = "library-emulated.json"
	emulatedVersion  = 1
	maxImageListSize = 1 << 20

	// InstallTypeImage is the install type of an image the launcher put into
	// a folder of its own.
	InstallTypeImage = "console_image"
)

var (
	errCoverInvalid = uierr.New("library.cover_invalid", "обложка должна быть адресом http или https")
	errImageOutside = uierr.New("library.image_outside_dir", "образ лежит вне папки установки")

	// ErrImageList says a .cue or .m3u could not be read, which is not the same
	// as there being no image.
	ErrImageList = errors.New("не удалось прочитать список файлов образа")

	// ErrAlreadyAdded lets the install refuse a duplicate before it copies
	// anything.
	ErrAlreadyAdded = errAlreadyAdded
)

var imageStat = os.Lstat

// A variable because rights cannot be taken away from a file portably in a test.
var loadEmulatedFile = storage.Load

// Emulation marks a game started through an emulator. Executable then holds the
// image and InstallDir its folder, which keeps every "the file lies inside the
// game's folder" check true. The folder is not the game's: it can be Downloads,
// so nothing may treat it as an installation, unless the launcher made it for the
// image (Owned with InstallTypeImage). Files are the other files the
// image lists (tracks of a .cue, discs of an .m3u). The emulator is not stored
// here, it is resolved at launch.
//
// Such games live in library-emulated.json, not library.json: an older launcher
// must not read one as a plain game whose folder is Downloads.
type Emulation struct {
	Platform string   `json:"platform"`
	Files    []string `json:"files,omitempty"`
	// CatalogGameID links the card to the catalog entry it was installed from,
	// for the game page and nothing else. It is deliberately not CanonicalGameID,
	// which would attach the image to the PC game's sync and cloud saves.
	CatalogGameID string `json:"catalogGameId,omitempty"`
	// TitleID is set once the emulator has copied the game into its own storage
	// (see emulators.Platform.RemembersInstall) and is started by this id.
	TitleID string `json:"titleId,omitempty"`
}

// Footprint is what the game's data occupies on disk. For an emulated game that
// is its files, not the folder they happen to lie in.
func (g Game) Footprint() []string {
	if g.Emulation != nil {
		files := append([]string{g.Executable}, g.Emulation.Files...)
		if g.Owned && g.InstallType == InstallTypeImage {
			return append([]string{g.InstallDir}, files...)
		}
		return files
	}
	if g.InstallDir == "" {
		return nil
	}
	return []string{g.InstallDir}
}

func emulatedPathFor(path string) string {
	return filepath.Join(filepath.Dir(path), emulatedFileName)
}

// loadEmulated follows invariant 3: only a missing file means "no games".
func loadEmulated(path string) ([]Game, error) {
	var games []Game
	err := loadEmulatedFile(path, emulatedVersion, nil, &games)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read emulated games %s: %w", path, err)
	}
	for _, game := range games {
		if game.Emulation == nil || game.InstallDir == "" || game.Executable == "" {
			return nil, fmt.Errorf("read emulated games %s: incomplete record %q", path, game.ID)
		}
	}
	return games, nil
}

// dropRemovalStubs settles a process killed inside the removal of an emulated
// game (invariant 9). Removal writes the stub to library.json first, so a stub
// whose game is still in the second file means the second write never happened.
// The game stays; the stub only repeats its record and would double its hours.
// Nothing is written here: the next persist rewrites library.json.
func dropRemovalStubs(games, emulated []Game) []Game {
	alive := make(map[string]struct{}, len(emulated))
	for _, game := range emulated {
		alive[game.ID] = struct{}{}
	}
	return slices.DeleteFunc(games, func(game Game) bool {
		_, twin := alive[game.ID]
		if game.Archived && twin {
			slog.Warn("drop the stub of an interrupted removal", "id", game.ID)
			return true
		}
		return false
	})
}

func splitEmulated(games []Game) (native, emulated []Game) {
	native, emulated = []Game{}, []Game{}
	for _, game := range games {
		if game.Emulation != nil {
			emulated = append(emulated, game)
		} else {
			native = append(native, game)
		}
	}
	return native, emulated
}

func encodeEmulated(games []Game) ([]byte, error) {
	data, err := json.Marshal(games)
	if err != nil {
		return nil, fmt.Errorf("encode emulated games: %w", err)
	}
	return data, nil
}

// AddEmulatedGame adds an image file as a game under the name the user gave it.
// No marker is written: the folder belongs to the user, and a marker there would
// make a later scan take it for an installation.
func (s *Service) AddEmulatedGame(ctx context.Context, romPath, platformID, title string) (Game, error) {
	if err := ctx.Err(); err != nil {
		return Game{}, err
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return Game{}, emulators.ErrTitleEmpty
	}
	rom, dir, target, err := checkImage(romPath, platformID)
	if err != nil {
		return Game{}, err
	}
	facts, err := readImage(rom, ctx.Err)
	if err != nil {
		return Game{}, err
	}
	if target.IsGameFile(rom) {
		facts.size, facts.unknown = measureInstall(ctx, "", dir)
	}
	game := Game{
		ID:          newID(),
		Title:       title,
		Executable:  rom,
		InstallDir:  dir,
		SizeBytes:   facts.size,
		SizeUnknown: facts.unknown,
		InstalledAt: time.Now(),
		Source:      SourceManaged,
		Emulation:   &Emulation{Platform: target.ID, Files: facts.files},
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.games {
		if !platform.SamePath(s.games[i].Executable, rom) {
			continue
		}
		if !s.games[i].Uninstalled || s.games[i].Emulation == nil {
			return Game{}, errAlreadyAdded
		}
		return s.bindImageLocked(i, title, rom, dir, target.ID, facts)
	}
	s.games = append(s.games, game)
	if err := s.persist(); err != nil {
		s.games = s.games[:len(s.games)-1]
		return Game{}, fmt.Errorf("save library: %w", err)
	}
	slog.Info("emulated game added", "id", game.ID, "title", game.Title, "platform", target.ID)
	s.emitUpdated()
	return game, nil
}

// InstalledImage is an image the install service has put on disk. CanonicalGameID
// stays out on purpose: the catalog id belongs to the PC game, and carrying it
// would attach the card to that game's page, sync and cloud saves.
type InstalledImage struct {
	Title            string `json:"title"`
	Cover            string `json:"cover"`
	CatalogGameID    string `json:"catalogGameId"`
	Platform         string `json:"platform"`
	Image            string `json:"image"`
	InstallDir       string `json:"installDir"`
	Owned            bool   `json:"owned"`
	ReleaseID        string `json:"releaseId"`
	SourceID         string `json:"sourceId"`
	SourceDownloadID string `json:"sourceDownloadId"`
	Version          string `json:"version"`
}

// RegisterEmulated records an image installed by the launcher, under the checks
// of AddEmulatedGame. The same folder is the same install: a process killed
// between this write and closing its job comes back here on startup and must not
// fail on its own card. Another folder with the same release is refused rather
// than rebound: the old folder would lose its card and with it the only way to
// delete it. A card left without files comes back with its playtime.
func (s *Service) RegisterEmulated(ctx context.Context, g InstalledImage) (Game, error) {
	if err := ctx.Err(); err != nil {
		return Game{}, err
	}
	title := strings.TrimSpace(g.Title)
	if title == "" {
		return Game{}, emulators.ErrTitleEmpty
	}
	dir := strings.TrimSpace(g.InstallDir)
	if !filepath.IsAbs(dir) {
		return Game{}, errEmptyInstallDir
	}
	dir = filepath.Clean(dir)
	if g.Cover != "" && !emulators.ValidCover(g.Cover) {
		return Game{}, errCoverInvalid
	}
	rom, _, target, err := checkImage(g.Image, g.Platform)
	if err != nil {
		return Game{}, err
	}
	if !platform.Inside(dir, rom) {
		return Game{}, fmt.Errorf("%w: %s", errImageOutside, rom)
	}
	facts, err := readImage(rom, ctx.Err)
	if err != nil {
		return Game{}, err
	}
	if g.Owned || target.IsGameFile(rom) {
		// The folder is the game's here, disc two the list does not name included.
		facts.size, facts.unknown = measureInstall(ctx, "", dir)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	pos, err := s.imageCardLocked(g, rom, dir, title, target.ID)
	if err != nil {
		return Game{}, err
	}
	if pos < 0 {
		s.games = append(s.games, Game{ID: newID(), InstalledAt: time.Now()})
		pos = len(s.games) - 1
		fillImageCard(&s.games[pos], g, title, rom, dir, target.ID, facts)
		if err := s.persist(); err != nil {
			s.games = s.games[:pos]
			return Game{}, fmt.Errorf("save library: %w", err)
		}
	} else {
		previous := s.games[pos]
		fillImageCard(&s.games[pos], g, title, rom, dir, target.ID, facts)
		if err := s.persist(); err != nil {
			s.games[pos] = previous
			return Game{}, fmt.Errorf("save library: %w", err)
		}
	}
	slog.Info("console image installed", "id", s.games[pos].ID, "title", title, "platform", target.ID)
	s.emitUpdated()
	return s.games[pos], nil
}

// imageCardLocked finds the card this image lands on: -1 means a new one. A card
// without files matches by name too, because removal forgets its release.
func (s *Service) imageCardLocked(g InstalledImage, rom, dir, title, platformID string) (int, error) {
	match := -1
	for i := range s.games {
		game := &s.games[i]
		sameImage := platform.SamePath(game.Executable, rom)
		if game.Emulation == nil || game.Emulation.Platform != platformID {
			if sameImage {
				return -1, fmt.Errorf("%w: %s", errAlreadyAdded, game.Title)
			}
			continue
		}
		if game.Uninstalled {
			if match < 0 && (sameImage || game.HoldsRelease("", "", g.SourceDownloadID) || strings.EqualFold(game.Title, title)) {
				match = i
			}
			continue
		}
		if game.Owned && platform.SamePath(game.InstallDir, dir) {
			return i, nil
		}
		if sameImage || game.HoldsRelease(g.ReleaseID, g.SourceID, g.SourceDownloadID) {
			return -1, fmt.Errorf("%w: %s", errAlreadyAdded, game.Title)
		}
	}
	return match, nil
}

func fillImageCard(game *Game, g InstalledImage, title, rom, dir, platformID string, facts imageFacts) {
	game.Title = title
	if g.Cover != "" {
		game.Cover = g.Cover
	}
	game.Executable = rom
	game.InstallDir = dir
	game.Uninstalled = false
	game.Version = g.Version
	game.SizeBytes, game.SizeUnknown = facts.size, facts.unknown
	game.SourceDownloadID = g.SourceDownloadID
	game.ReleaseID = g.ReleaseID
	game.SourceID = g.SourceID
	game.Source = SourceManaged
	game.Owned = g.Owned
	game.InstallType = ""
	if g.Owned {
		game.InstallType = InstallTypeImage
	}
	game.Emulation = &Emulation{Platform: platformID, Files: facts.files, CatalogGameID: g.CatalogGameID}
}

// HoldsRelease says whether the card was installed from this release or this
// download.
func (g Game) HoldsRelease(releaseID, sourceID, downloadID string) bool {
	if releaseID != "" && g.ReleaseID == releaseID && g.SourceID == sourceID {
		return true
	}
	return downloadID != "" && g.SourceDownloadID == downloadID
}

// ImageMembers returns the files an image lists. complete is false when the list
// names a file it should not; a list that cannot be read is ErrImageList.
func ImageMembers(rom string, alive func() error) (files []string, complete bool, err error) {
	return followList(rom, alive)
}

// bindImageLocked points a card at an image, a new folder being the image's game
// folder. An empty title keeps the old name. The title id goes with the old image:
// the emulator installs what the card now points at.
func (s *Service) bindImageLocked(pos int, title, rom, folder, platformID string, facts imageFacts) (Game, error) {
	previous := s.games[pos]
	s.games[pos].Emulation = &Emulation{Platform: platformID, Files: facts.files}
	if previous.Emulation != nil && previous.Owned && platform.Inside(previous.InstallDir, rom) {
		s.games[pos].Emulation.CatalogGameID = previous.Emulation.CatalogGameID
	}
	s.games[pos].Uninstalled = false
	s.games[pos].Executable = rom
	if !previous.Owned || !platform.Inside(previous.InstallDir, rom) {
		// A card that followed its file elsewhere must not delete that folder.
		s.games[pos].InstallDir = folder
		s.games[pos].Owned = false
		s.games[pos].InstallType = ""
		clearInstalledProvenance(&s.games[pos])
		s.games[pos].SourceDownloadID = ""
	}
	s.games[pos].SizeBytes, s.games[pos].SizeUnknown = facts.size, facts.unknown
	if title != "" {
		s.games[pos].Title = title
	}
	if err := s.persist(); err != nil {
		s.games[pos] = previous
		return Game{}, fmt.Errorf("save library: %w", err)
	}
	s.emitUpdated()
	return s.games[pos], nil
}

func (s *Service) emulatedPlatform(id string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	game := s.findLocked(id)
	if game == nil || game.Emulation == nil {
		return "", false
	}
	return game.Emulation.Platform, true
}

// rebindImage is SetExecutable of an emulated game: a moved file comes back to
// its card, with the playtime, under the same rules as a newly added image.
func (s *Service) rebindImage(id, path, platformID string) (Game, error) {
	rom, folder, target, err := checkImage(path, platformID)
	if err != nil {
		return Game{}, err
	}
	facts, err := readImage(rom, func() error { return nil })
	if err != nil {
		return Game{}, err
	}
	if target.IsGameFile(rom) {
		s.mu.Lock()
		ctx := s.ctx
		s.mu.Unlock()
		// Before startup there is no context to measure a folder under.
		facts.size, facts.unknown = 0, true
		if ctx != nil {
			facts.size, facts.unknown = measureInstall(ctx, "", folder)
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	pos := -1
	for i := range s.games {
		if s.games[i].ID == id {
			pos = i
		} else if platform.SamePath(s.games[i].Executable, rom) {
			return Game{}, errAlreadyAdded
		}
	}
	if pos < 0 {
		return Game{}, errNotFound
	}
	if s.games[pos].Emulation == nil {
		return Game{}, errInstallationChanged
	}
	return s.bindImageLocked(pos, "", rom, folder, target.ID, facts)
}

// FindEmulatedCover runs only when the user asks for it, with the name the user
// sees on the card. Adding a game sends nothing anywhere.
func (s *Service) FindEmulatedCover(ctx context.Context, id string) (Game, error) {
	s.mu.Lock()
	game := s.findLocked(id)
	if game == nil {
		s.mu.Unlock()
		return Game{}, errNotFound
	}
	if game.Emulation == nil {
		s.mu.Unlock()
		return Game{}, emulators.ErrUnsupported
	}
	title, platformID, search := game.Title, game.Emulation.Platform, s.coverSearch
	s.mu.Unlock()

	cover, err := search(ctx, title, platformID)
	if err != nil {
		return Game{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	game = s.findLocked(id)
	if game == nil {
		return Game{}, errNotFound
	}
	previous := *game
	game.Cover = cover
	if err := s.persist(); err != nil {
		*game = previous
		return Game{}, fmt.Errorf("save library: %w", err)
	}
	s.emitUpdated()
	return *game, nil
}

// SetCoverSearch wires the catalog lookup behind FindEmulatedCover.
//
//wails:ignore
func (s *Service) SetCoverSearch(fn func(ctx context.Context, title, platform string) (string, error)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.coverSearch = fn
}

// checkImage returns the cleaned path and the folder of the whole game.
func checkImage(romPath, platformID string) (string, string, emulators.Platform, error) {
	refuse := func(err error) (string, string, emulators.Platform, error) {
		return "", "", emulators.Platform{}, err
	}
	target, ok := emulators.Lookup(platformID)
	if !ok {
		return refuse(fmt.Errorf("%w: %q", emulators.ErrUnknownPlatform, platformID))
	}
	rom := strings.TrimSpace(romPath)
	if rom == "" {
		return refuse(emulators.ErrImagePathEmpty)
	}
	if !filepath.IsAbs(rom) {
		return refuse(fmt.Errorf("%w: %s", emulators.ErrImagePathRelative, rom))
	}
	rom = filepath.Clean(rom)
	if target.ManualOnly(rom) {
		return refuse(fmt.Errorf("%w: %s", emulators.ErrImageManualOnly, filepath.Ext(rom)))
	}
	if !target.AcceptsImage(rom) {
		return refuse(fmt.Errorf("%w: %s", emulators.ErrImageExtension, filepath.Ext(rom)))
	}
	info, err := imageStat(rom)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return refuse(fmt.Errorf("%w: %s", emulators.ErrImageMissing, rom))
	case err != nil:
		return refuse(fmt.Errorf("%w: %w", emulators.ErrImageUnreadable, err))
	case !info.Mode().IsRegular():
		return refuse(fmt.Errorf("%w: %s", emulators.ErrImageNotFile, rom))
	}
	folder, err := target.GameFolder(rom)
	if err != nil {
		return refuse(err)
	}
	return rom, folder, target, nil
}

type imageFacts struct {
	size    int64
	unknown bool
	files   []string
}

// readImage measures the game, not the folder it lies in. A list that cannot be
// followed to the end gives an unknown size, not a part of it.
func readImage(rom string, alive func() error) (imageFacts, error) {
	info, err := imageStat(rom)
	if err != nil {
		return imageFacts{}, fmt.Errorf("%w: %w", emulators.ErrImageUnreadable, err)
	}
	files, complete, err := listedFiles(rom, alive)
	if err != nil {
		return imageFacts{}, err
	}
	facts := imageFacts{size: info.Size(), files: files}
	for _, file := range files {
		if err := alive(); err != nil {
			return imageFacts{}, err
		}
		info, err := imageStat(file)
		if err != nil {
			complete = false
			continue
		}
		facts.size += info.Size()
	}
	if !complete {
		facts.size, facts.unknown = 0, true
	}
	return facts, nil
}

// listedFiles is followList for a file the user chose: a list that cannot be read
// costs the card its size, not the card. Only a cancelled alive is an error.
func listedFiles(rom string, alive func() error) (files []string, complete bool, err error) {
	files, complete, err = followList(rom, alive)
	if errors.Is(err, ErrImageList) {
		slog.Warn("read image list", "error", err)
		return files, false, nil
	}
	return files, complete, err
}

// followList returns the files an image names. Names come from a file the user
// downloaded (invariant 32): only those inside the image's folder are taken, and
// any other makes the list incomplete. A list that cannot be read or names
// nothing is ErrImageList; a disc of an .m3u that is simply absent only makes the
// list incomplete.
func followList(rom string, alive func() error) (files []string, complete bool, err error) {
	ext := strings.ToLower(filepath.Ext(rom))
	if ext != ".cue" && ext != ".m3u" {
		return nil, true, nil
	}
	if err := alive(); err != nil {
		return nil, false, err
	}
	names, err := readImageList(rom, ext)
	if err != nil {
		return nil, false, fmt.Errorf("%w: %s: %w", ErrImageList, filepath.Base(rom), err)
	}
	if len(names) == 0 {
		return nil, false, fmt.Errorf("%w: %s: в списке нет файлов", ErrImageList, filepath.Base(rom))
	}
	dir := filepath.Dir(rom)
	complete = true
	for _, name := range names {
		local := filepath.FromSlash(strings.ReplaceAll(name, `\`, "/"))
		if !filepath.IsLocal(local) {
			complete = false
			continue
		}
		path := filepath.Join(dir, local)
		files = append(files, path)
		if ext == ".m3u" && strings.EqualFold(filepath.Ext(path), ".cue") {
			if _, statErr := imageStat(path); errors.Is(statErr, fs.ErrNotExist) {
				complete = false
				continue
			}
			tracks, whole, err := followList(path, alive)
			if err != nil {
				return files, false, err
			}
			files = append(files, tracks...)
			complete = complete && whole
		}
	}
	return files, complete, nil
}

func readImageList(path, ext string) (names []string, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()
	r := io.LimitReader(f, maxImageListSize)
	if ext == ".m3u" {
		return playlistFiles(r)
	}
	return cueFiles(r)
}

func playlistFiles(r io.Reader) ([]string, error) {
	var names []string
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := listLine(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") && !slices.Contains(names, line) {
			names = append(names, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read playlist: %w", err)
	}
	return names, nil
}

// listLine drops the byte order mark Windows editors put first: with it the
// opening FILE of a sheet is not recognised and the sheet looks empty.
func listLine(raw string) string {
	return strings.TrimSpace(strings.TrimPrefix(raw, "\ufeff"))
}

func cueFiles(r io.Reader) ([]string, error) {
	var names []string
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := listLine(scanner.Text())
		if len(line) < 5 || !strings.EqualFold(line[:5], "FILE ") {
			continue
		}
		rest := strings.TrimSpace(line[5:])
		var name string
		if after, ok := strings.CutPrefix(rest, `"`); ok {
			name, _, ok = strings.Cut(after, `"`)
			if !ok {
				return nil, errors.New("unterminated file name in cue sheet")
			}
		} else {
			name, _, _ = strings.Cut(rest, " ")
		}
		if name != "" && !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read cue sheet: %w", err)
	}
	return names, nil
}

// emulatedLaunch turns the game's launch into the emulator's. It runs outside
// the library mutex: the resolver touches the disk (invariant 18). The player's
// own arguments go first: the emulators read options only before the image.
// The working folder is the emulator's on every console: shadPS4 keeps its data
// beside the working folder, so anything else would give it an empty install.
func emulatedLaunch(ctx context.Context, resolve func(string) (string, error), game Game, chosen chosenProfile, req *launch) error {
	target, ok := emulators.Lookup(game.Emulation.Platform)
	if !ok {
		return fmt.Errorf("%w: %q", emulators.ErrUnknownPlatform, game.Emulation.Platform)
	}
	program, err := resolve(game.Emulation.Platform)
	if err != nil {
		return err
	}
	workDir, err := filepath.Abs(filepath.Dir(program))
	if err != nil {
		return fmt.Errorf("рабочая папка эмулятора: %w", err)
	}
	command, run, err := emulatorCommand(ctx, target, game, chosen.executable)
	if err != nil {
		return err
	}
	req.executable = program
	req.args = append(slices.Clone(chosen.args), command...)
	req.workDir = workDir
	req.elevated = false
	req.emulator = run
	return nil
}

// emulatorRun is what the end of a session needs to know about how it started.
type emulatorRun struct {
	// titleID is what the emulator was asked to install or to start.
	titleID string
	// installed means it was started by titleID, not given the image.
	installed bool
	image     string
}

func emulatorCommand(ctx context.Context, target emulators.Platform, game Game, image string) ([]string, emulatorRun, error) {
	if !target.RemembersInstall() {
		return target.Command(image), emulatorRun{}, nil
	}
	run := emulatorRun{titleID: game.Emulation.TitleID, installed: true, image: image}
	if run.titleID != "" {
		command, err := target.InstalledCommand(run.titleID)
		return command, run, err
	}
	info, err := target.ReadGameInfo(ctx, image)
	if err != nil {
		return nil, emulatorRun{}, err
	}
	run.titleID, run.installed = info.TitleID, false
	return target.Command(image), run, nil
}

// settleInstalledTitle says what the card remembers after a session. An install
// counts only after the emulator outlived the quick exit and closed cleanly by
// itself: anything else may have cut the copy short, and repeating an install
// costs nothing. The id goes when a start by id fails at once: the emulator no
// longer has the game, and the next launch installs it. The result is a new
// value: copies of the card given out earlier share the old pointer.
func settleInstalledTitle(game Game, run emulatorRun, quick, clean, failed bool) *Emulation {
	next := *game.Emulation
	switch {
	case run.titleID == "":
		return game.Emulation
	case run.installed && quick && failed && next.TitleID == run.titleID:
		next.TitleID = ""
	case !run.installed && !quick && clean && next.TitleID == "" && platform.SamePath(game.Executable, run.image):
		next.TitleID = run.titleID
	default:
		return game.Emulation
	}
	return &next
}

// MissingImages lists the emulated games whose image file is gone, for the cards
// to offer rebinding. The disk is read outside the mutex (invariant 18). A file
// that cannot be checked is not reported as gone: it may be only unreadable.
func (s *Service) MissingImages(ctx context.Context) ([]string, error) {
	type image struct{ id, path string }
	s.mu.Lock()
	var images []image
	for i := range s.games {
		if s.games[i].Emulation != nil && !s.games[i].Uninstalled {
			images = append(images, image{s.games[i].ID, s.games[i].Executable})
		}
	}
	s.mu.Unlock()
	missing := []string{}
	for _, img := range images {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		_, err := imageStat(img.path)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			missing = append(missing, img.id)
		case err != nil:
			slog.Warn("check emulated image", "id", img.id, "error", err)
		}
	}
	return missing, nil
}

// SetEmulatorResolver wires the lookup of the program for a console. The
// library knows the table of consoles but not where the user keeps emulators.
//
//wails:ignore
func (s *Service) SetEmulatorResolver(fn func(platform string) (string, error)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.emulator = fn
}

// GetPlayableGames is everything that can be launched. GetInstalledGames leaves
// out emulated games; this is for callers that only show games to the player.
//
//wails:ignore
func (s *Service) GetPlayableGames() []Game {
	s.mu.Lock()
	defer s.mu.Unlock()
	games := make([]Game, 0, len(s.games))
	for _, game := range s.games {
		if !game.Uninstalled {
			games = append(games, game)
		}
	}
	return games
}

// FindNative is Find for services that treat InstallDir as the game's own
// folder and would otherwise write to, scan or run files from any folder an
// image happens to lie in.
//
//wails:ignore
func (s *Service) FindNative(id string) (Game, error) {
	game, err := s.Find(id)
	if err != nil {
		return Game{}, err
	}
	if game.Emulation != nil {
		return Game{}, emulators.ErrUnsupported
	}
	return game, nil
}
