package download

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"strings"

	"typhon/internal/settings"

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

type engineTorrent interface {
	setPriorities(selected []bool)
	allowDownload()
	disallowDownload()
	allowUpload()
	disallowUpload()
	fileBytes() []int64
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
	tc := torrent.NewDefaultClientConfig()
	tc.DataDir = cfg.DownloadsPath
	tc.ListenPort = listenPort
	tc.Seed = true
	tc.Slogger = slog.Default()
	tc.DownloadRateLimiter = newLimiter(cfg.DownloadRateLimit)
	tc.UploadRateLimiter = newLimiter(cfg.UploadRateLimit)
	tc.DefaultStorage = storage.NewFileWithCompletion(cfg.DownloadsPath, storage.NewMapPieceCompletion())

	cl, err := torrent.NewClient(tc)
	if err != nil {
		return nil, err
	}
	return &client{cl: cl, down: tc.DownloadRateLimiter, up: tc.UploadRateLimiter, metaDir: metaDir}, nil
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

func (c *client) addMetainfo(mi *metainfo.MetaInfo, destination string) (*liveTorrent, error) {
	spec, err := torrent.TorrentSpecFromMetaInfoErr(mi)
	if err != nil {
		return nil, err
	}
	return c.add(spec, destination)
}

func (c *client) addMagnet(uri, destination string) (*liveTorrent, error) {
	spec, err := magnetSpec(uri)
	if err != nil {
		return nil, err
	}
	return c.add(spec, destination)
}

func (c *client) add(spec *torrent.TorrentSpec, destination string) (*liveTorrent, error) {
	st := storage.NewFileWithCompletion(destination, storage.NewMapPieceCompletion())
	spec.Storage = st
	spec.DisallowDataDownload = true
	spec.DisallowDataUpload = true

	t, _, err := c.cl.AddTorrentSpec(spec)
	if err != nil {
		st.Close()
		return nil, err
	}
	t.SetMaxEstablishedConns(maxTorrentConns)
	return &liveTorrent{t: t, storage: st}, nil
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
		return rate.NewLimiter(rate.Inf, 0)
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
		l.SetBurst(0)
		return
	}
	l.SetLimit(rate.Limit(bytesPerSecond))
	l.SetBurst(limiterBurst(bytesPerSecond))
}

type liveTorrent struct {
	t       *torrent.Torrent
	storage io.Closer
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
