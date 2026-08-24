package download

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

	"github.com/anacrolix/torrent/metainfo"
)

type Layout string

const (
	LayoutDirectFiles      Layout = "direct_files"
	LayoutInstallerPackage Layout = "installer_package"
	LayoutArchivePackage   Layout = "archive_package"
	LayoutUnknown          Layout = "unknown"
)

var errHashBusy = errors.New("этот торрент уже используется другой загрузкой")

type ReuseRequest struct {
	Source   string `json:"source"`
	InfoHash string `json:"infoHash"`
	Path     string `json:"path"`
	Flat     *bool  `json:"flat,omitempty"`
}

type ReuseFile struct {
	Path      string `json:"path"`
	Size      int64  `json:"size"`
	BytesDone int64  `json:"bytesDone"`
	Missing   bool   `json:"missing"`
}

type ReuseReport struct {
	InfoHash     string      `json:"infoHash"`
	Name         string      `json:"name"`
	Layout       Layout      `json:"layout"`
	Flat         bool        `json:"flat"`
	Applicable   bool        `json:"applicable"`
	PresentFiles int         `json:"presentFiles"`
	TotalBytes   int64       `json:"totalBytes"`
	MatchedBytes int64       `json:"matchedBytes"`
	MissingBytes int64       `json:"missingBytes"`
	TotalPieces  int         `json:"totalPieces"`
	OkPieces     int         `json:"okPieces"`
	BadPieces    int         `json:"badPieces"`
	MissingFiles int         `json:"missingFiles"`
	Files        []ReuseFile `json:"files"`
}

type VerifyProgress struct {
	ProcessedBytes int64  `json:"processedBytes"`
	TotalBytes     int64  `json:"totalBytes"`
	CurrentFile    string `json:"currentFile"`
}

var archiveExts = map[string]bool{
	".rar": true, ".7z": true, ".zip": true, ".arc": true, ".cab": true,
	".iso": true, ".bin": true, ".wim": true, ".tar": true, ".gz": true,
}

var installerNames = []string{"setup", "install", "installer", "autorun"}

func splitArchivePart(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	if archiveExts[ext] {
		return true
	}
	if len(ext) != 4 {
		return false
	}
	if ext[1] >= '0' && ext[1] <= '9' && ext[2] >= '0' && ext[2] <= '9' && ext[3] >= '0' && ext[3] <= '9' {
		return true
	}
	return ext[1] == 'r' && ext[2] >= '0' && ext[2] <= '9' && ext[3] >= '0' && ext[3] <= '9'
}

func isInstaller(name string) bool {
	lower := strings.ToLower(name)
	if strings.HasSuffix(lower, ".msi") {
		return true
	}
	if !strings.HasSuffix(lower, ".exe") {
		return false
	}
	base := strings.TrimSuffix(filepath.Base(lower), ".exe")
	for _, marker := range installerNames {
		if strings.Contains(base, marker) {
			return true
		}
	}
	return false
}

func fileName(info *metainfo.Info, fi metainfo.FileInfo) string {
	if path := fi.BestPath(); len(path) > 0 {
		return path[len(path)-1]
	}
	return info.BestName()
}

// ClassifyLayout tells apart torrents that hold playable game files from
// installer or archive packages, where existing data can rarely be reused.
func ClassifyLayout(info *metainfo.Info) Layout {
	files := info.UpvertedFiles()
	if len(files) == 0 {
		return LayoutUnknown
	}
	var total, archived int64
	installer, executables := false, 0
	for _, fi := range files {
		name := fileName(info, fi)
		total += fi.Length
		if splitArchivePart(name) {
			archived += fi.Length
		}
		if isInstaller(name) {
			installer = true
		}
		if strings.HasSuffix(strings.ToLower(name), ".exe") {
			executables++
		}
	}
	if total == 0 {
		return LayoutUnknown
	}
	switch {
	case archived*10 >= total*6:
		return LayoutArchivePackage
	case installer && len(files) <= 8:
		return LayoutInstallerPackage
	case executables > 0 && len(files) >= 5:
		return LayoutDirectFiles
	case installer:
		return LayoutInstallerPackage
	default:
		return LayoutUnknown
	}
}

func relativePaths(info *metainfo.Info, flat bool) []string {
	files := info.UpvertedFiles()
	out := make([]string, len(files))
	for i := range files {
		path := files[i].BestPath()
		if len(path) == 0 {
			out[i] = info.BestName()
			continue
		}
		if flat {
			out[i] = filepath.Join(path...)
			continue
		}
		out[i] = filepath.Join(append([]string{info.BestName()}, path...)...)
	}
	return out
}

