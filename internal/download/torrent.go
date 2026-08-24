package download

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"math"
	"net"
	"path/filepath"
	"strings"

	"typhon/internal/settings"

	g "github.com/anacrolix/generics"
	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/storage"
	"golang.org/x/time/rate"
)

const (
	listenPort      = 42815
	minLimiterBurst = 256 * 1024
	maxTorrentConns = 60
)

var errBadPaths = errors.New("недопустимые пути файлов в торренте")

type engineStats struct {
	downloaded int64
	uploaded   int64
	seeders    int
	peers      int
}

// storageOpts maps torrent files onto an existing directory. flat drops the
// torrent name folder, inPlace disables .part files so that already installed
// files are read and repaired where they are.
type storageOpts struct {
	flat    bool
	inPlace bool
}

type engineTorrent interface {
	setPriorities(selected []bool)
	allowDownload()
	disallowDownload()
	allowUpload()
	disallowUpload()
	fileBytes() []int64
	filePaths(destination string) []string
	stats() engineStats
	verify(ctx context.Context) error
	drop()
}

type client struct {
	cl      *torrent.Client
	down    *rate.Limiter
	up      *rate.Limiter
	metaDir string
}

func newClient(cfg settings.Settings, metaDir string) (*client, error) {
	tc := clientConfig(cfg, metaDir, listenPort)
	cl, err := torrent.NewClient(tc)
	if err != nil && isListenError(err) {
		slog.Warn("torrent port unavailable, retrying on a random port", "port", listenPort, "error", err)
		closeDefaultStorage(tc)
		tc = clientConfig(cfg, metaDir, 0)
		cl, err = torrent.NewClient(tc)
	}
	if err != nil {
		closeDefaultStorage(tc)
		return nil, err
	}
	slog.Info("torrent client started", "port", cl.LocalPort())
	return &client{cl: cl, down: tc.DownloadRateLimiter, up: tc.UploadRateLimiter, metaDir: metaDir}, nil
}

func clientConfig(cfg settings.Settings, dataDir string, port int) *torrent.ClientConfig {
	tc := torrent.NewDefaultClientConfig()
	tc.DataDir = dataDir
	tc.ListenPort = port
	tc.Seed = true
	tc.Slogger = slog.Default()
	tc.DownloadRateLimiter = newLimiter(cfg.DownloadRateLimit)
	tc.UploadRateLimiter = newLimiter(cfg.UploadRateLimit)
	tc.DefaultStorage = storage.NewFileWithCompletion(dataDir, storage.NewMapPieceCompletion())
	return tc
}

func closeDefaultStorage(tc *torrent.ClientConfig) {
	if closer, ok := tc.DefaultStorage.(io.Closer); ok {
		if err := closer.Close(); err != nil {
			slog.Warn("close default storage", "error", err)
		}
	}
}

func isListenError(err error) bool {
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "bind") ||
		strings.Contains(text, "listen") ||
		strings.Contains(text, "address already in use")
}

func (c *client) applyLimits(down, up int64) {
	applyLimit(c.down, down)
	applyLimit(c.up, up)
}

func (c *client) close() {
	for _, err := range c.cl.Close() {
		slog.Error("close torrent client", "error", err)
	}
}

func (c *client) addMetainfo(mi *metainfo.MetaInfo, destination string, opts storageOpts) (*liveTorrent, error) {
	spec, err := torrent.TorrentSpecFromMetaInfoErr(mi)
	if err != nil {
		return nil, err
	}
	return c.add(spec, destination, opts)
}

func (c *client) addMagnet(uri, destination string, opts storageOpts) (*liveTorrent, error) {
	spec, err := magnetSpec(uri)
	if err != nil {
		return nil, err
	}
	return c.add(spec, destination, opts)
}

func newStorage(destination string, opts storageOpts) storage.ClientImplCloser {
	if !opts.flat && !opts.inPlace {
		return storage.NewFileWithCompletion(destination, storage.NewMapPieceCompletion())
	}
	clientOpts := storage.NewFileClientOpts{
		ClientBaseDir:   destination,
		PieceCompletion: storage.NewMapPieceCompletion(),
	}
	if opts.flat {
		clientOpts.FilePathMaker = func(o storage.FilePathMakerOpts) string {
			return filepath.Join(o.File.BestPath()...)
		}
	}
	if opts.inPlace {
		clientOpts.UsePartFiles = g.Some(false)
	}
	return storage.NewFileOpts(clientOpts)
}

func (c *client) add(spec *torrent.TorrentSpec, destination string, opts storageOpts) (*liveTorrent, error) {
	if len(spec.PieceLayers) == 0 {
		spec.PieceLayers = nil
	}
	st := newStorage(destination, opts)
	spec.Storage = st

	t, _, err := c.cl.AddTorrentSpec(spec)
	if err != nil {
		st.Close()
		return nil, err
	}
	// AddTorrentOpts.DisallowData* are declared but never read by the engine,
	// so a torrent starts fully enabled and has to be gated after it is added.
	t.DisallowDataDownload()
	t.DisallowDataUpload()
	t.SetMaxEstablishedConns(maxTorrentConns)
	return &liveTorrent{t: t, storage: st, flat: opts.flat}, nil
}

