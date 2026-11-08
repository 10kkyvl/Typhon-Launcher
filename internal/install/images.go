package install

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"typhon/internal/emulators"
	"typhon/internal/hashdir"
	"typhon/internal/library"
	"typhon/internal/settings"
	"typhon/internal/uierr"
)

const (
	// ConsolesFolder lies in a games folder and holds images, not a game.
	ConsolesFolder = "Consoles"
	maxImageDepth  = 5
	maxImageTitle  = 200
)

var (
	errImageNotFound    = uierr.New("install.image_not_found", "в загрузке не найден образ игры для этой консоли")
	errImageUnreadable  = uierr.New("install.image_unreadable", "не удалось прочитать список файлов образа игры (.cue или .m3u)")
	errImageAmbiguous   = uierr.New("install.image_ambiguous", "в загрузке лежат образы разных игр, выбрать один автоматически нельзя")
	errImageOptions     = uierr.New("install.image_options_invalid", "неверные данные для установки образа игры")
	errImageDestination = uierr.New("install.image_destination", "образ игры ставится только в папку Consoles каталога игр")
	errImageUpdateOnly  = uierr.New("install.image_update_only", "в загрузке только обновление игры, самой игры нет")
	errImageAddonOnly   = uierr.New("install.image_addon_only", "в загрузке только дополнение к игре, самой игры нет")
)

// Seams: a mismatch of two hashes and a refused rename of one file cannot be
// produced on a healthy disk.
var (
	hashImageFile   = hashdir.HashFile
	renameImageFile = os.Rename
)

// discMark finds "Disc 2", "(CD1)", "_disk_b" in a file name: the only sign that
// two images are one game and not two.
var discMark = regexp.MustCompile(`(?i)(?:^|[\s._\-(\[])(?:disc|disk|cd|dvd)[\s._\-]*(\d{1,2}|[a-d])(?:\s*of\s*\d+)?(?:$|[\s._\-)\]])`)

// imageRanks is the order in which a file starts a game: a list names the rest,
// and a bare track is the last resort. Anything else the console accepts ranks
// below these.
var imageRanks = map[string]int{".m3u": 0, ".cue": 1, ".chd": 2, ".pbp": 3, ".iso": 4, ".cso": 4, ".zso": 4}

const lastImageRank = 5

type imageFile struct {
	path    string
	ext     string
	rank    int
	size    int64
	members []string
}

// FindImages picks the file the emulator is given. Every other file stays where
// it is. A choice it cannot justify is an error, not a guess.
func FindImages(ctx context.Context, dir, platformID string) (string, error) {
	target, ok := emulators.Lookup(platformID)
	if !ok {
		return "", fmt.Errorf("%w: %q", emulators.ErrUnknownPlatform, platformID)
	}
	found, manual, err := imageFiles(ctx, dir, target)
	if err != nil {
		return "", err
	}
	if target.HasGameFiles() {
		return pickGame(ctx, found, manual, target)
	}
	usable, err := usableImages(ctx, found)
	if err != nil {
		return "", err
	}
	return pickImage(usable)
}

// imageFiles also says whether it saw a form of the console's games that only the
// emulator itself can install.
func imageFiles(ctx context.Context, dir string, target emulators.Platform) ([]imageFile, bool, error) {
	var found []imageFile
	manual := false
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("обход %s: %w", path, err)
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		hidden := strings.HasPrefix(d.Name(), ".")
		if d.IsDir() {
			if path == dir {
				return nil
			}
			rel, err := filepath.Rel(dir, path)
			if err != nil {
				return fmt.Errorf("путь %s: %w", path, err)
			}
			if hidden || target.SkipsFolder(d.Name()) || strings.Count(rel, string(filepath.Separator)) >= maxImageDepth {
				return fs.SkipDir
			}
			return nil
		}
		manualForm := target.ManualOnly(path)
		if hidden || !d.Type().IsRegular() || (!manualForm && !target.AcceptsImage(path)) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("stat %s: %w", path, err)
		}
		if info.Size() == 0 {
			return nil
		}
		if manualForm {
			manual = true
			return nil
		}
		if _, err := target.GameFolder(path); errors.Is(err, emulators.ErrImageIncomplete) {
			return nil
		} else if err != nil {
			return err
		}
		ext := strings.ToLower(filepath.Ext(path))
		rank, ranked := imageRanks[ext]
		if !ranked {
			rank = lastImageRank
		}
		found = append(found, imageFile{path: path, ext: ext, rank: rank, size: info.Size()})
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return found, manual, nil
}