func supportsFlat(info *metainfo.Info) bool {
	for _, fi := range info.UpvertedFiles() {
		if len(fi.BestPath()) == 0 {
			return false
		}
	}
	return true
}

type mapping struct {
	flat    bool
	present int
	bytes   int64
}

func inspectMapping(root string, info *metainfo.Info, flat bool) mapping {
	files := info.UpvertedFiles()
	paths := relativePaths(info, flat)
	found := mapping{flat: flat}
	for i := range files {
		stat, err := os.Stat(filepath.Join(root, paths[i]))
		if err != nil || stat.IsDir() {
			continue
		}
		found.present++
		if stat.Size() == files[i].Length {
			found.bytes += files[i].Length
		}
	}
	return found
}

// chooseMapping picks between laying the torrent out directly inside root and
// keeping its own top level folder, based on which files are already there.
// A mapping that finds nothing on disk means the torrent does not describe this
// directory at all, which the caller must tell apart from damaged data.
func chooseMapping(info *metainfo.Info, root string) mapping {
	direct := inspectMapping(root, info, false)
	if !supportsFlat(info) {
		return direct
	}
	flat := inspectMapping(root, info, true)
	if flat.bytes > direct.bytes || (flat.bytes == direct.bytes && flat.present > direct.present) {
		return flat
	}
	return direct
}