func magnetSpec(uri string) (*torrent.TorrentSpec, error) {
	spec, err := torrent.TorrentSpecFromMagnetUri(uri)
	if err != nil {
		return nil, err
	}
	if spec.InfoHash.IsZero() {
		return nil, errors.New("magnet uri without v1 infohash")
	}
	return spec, nil
}

func newLimiter(bytesPerSecond int64) *rate.Limiter {
	if bytesPerSecond <= 0 {
		return rate.NewLimiter(rate.Inf, math.MaxInt)
	}
	return rate.NewLimiter(rate.Limit(bytesPerSecond), limiterBurst(bytesPerSecond))
}

func limiterBurst(bytesPerSecond int64) int {
	if bytesPerSecond < minLimiterBurst {
		return minLimiterBurst
	}
	return int(bytesPerSecond)
}

func applyLimit(l *rate.Limiter, bytesPerSecond int64) {
	if l == nil {
		return
	}
	if bytesPerSecond <= 0 {
		l.SetLimit(rate.Inf)
		l.SetBurst(math.MaxInt)
		return
	}
	l.SetLimit(rate.Limit(bytesPerSecond))
	l.SetBurst(limiterBurst(bytesPerSecond))
}

type liveTorrent struct {
	t       *torrent.Torrent
	storage io.Closer
	flat    bool
}

func (l *liveTorrent) setPriorities(selected []bool) {
	for i, f := range l.t.Files() {
		if i < len(selected) && selected[i] {
			f.SetPriority(torrent.PiecePriorityNormal)
		} else {
			f.SetPriority(torrent.PiecePriorityNone)
		}
	}
}

func (l *liveTorrent) allowDownload()    { l.t.AllowDataDownload() }
func (l *liveTorrent) disallowDownload() { l.t.DisallowDataDownload() }
func (l *liveTorrent) allowUpload()      { l.t.AllowDataUpload() }
func (l *liveTorrent) disallowUpload()   { l.t.DisallowDataUpload() }

func (l *liveTorrent) fileBytes() []int64 {
	files := l.t.Files()
	done := make([]int64, len(files))
	for i, f := range files {
		done[i] = f.BytesCompleted()
	}
	return done
}

func (l *liveTorrent) filePaths(destination string) []string {
	rel := relativePaths(l.t.Info(), l.flat)
	out := make([]string, len(rel))
	for i, r := range rel {
		out[i] = filepath.Join(destination, r)
	}
	return out
}

func (l *liveTorrent) stats() engineStats {
	st := l.t.Stats()
	return engineStats{
		downloaded: st.BytesReadUsefulData.Int64(),
		uploaded:   st.BytesWrittenData.Int64(),
		seeders:    st.ConnectedSeeders,
		peers:      st.ActivePeers,
	}
}

func (l *liveTorrent) verify(ctx context.Context) error {
	return l.t.VerifyDataContext(ctx)
}

func (l *liveTorrent) verifyEach(ctx context.Context, done func(index int, length int64)) error {
	for i := 0; i < l.t.NumPieces(); i++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		piece := l.t.Piece(i)
		if err := piece.VerifyDataContext(ctx); err != nil {
			return err
		}
		done(i, piece.Info().Length())
	}
	return nil
}

func (l *liveTorrent) completePieces() (complete, total int) {
	total = l.t.NumPieces()
	for i := 0; i < total; i++ {
		if l.t.Piece(i).State().Complete {
			complete++
		}
	}
	return complete, total
}

func (l *liveTorrent) drop() {
	l.t.Drop()
	if l.storage != nil {
		if err := l.storage.Close(); err != nil {
			slog.Warn("close torrent storage", "error", err)
		}
	}
}

func validateInfo(info *metainfo.Info) error {
	if info == nil {
		return errBadPaths
	}
	if !isSafeTorrentPath(info.BestName()) {
		return errBadPaths
	}
	for _, fi := range info.UpvertedFiles() {
		components := fi.BestPath()
		if len(components) == 0 {
			if info.IsDir() {
				return errBadPaths
			}
			continue
		}
		if !isSafeTorrentPath(strings.Join(components, "/")) {
			return errBadPaths
		}
	}
	return nil
}

func isSafeTorrentPath(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	for _, component := range strings.Split(path, "/") {
		if component == "" || component == "." || component == ".." {
			return false
		}
		if strings.ContainsAny(component, `\:`) {
			return false
		}
		if !filepath.IsLocal(component) {
			return false
		}
	}
	return true
}