type foundGame struct {
	path  string
	title string
	disc  bool
}

// pickGame chooses among games of a console that describe themselves. Each file
// that stays after what follows is a game of its own: size cannot tell a second
// game from a small file here, so two are never settled by a guess.
func pickGame(ctx context.Context, found []imageFile, manual bool, target emulators.Platform) (string, error) {
	games := make([]foundGame, 0, len(found))
	updates, addons := false, false
	for _, file := range outermostGames(found, target) {
		info, err := target.ReadGameInfo(ctx, file.path)
		if err != nil {
			return "", err
		}
		if target.IsUpdate(info) {
			updates = true
			continue
		}
		if target.IsAddon(info) {
			addons = true
			continue
		}
		games = append(games, foundGame{path: file.path, title: info.TitleID, disc: target.InDiscFolder(file.path)})
	}
	games = preferDisc(games)
	switch {
	case len(games) == 1:
		return games[0].path, nil
	case len(games) > 1:
		return "", fmt.Errorf("%w: найдено игр: %d", errImageAmbiguous, len(games))
	case updates:
		return "", errImageUpdateOnly
	case addons:
		return "", errImageAddonOnly
	case manual:
		return "", emulators.ErrImageManualOnly
	}
	return "", errImageNotFound
}

// outermostGames drops what lies inside the folder of an unpacked game: a game
// carries its own tools and extras, and those are not games of the download.
func outermostGames(found []imageFile, target emulators.Platform) []imageFile {
	outer := make([]imageFile, 0, len(found))
	for _, file := range found {
		nested := slices.ContainsFunc(found, func(other imageFile) bool {
			return other.path != file.path && target.IsGameFile(other.path) &&
				!samePath(target.GameDir(other.path), target.GameDir(file.path)) &&
				inside(target.GameDir(other.path), file.path)
		})
		if !nested {
			outer = append(outer, file)
		}
	}
	return outer
}

// preferDisc keeps a disc over another copy of the same game. A folder with an id
// of its own is another game, and that stays for the caller to refuse.
func preferDisc(games []foundGame) []foundGame {
	discs := map[string]bool{}
	for _, game := range games {
		if game.disc && game.title != "" {
			discs[game.title] = true
		}
	}
	return slices.DeleteFunc(games, func(game foundGame) bool { return !game.disc && discs[game.title] })
}

// usableImages drops what cannot start a game: a list with a missing file, and a
// bare track once a cue sheet is present, even a broken one, because a track
// without its sheet loses the audio and the layout of the disc.
func usableImages(ctx context.Context, found []imageFile) ([]imageFile, error) {
	sheet := slices.ContainsFunc(found, func(f imageFile) bool { return f.ext == ".cue" })
	usable := make([]imageFile, 0, len(found))
	for _, file := range found {
		if sheet && (file.ext == ".bin" || file.ext == ".img") {
			continue
		}
		if file.ext != ".cue" && file.ext != ".m3u" {
			usable = append(usable, file)
			continue
		}
		members, complete, err := library.ImageMembers(file.path, ctx.Err)
		if errors.Is(err, library.ErrImageList) {
			return nil, fmt.Errorf("%w: %w", errImageUnreadable, err)
		}
		if err != nil {
			return nil, err
		}
		if !complete {
			continue
		}
		size, whole, err := membersSize(members)
		if err != nil {
			return nil, err
		}
		if !whole {
			continue
		}
		file.members = members
		file.size += size
		usable = append(usable, file)
	}
	return usable, nil
}

func membersSize(members []string) (int64, bool, error) {
	var total int64
	for _, member := range members {
		info, err := os.Lstat(member)
		if errors.Is(err, fs.ErrNotExist) {
			return 0, false, nil
		}
		if err != nil {
			return 0, false, fmt.Errorf("stat %s: %w", member, err)
		}
		if !info.Mode().IsRegular() {
			return 0, false, nil
		}
		total += info.Size()
	}
	return total, true, nil
}

func pickImage(files []imageFile) (string, error) {
	if len(files) == 0 {
		return "", errImageNotFound
	}
	best := files[0].rank
	for _, file := range files {
		best = min(best, file.rank)
	}
	var chosen imageFile
	for _, file := range files {
		if file.rank == best && (chosen.path == "" || file.size > chosen.size || (file.size == chosen.size && file.path < chosen.path)) {
			chosen = file
		}
	}
	chosen = firstDisc(chosen, files)
	for _, file := range files {
		if file.path == chosen.path || slices.ContainsFunc(chosen.members, func(m string) bool { return samePath(m, file.path) }) {
			continue
		}
		if sameGameDisc(file, chosen) {
			continue
		}
		if float64(file.size) >= float64(chosen.size)*dominantFraction {
			return "", fmt.Errorf("%w: найдено несколько крупных образов", errImageAmbiguous)
		}
	}
	return chosen.path, nil
}

