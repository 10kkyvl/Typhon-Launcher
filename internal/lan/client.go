package lan

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"math"
	"path/filepath"
	"sync"
	"time"

	// classicio must be initialized before any anacrolix storage read: its
	// mmap-based file IO corrupts piece verification and blocks incoming
	// peer connections on Windows (see internal/download/manager.go for the
	// same fix on the download client's own, separate torrent.Client).
	_ "typhon/internal/download/classicio"
	"typhon/internal/download/listenport"
	"typhon/internal/download/torrentpath"
	"typhon/internal/settings"

	g "github.com/anacrolix/generics"
	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	tstorage "github.com/anacrolix/torrent/storage"
	"golang.org/x/time/rate"
)

// listenPort is the LAN client's own port, distinct from the download
// client's (internal/download/torrent.go:listenPort = 42815) and from the
// multicast announce port (announce.go:multicastPort = 42816).
const (
	listenPort      = 42817
	minLimiterBurst = 256 * 1024
	maxTorrentConns = 32

	// peerWriterWake bounds how long a peer writer can sleep. Released anacrolix
	// v1.61.0 lost a wake that landed between filling the buffer and waiting, so
	// the writer slept until its keep-alive timer with a request or a chunk unsent
	// and the transfer froze for exactly the default minute. The engine is now
	// built from third_party/anacrolix-torrent with the upstream fix; the short
	// interval stays as a safety net because on a LAN a keep-alive costs nothing.
	peerWriterWake = 2 * time.Second
)

// newClientConfig builds the ClientConfig for the LAN sharing client. Every
// field here that keeps a torrent off the public swarm is load-bearing:
// anacrolix/torrent v1.61.0 does not read info.Private from a metainfo
// (only cmd/torrent/create.go in that module ever sets it), so the only way
// to keep a game's infohash out of the public DHT is to never wire up DHT,
// trackers or PEX on this client in the first place.
func newClientConfig(cfg settings.Settings, dataDir string, port int) *torrent.ClientConfig {
	tc := torrent.NewDefaultClientConfig()
	tc.DataDir = dataDir
	tc.ListenPort = port
	tc.Seed = true
	tc.NoDHT = true
	tc.DisableTrackers = true
	tc.DisablePEX = true
	tc.NoDefaultPortForwarding = true
	tc.KeepAliveTimeout = peerWriterWake
	tc.Slogger = slog.Default()
	tc.DownloadRateLimiter = newLimiter(cfg.DownloadRateLimit)
	tc.UploadRateLimiter = newLimiter(cfg.UploadRateLimit)
	tc.DefaultStorage = tstorage.NewFileWithCompletion(dataDir, tstorage.NewMapPieceCompletion())
	return tc
}

type lanClient struct {
	cl      *torrent.Client
	down    *rate.Limiter
	up      *rate.Limiter
	dataDir string

	mu       sync.Mutex
	refusals map[string]error // receiving dest → why its metainfo was refused
}

var openTorrentClient = torrent.NewClient

var receiveStorage = func(dest string) tstorage.ClientImplCloser { return flatStorage(dest, false) }

// guardedStorage validates the metainfo a peer sent before the file storage
// opens it. The engine's own storage refuses paths that escape dest, but it
// does so by failing setInfo silently: GotInfo never fires and the transfer
// hangs until cancelled. Recording the refusal lets watchTransfer fail the
// transfer with a user-visible code within one sample interval, and the
// stricter check also rejects Windows-hostile names the engine would accept.
type guardedStorage struct {
	tstorage.ClientImplCloser
	refuse func(error)
}

func (g guardedStorage) OpenTorrent(ctx context.Context, info *metainfo.Info, hash metainfo.Hash) (tstorage.TorrentImpl, error) {
	if err := torrentpath.ValidateInfo(info); err != nil {
		g.refuse(errUnsafeTorrentPaths)
		return tstorage.TorrentImpl{}, errUnsafeTorrentPaths
	}
	return g.ClientImplCloser.OpenTorrent(ctx, info, hash)
}

func (c *lanClient) recordRefusal(dest string, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.refusals == nil {
		c.refusals = map[string]error{}
	}
	c.refusals[dest] = err
}

// refusal reports why the metainfo for a receiving destination was rejected,
// or nil while nothing was refused.
func (c *lanClient) refusal(dest string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.refusals[dest]
}

func (c *lanClient) forgetRefusal(dest string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.refusals, dest)
}