func (m *Manager) engine() (*client, context.Context, error) {
	m.mu.Lock()
	cl, ctx, closing := m.client, m.ctx, m.closing
	m.mu.Unlock()
	if cl == nil || closing {
		return nil, nil, errNoClient
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return cl, ctx, nil
}

func (m *Manager) metainfoFor(ctx context.Context, cl *client, source, infoHash string) (*metainfo.MetaInfo, error) {
	if infoHash != "" {
		mi, err := m.store.loadMetainfo(infoHash)
		switch {
		case err == nil:
			return mi, nil
		case !errors.Is(err, fs.ErrNotExist):
			return nil, fmt.Errorf("кеш torrent-файла %s: %w", infoHash, err)
		}
	}
	source = strings.TrimSpace(source)
	if source == "" {
		return nil, errNoMetadata
	}
	if !strings.HasPrefix(source, "magnet:") {
		mi, err := metainfo.LoadFromFile(source)
		if err != nil {
			return nil, errors.New("не удалось прочитать torrent-файл")
		}
		return mi, nil
	}

	spec, err := magnetSpec(source)
	if err != nil {
		return nil, errors.New("некорректная magnet-ссылка")
	}
	lt, err := cl.add(spec, cl.metaDir, storageOpts{})
	if err != nil {
		return nil, err
	}
	select {
	case <-lt.t.GotInfo():
	case <-ctx.Done():
		lt.drop()
		return nil, ctx.Err()
	case <-time.After(metadataTimeout):
		lt.drop()
		return nil, errNoMetadata
	}
	mi := lt.t.Metainfo()
	lt.drop()
	hash := spec.InfoHash.HexString()
	if err := m.store.saveMetainfo(hash, &mi); err != nil {
		slog.Warn("save metainfo", "operation", "inspect_reuse", "error", err)
	}
	return &mi, nil
}

// hashBusy reports whether the torrent is already live in the client. A
// completed download that no longer seeds holds no engine, so its files can be
// rechecked against another directory.
func (m *Manager) hashBusy(infoHash string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, waiting := m.pending[infoHash]; waiting {
		return true
	}
	for _, d := range m.items {
		if !strings.EqualFold(d.InfoHash, infoHash) {
			continue
		}
		if m.engines[d.ID] != nil || m.jobs[d.ID] != nil {
			return true
		}
	}
	return false
}

// hashInUse reports whether another download already holds a live torrent with
// the same infohash. Two of them would share one engine and write to the wrong
// directory.
func (m *Manager) hashInUse(infoHash, excludeID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, d := range m.items {
		if d.ID == excludeID || !strings.EqualFold(d.InfoHash, infoHash) {
			continue
		}
		if m.engines[d.ID] != nil {
			return true
		}
	}
	return false
}

// InspectReuse hashes the files already present at a path against a torrent and
// reports how much of it can be reused.
//
//wails:ignore
func (m *Manager) InspectReuse(ctx context.Context, req ReuseRequest, onProgress func(VerifyProgress)) (ReuseReport, error) {
	cl, base, err := m.engine()
	if err != nil {
		return ReuseReport{}, err
	}
	if ctx == nil {
		ctx = base
	}
	root := strings.TrimSpace(req.Path)
	if root == "" {
		return ReuseReport{}, errors.New("папка установки не указана")
	}
	if stat, err := os.Stat(root); err != nil || !stat.IsDir() {
		return ReuseReport{}, errors.New("папка установки недоступна")
	}

	mi, err := m.metainfoFor(ctx, cl, req.Source, req.InfoHash)
	if err != nil {
		return ReuseReport{}, err
	}
	info, err := mi.UnmarshalInfo()
	if err != nil {
		return ReuseReport{}, errNoMetadata
	}
	if err := validateInfo(&info); err != nil {
		return ReuseReport{}, err
	}
	infoHash := mi.HashInfoBytes().HexString()
	if m.hashBusy(infoHash) {
		return ReuseReport{}, errHashBusy
	}

	chosen := chooseMapping(&info, root)
	if req.Flat != nil {
		chosen = inspectMapping(root, &info, *req.Flat)
	}
	total := info.TotalLength()
	files := info.UpvertedFiles()
	paths := relativePaths(&info, chosen.flat)
	report := ReuseReport{
		InfoHash:     infoHash,
		Name:         info.BestName(),
		Layout:       ClassifyLayout(&info),
		Flat:         chosen.flat,
		Applicable:   chosen.present > 0,
		PresentFiles: chosen.present,
		TotalBytes:   total,
	}
	if !report.Applicable {
		report.MissingBytes = total
		report.MissingFiles = len(files)
		report.Files = make([]ReuseFile, 0, len(files))
		for i := range files {
			report.Files = append(report.Files, ReuseFile{Path: paths[i], Size: files[i].Length, Missing: true})
		}
		slog.Info("torrent does not describe path", "operation", "inspect_reuse",
			"layout", report.Layout, "files", len(files))
		return report, nil
	}

	lt, err := cl.addMetainfo(mi, root, storageOpts{flat: chosen.flat, inPlace: true})
	if err != nil {
		return ReuseReport{}, err
	}
	defer lt.drop()

	owners := pieceFileIndex(&info)

	var processed int64
	if err := lt.verifyEach(ctx, func(index int, length int64) {
		processed += length
		if onProgress == nil {
			return
		}
		current := ""
		if index < len(owners) && owners[index] < len(paths) {
			current = paths[owners[index]]
		}
		onProgress(VerifyProgress{ProcessedBytes: processed, TotalBytes: total, CurrentFile: current})
	}); err != nil {
		return ReuseReport{}, err
	}

	ok, pieces := lt.completePieces()
	report.TotalPieces = pieces
	report.OkPieces = ok
	report.BadPieces = pieces - ok
	done := lt.fileBytes()
	report.Files = make([]ReuseFile, 0, len(files))
	for i := range files {
		entry := ReuseFile{Path: paths[i], Size: files[i].Length}
		if i < len(done) {
			entry.BytesDone = done[i]
		}
		if stat, err := os.Stat(filepath.Join(root, paths[i])); err != nil || stat.IsDir() {
			entry.Missing = true
			report.MissingFiles++
		}
		report.MatchedBytes += entry.BytesDone
		report.Files = append(report.Files, entry)
	}
	report.MissingBytes = total - report.MatchedBytes
	slog.Info("torrent reuse inspected", "operation", "inspect_reuse",
		"layout", report.Layout, "flat", report.Flat, "present", report.PresentFiles,
		"matched", report.MatchedBytes, "missing", report.MissingBytes)
	return report, nil
}

func pieceFileIndex(info *metainfo.Info) []int {
	if info.PieceLength <= 0 {
		return nil
	}
	files := info.UpvertedFiles()
	count := int((info.TotalLength() + info.PieceLength - 1) / info.PieceLength)
	ends := make([]int64, len(files))
	var running int64
	for i := range files {
		running += files[i].Length
		ends[i] = running
	}
	out := make([]int, count)
	var offset int64
	file := 0
	for piece := 0; piece < count; piece++ {
		for file+1 < len(files) && offset >= ends[file] {
			file++
		}
		out[piece] = file
		offset += info.PieceLength
	}
	return out
}