// firstDisc walks from the chosen file to disc one of its set. Size says nothing
// here: the last disc of a game is often the largest.
func firstDisc(chosen imageFile, files []imageFile) imageFile {
	stem, disc, ok := discOf(chosen.path)
	if !ok {
		return chosen
	}
	for _, file := range files {
		if file.rank != chosen.rank {
			continue
		}
		other, number, ok := discOf(file.path)
		if ok && other == stem && (number < disc || (number == disc && file.path < chosen.path)) {
			chosen, disc = file, number
		}
	}
	return chosen
}

func sameGameDisc(a, b imageFile) bool {
	first, _, ok := discOf(a.path)
	if !ok {
		return false
	}
	second, _, ok := discOf(b.path)
	return ok && first == second
}

// discOf splits a file name into the name of the set and the number of the disc.
func discOf(path string) (string, int, bool) {
	dir := filepath.Dir(path)
	name := strings.ToLower(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
	if set, number, ok := splitDisc(name); ok {
		return filepath.Join(dir, set), number, true
	}
	// Discs in folders of their own: "Disc 1/game.cue", "Disc 2/game.cue".
	if set, number, ok := splitDisc(strings.ToLower(filepath.Base(dir))); ok {
		return filepath.Join(filepath.Dir(dir), set, name), number, true
	}
	return "", 0, false
}

func splitDisc(name string) (string, int, bool) {
	match := discMark.FindStringSubmatchIndex(name)
	if match == nil {
		return "", 0, false
	}
	token := name[match[2]:match[3]]
	number, err := strconv.Atoi(token)
	if err != nil {
		number = int(token[0]-'a') + 1
	}
	return strings.TrimSpace(name[:match[0]] + " " + name[match[1]:]), number, true
}

// InspectImage is Inspect for a download the user declared a console image: no
// installer is looked for, and .bin is a track rather than installer data.
func InspectImage(ctx context.Context, path, platformID string) (Plan, error) {
	target, ok := emulators.Lookup(platformID)
	if !ok {
		return Plan{}, fmt.Errorf("%w: %q", emulators.ErrUnknownPlatform, platformID)
	}
	info, err := os.Stat(path)
	if err != nil {
		return Plan{}, fmt.Errorf("%w: %w", errNoSource, err)
	}
	if !info.IsDir() {
		return inspectImageFile(ctx, path, info.Size(), target)
	}
	root, err := normalizeRoot(path)
	if err != nil {
		return Plan{}, err
	}
	plan := Plan{Type: TypeImage, SourcePath: path, ContentRoot: root, Platform: target.ID, Candidates: []Candidate{}}
	entries, err := sizedEntries(root)
	if err != nil {
		return Plan{}, err
	}
	if len(entries) == 0 {
		return Plan{}, fmt.Errorf("%w: %s", errEmptySource, root)
	}
	files := make([]sizedEntry, 0, len(entries))
	for _, entry := range entries {
		if !entry.isDir {
			files = append(files, entry)
		}
	}
	if len(files) > 0 && allPartial(files) {
		return Plan{}, fmt.Errorf("%w: %s", errIncompleteSource, root)
	}
	if plan.EstimatedSize, err = DirSize(ctx, root); err != nil {
		return Plan{}, err
	}
	image, err := FindImages(ctx, root, target.ID)
	if err == nil {
		plan.ImagePath = image
		plan.CanAutoInstall = true
		return plan, nil
	}
	if !slices.ContainsFunc([]error{errImageNotFound, errImageUpdateOnly, errImageAddonOnly, emulators.ErrImageManualOnly}, func(refusal error) bool {
		return errors.Is(err, refusal)
	}) {
		return Plan{}, err
	}
	archive := pickDominantArchive(root, files, plan.EstimatedSize)
	if archive == "" {
		return Plan{}, fmt.Errorf("%w: %s", err, filepath.Base(root))
	}
	if err := fillImageArchive(&plan, archive); err != nil {
		return Plan{}, err
	}
	return plan, nil
}

func inspectImageFile(ctx context.Context, path string, size int64, target emulators.Platform) (Plan, error) {
	plan := Plan{Type: TypeImage, SourcePath: path, ContentRoot: path, Platform: target.ID, Candidates: []Candidate{}}
	if IsArchive(path) {
		plan.ContentRoot = filepath.Dir(path)
		if err := fillImageArchive(&plan, path); err != nil {
			return Plan{}, err
		}
		return plan, nil
	}
	if target.ManualOnly(path) {
		return Plan{}, fmt.Errorf("%w: %s", emulators.ErrImageManualOnly, filepath.Base(path))
	}
	ext := strings.ToLower(filepath.Ext(path))
	// A file that starts an unpacked game means nothing away from its folder.
	if !target.AcceptsImage(path) || target.IsGameFile(path) || ext == ".cue" || ext == ".m3u" {
		return Plan{}, fmt.Errorf("%w: %s", errImageNotFound, filepath.Base(path))
	}
	if target.HasGameFiles() {
		info, err := target.ReadGameInfo(ctx, path)
		if err != nil {
			return Plan{}, err
		}
		if target.IsUpdate(info) {
			return Plan{}, fmt.Errorf("%w: %s", errImageUpdateOnly, filepath.Base(path))
		}
		if target.IsAddon(info) {
			return Plan{}, fmt.Errorf("%w: %s", errImageAddonOnly, filepath.Base(path))
		}
	}
	plan.ImagePath = path
	plan.EstimatedSize = size
	plan.CanAutoInstall = true
	return plan, nil
}

func fillImageArchive(plan *Plan, archive string) error {
	if err := fillArchivePlan(plan, archive); err != nil {
		return err
	}
	plan.Type = TypeImage
	return nil
}

// expectedImageRefusal is what the content of a download or the state of the
// library decides: the player sees it, and diagnostics must not count it as a
// fault of the launcher.
func expectedImageRefusal(err error) bool {
	refusals := []error{
		errImageAmbiguous, errImageNotFound, errImageUnreadable, errImageUpdateOnly, errImageAddonOnly, library.ErrAlreadyAdded,
		emulators.ErrImageManualOnly, emulators.ErrSFOMissing, emulators.ErrSFOInvalid,
		emulators.ErrTitleIDInvalid, emulators.ErrPackageInvalid,
	}
	for _, refusal := range refusals {
		if errors.Is(err, refusal) {
			return true
		}
	}
	return false
}

// checkImageOptions validates what the interface sends before anything looks at
// the download (invariant 32). Image fields without a console mean the caller
// mixed up two flows, and dropping them would install a console game as a PC one.
func checkImageOptions(opts StartOptions) error {
	if opts.Platform == "" {
		if opts.Type == TypeImage || opts.Title != "" || opts.Cover != "" {
			return fmt.Errorf("%w: не указана консоль", errImageOptions)
		}
		return nil
	}
	if _, ok := emulators.Lookup(opts.Platform); !ok {
		return fmt.Errorf("%w: %q", emulators.ErrUnknownPlatform, opts.Platform)
	}
	switch {
	case opts.Type != "" && opts.Type != TypeImage:
		return fmt.Errorf("%w: тип %s", errImageOptions, opts.Type)
	case opts.InstallerPath != "":
		return fmt.Errorf("%w: у образа нет установщика", errImageOptions)
	case opts.SkipRegister:
		return fmt.Errorf("%w: образ без карточки недоступен для запуска", errImageOptions)
	case utf8.RuneCountInString(opts.Title) > maxImageTitle:
		return fmt.Errorf("%w: слишком длинное название", errImageOptions)
	case strings.ContainsFunc(opts.Title, unicode.IsControl):
		return fmt.Errorf("%w: управляющие символы в названии", errImageOptions)
	case opts.Cover != "" && !emulators.ValidCover(opts.Cover):
		return fmt.Errorf("%w: обложка", errImageOptions)
	}
	return nil
}

// checkImageNew refuses a release that already has a card before the copy
// starts: a second folder of the same game would be left with no card at all.
func (s *Service) checkImageNew(origin Installation) error {
	if s.library == nil {
		return errNoLibrary
	}
	for _, game := range s.library.GetGames() {
		if game.Emulation == nil || game.Uninstalled || game.Emulation.Platform != origin.Platform {
			continue
		}
		if game.HoldsRelease(origin.Origin.ReleaseID, origin.Origin.SourceID, origin.DownloadID) {
			return fmt.Errorf("%w: %s", library.ErrAlreadyAdded, game.Title)
		}
	}
	return nil
}

// imageDestination keeps images under Consoles of a games folder: that layout is
// what lets RemoveGame later trust the folder as the launcher's own.
func (s *Service) imageDestination(cfg settings.Settings, requested, platformID, name string) (string, error) {
	if requested != "" {
		dest, err := cleanDestination(requested)
		if err != nil {
			return "", err
		}
		if !imageLayout(dest, platformID) {
			return "", fmt.Errorf("%w: %s", errImageDestination, dest)
		}
		return dest, nil
	}
	root := s.chooseRoot(cfg)
	if root == "" {
		return "", errNoDestination
	}
	return s.proposeDestination(filepath.Join(root, ConsolesFolder, platformID), name)
}

func imageLayout(dest, platformID string) bool {
	platformDir := filepath.Dir(dest)
	consoles := filepath.Dir(platformDir)
	return strings.EqualFold(filepath.Base(platformDir), platformID) && strings.EqualFold(filepath.Base(consoles), ConsolesFolder)
}

// ownedImage is the only way an image card gets to delete files. The flag alone
// is not enough: the card is a record, and deleting a folder because of a wrong
// record cannot be undone, so the shape of what is on it is checked as well.
func ownedImage(game library.Game) bool {
	if game.Emulation == nil || !game.Owned || game.InstallType != string(TypeImage) {
		return false
	}
	if game.InstallDir == "" || !imageLayout(game.InstallDir, game.Emulation.Platform) {
		return false
	}
	target, ok := emulators.Lookup(game.Emulation.Platform)
	// The whole game has to be inside: a card standing on USRDIR would delete
	// half of an unpacked game and leave the rest.
	if !ok || !inside(game.InstallDir, game.Executable) || !inside(game.InstallDir, target.GameDir(game.Executable)) {
		return false
	}
	for _, file := range game.Emulation.Files {
		if !inside(game.InstallDir, file) {
			return false
		}
	}
	return true
}

// checkImageFolder is invariant 16 for an installed image: nothing but the game
// lies in the folder about to go.
func (s *Service) checkImageFolder(game library.Game) error {
	cfg := s.config()
	for _, root := range append(cfg.GamesRoots(), cfg.DownloadsPath) {
		if root != "" && inside(game.InstallDir, root) {
			return fmt.Errorf("%w: %s", errUnsafeRemoval, game.InstallDir)
		}
	}
	for _, other := range s.library.GetGames() {
		if other.ID != game.ID && sharesFootprint(other, game.InstallDir) {
			return fmt.Errorf("%w: %s", errUnsafeRemoval, other.Title)
		}
	}
	return nil
}

func (s *Service) registerImage(ctx context.Context, item Installation, version string) (library.Game, error) {
	if s.library == nil {
		return library.Game{}, errNoLibrary
	}
	if item.Destination == "" {
		return library.Game{}, errEmptyDestination
	}
	return s.library.RegisterEmulated(ctx, library.InstalledImage{
		Title:            item.Name,
		Cover:            item.Cover,
		CatalogGameID:    item.Origin.GameID,
		Platform:         item.Platform,
		Image:            item.Executable,
		InstallDir:       item.Destination,
		Owned:            item.Owned,
		ReleaseID:        item.Origin.ReleaseID,
		SourceID:         item.Origin.SourceID,
		SourceDownloadID: item.DownloadID,
		Version:          version,
	})
}

// copyImageSource is CopyDirVerified that also takes a single file: a download of
// one image is a file beside other downloads, and its folder is not the game.
func copyImageSource(ctx context.Context, src, partial string, report func(Progress)) error {
	info, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("source of the copy: %w", err)
	}
	if info.IsDir() {
		return CopyDirVerified(ctx, src, partial, report)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%w: %s", errNonRegular, src)
	}
	if !destAvailable(partial) {
		return errDestExists
	}
	if err := os.MkdirAll(partial, 0o755); err != nil {
		return err
	}
	target := filepath.Join(partial, filepath.Base(src))
	rep := newReporter(report, info.Size())
	rep.setFile(filepath.Base(src))
	buf := make([]byte, copyBufferSize)
	if err := copyFile(ctx, src, target, info.Mode(), rep, buf); err != nil {
		return err
	}
	want, _, err := hashImageFile(ctx, src, buf)
	if err != nil {
		return err
	}
	got, _, err := hashImageFile(ctx, target, buf)
	if err != nil {
		return err
	}
	if want != got {
		return fmt.Errorf("%w: %s", errCopyVerify, filepath.Base(src))
	}
	return nil
}