func newLANClient(cfg settings.Settings, dataDir string, preferredPort int) (*lanClient, error) {
	port := preferredPort
	for attempt := 0; ; attempt++ {
		tc := newClientConfig(cfg, dataDir, port)
		cl, err := openTorrentClient(tc)
		if err == nil {
			slog.Info("lan torrent client started", "port", cl.LocalPort())
			return &lanClient{cl: cl, down: tc.DownloadRateLimiter, up: tc.UploadRateLimiter, dataDir: dataDir}, nil
		}
		closeDefaultStorage(tc)
		if !listenport.IsListenError(err) || attempt == listenport.Attempts {
			return nil, fmt.Errorf("start lan torrent client: %w", err)
		}
		// Port 0 hands out the next number in a row on Windows, which stays
		// inside a UDP range Hyper-V or Docker excluded, so retries pick their own.
		slog.Warn("lan torrent port unavailable, retrying on a random port", "port", port, "error", err)
		next, pickErr := listenport.Random()
		if pickErr != nil {
			return nil, pickErr
		}
		port = next
	}
}

func (c *lanClient) applyLimits(down, up int64) {
	applyLimit(c.down, down)
	applyLimit(c.up, up)
}

func (c *lanClient) close() {
	for _, err := range c.cl.Close() {
		slog.Error("close lan torrent client", "error", err)
	}
}

func (c *lanClient) localPort() int {
	return c.cl.LocalPort()
}

// clearSpec strips everything on a TorrentSpec that could lead a peer
// discovery attempt outside this LAN client's own announce protocol: no
// trackers, webseeds, DHT bootstrap nodes or magnet "xs"/"as" sources.
func clearSpec(spec *torrent.TorrentSpec) {
	spec.Trackers = nil
	spec.Webseeds = nil
	spec.DhtNodes = nil
	spec.Sources = nil
	spec.PeerAddrs = nil
}

// addForSeeding adds a torrent whose data already fully exists at root (an
// installed game), reading it in place rather than staging a copy.
func (c *lanClient) addForSeeding(mi *metainfo.MetaInfo, root string) (*torrent.Torrent, error) {
	spec, err := torrent.TorrentSpecFromMetaInfoErr(mi)
	if err != nil {
		return nil, fmt.Errorf("build spec: %w", err)
	}
	clearSpec(spec)
	st := flatStorage(root, true)
	spec.Storage = st
	// DisableInitialPieceCheck/IgnoreUnverifiedPieceCompletion look like a
	// natural optimization here (we just hashed these exact bytes in
	// BuildInfo), but verified empirically: with the initial piece check
	// skipped, the torrent never becomes ready to serve ut_metadata to a
	// connecting peer, so a receiver hangs on GotInfo forever. The engine
	// re-reads the install once at add time; that cost is accepted.
	t, isNew, err := c.cl.AddTorrentSpec(spec)
	if err == nil && !isNew {
		err = errAlreadyHave
	}
	if err != nil {
		if cerr := st.Close(); cerr != nil {
			slog.Warn("close lan seed storage", "error", cerr)
		}
		return nil, err
	}
	t.DisallowDataDownload()
	t.AllowDataUpload()
	t.SetMaxEstablishedConns(maxTorrentConns)
	return t, nil
}

// addForReceiving adds a torrent known only by infohash; its metainfo
// arrives from the peer over BEP 9 (ut_metadata) once AddPeers connects us.
func (c *lanClient) addForReceiving(hash metainfo.Hash, displayName, dest string) (*torrent.Torrent, error) {
	st := guardedStorage{
		ClientImplCloser: receiveStorage(dest),
		refuse:           func(err error) { c.recordRefusal(dest, err) },
	}
	spec := &torrent.TorrentSpec{
		AddTorrentOpts: torrent.AddTorrentOpts{
			InfoHash: hash,
			Storage:  st,
		},
		DisplayName: displayName,
	}
	t, isNew, err := c.cl.AddTorrentSpec(spec)
	if err == nil && !isNew {
		err = errAlreadyHave
	}
	if err != nil {
		if cerr := st.Close(); cerr != nil {
			slog.Warn("close lan receive storage", "error", cerr)
		}
		return nil, err
	}
	t.AllowDataDownload()
	t.DisallowDataUpload()
	t.SetMaxEstablishedConns(maxTorrentConns)
	return t, nil
}

// flatStorage stores torrent files directly under dir, ignoring whatever
// info.Name a peer's metainfo claims: the destination folder name is chosen
// by us (from the validated announce, not from remote-controlled info),
// never from data a peer sent (invariant 32).
func flatStorage(dir string, inPlace bool) tstorage.ClientImplCloser {
	opts := tstorage.NewFileClientOpts{
		ClientBaseDir:   dir,
		PieceCompletion: tstorage.NewMapPieceCompletion(),
		FilePathMaker: func(o tstorage.FilePathMakerOpts) string {
			return filepath.Join(o.File.BestPath()...)
		},
	}
	if inPlace {
		opts.UsePartFiles = g.Some(false)
	}
	return tstorage.NewFileOpts(opts)
}

func closeDefaultStorage(tc *torrent.ClientConfig) {
	if closer, ok := tc.DefaultStorage.(io.Closer); ok {
		if err := closer.Close(); err != nil {
			slog.Warn("close lan default storage", "error", err)
		}
	}
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
