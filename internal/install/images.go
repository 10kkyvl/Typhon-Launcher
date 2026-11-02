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
	errImageAmbiguous   = uierr.New("install.image_ambiguous", "в загрузке лежат образы разных игр, выбрать один автоматически нельзя")
	errImageOptions     = uierr.New("install.image_options_invalid", "неверные данные для установки образа игры")
	errImageDestination = uierr.New("install.image_destination", "образ игры ставится только в папку Consoles каталога игр")
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
	found, err := imageFiles(ctx, dir, target)
	if err != nil {
		return "", err
	}
	usable, err := usableImages(ctx, found)
	if err != nil {
		return "", err
	}
	return pickImage(usable)
}

func imageFiles(ctx context.Context, dir string, target emulators.Platform) ([]imageFile, error) {
	var found []imageFile
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
			if hidden || strings.Count(rel, string(filepath.Separator)) >= maxImageDepth {
				return fs.SkipDir
			}
			return nil
		}
		if hidden || !d.Type().IsRegular() || !target.AcceptsImage(path) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("stat %s: %w", path, err)
		}
		if info.Size() == 0 {
			return nil
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
		return nil, err
	}
	return found, nil
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
		return inspectImageFile(path, info.Size(), target)
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
	if !errors.Is(err, errImageNotFound) {
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

func inspectImageFile(path string, size int64, target emulators.Platform) (Plan, error) {
	plan := Plan{Type: TypeImage, SourcePath: path, ContentRoot: path, Platform: target.ID, Candidates: []Candidate{}}
	if IsArchive(path) {
		plan.ContentRoot = filepath.Dir(path)
		if err := fillImageArchive(&plan, path); err != nil {
			return Plan{}, err
		}
		return plan, nil
	}
	ext := strings.ToLower(filepath.Ext(path))
	if !target.AcceptsImage(path) || ext == ".cue" || ext == ".m3u" {
		return Plan{}, fmt.Errorf("%w: %s", errImageNotFound, filepath.Base(path))
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
	if !inside(game.InstallDir, game.Executable) {
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
	want, _, err := hashdir.HashFile(ctx, src, buf)
	if err != nil {
		return err
	}
	got, _, err := hashdir.HashFile(ctx, target, buf)
	if err != nil {
		return err
	}
	if want != got {
		return fmt.Errorf("%w: %s", errCopyVerify, filepath.Base(src))
	}
	return nil
}
