package download

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net"
	"path/filepath"
	"strings"
	"sync/atomic"

	"typhon/internal/settings"
	"typhon/internal/uierr"

	g "github.com/anacrolix/generics"
	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/storage"
	"golang.org/x/time/rate"
)

const (
	listenPort         = 42815
	randomPortAttempts = 8
	minLimiterBurst    = 256 * 1024
	maxTorrentConns    = 60
)

var openTorrentClient = torrent.NewClient

var errBadPaths = uierr.New("download.bad_paths", "недопустимые пути файлов в торренте")

// errTorrentAlreadyAdded means Client.AddTorrentSpec merged the spec into an
// existing *torrent.Torrent instead of creating a new one (its "new" return
// value was false). MergeSpec documents that it ignores the Storage the spec
// carried, so silently continuing here would hand back a *liveTorrent whose
// storage field points at destination while the actual data goes wherever
// the existing torrent was first added to. This is deliberately not
// errHashBusy: that sentinel means the manager's own bookkeeping (engines,
// jobs, pending, reservations) saw the hash as taken before touching the
// client; reaching this instead means the client itself already tracks the
// hash despite the manager believing it did not, which the reservations in
// reuse.go are meant to prevent — this is the last line of defence, not the
// expected path.
var errTorrentAlreadyAdded = errors.New("torrent already tracked by the client for this infohash")

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
	filesHashed() []bool
	filePaths(destination string) []string
	stats() engineStats
	verify(ctx context.Context) error
	drop()
}

type client struct {
	cl         *torrent.Client
	down       *rate.Limiter
	up         *rate.Limiter
	metaDir    string
	completion storage.PieceCompletion

	gen              uint64
	httpTrackersOnly bool
	filterTrackers   func([][]string) ([][]string, []lostTracker)
	retryTrackers    func(*torrent.Torrent, []lostTracker)
	later            *retrier
	stopped          atomic.Bool
}

func newClient(ctx context.Context, cfg settings.Settings, metaDir string, completion storage.PieceCompletion, plan netPlan) (*client, error) {
	wrapped := nonClosingCompletion{completion}
	port := listenPort
	for attempt := 0; ; attempt++ {
		tc, attach, err := networkedConfig(ctx, cfg, metaDir, port, wrapped, plan)
		if err != nil {
			closeDefaultStorage(tc)
			return nil, err
		}
		c := &client{
			down:             tc.DownloadRateLimiter,
			up:               tc.UploadRateLimiter,
			metaDir:          metaDir,
			completion:       wrapped,
			httpTrackersOnly: plan.mode == settings.NetworkProxy,
			filterTrackers:   attach.trackers,
			retryTrackers:    attach.retry,
			later:            attach.later,
		}
		guardNetwork(tc, c.halted)
		cl, err := openTorrentClient(tc)
		if err == nil {
			attach.attach(cl)
			c.cl = cl
			slog.Info("torrent client started", "port", cl.LocalPort(), "network", plan.mode)
			return c, nil
		}
		closeDefaultStorage(tc)
		attach.later.stop()
		if !isListenError(err) || attempt == randomPortAttempts {
			return nil, err
		}
		// The client takes one port number for TCP and UDP over both IPv4 and
		// IPv6. A random port is only free for the first of them, so the same
		// number can still be held in UDP by another process (Windows services
		// keep ephemeral UDP ports), and the next random port usually is not.
		slog.Warn("torrent port unavailable, retrying on a random port", "port", port, "error", err)
		port = 0
	}
}

func networkedConfig(ctx context.Context, cfg settings.Settings, dataDir string, port int, completion storage.PieceCompletion, plan netPlan) (*torrent.ClientConfig, netAttach, error) {
	tc := clientConfig(cfg, dataDir, port, completion)
	attach, err := applyNetwork(ctx, tc, plan)
	return tc, attach, err
}

func clientConfig(cfg settings.Settings, dataDir string, port int, completion storage.PieceCompletion) *torrent.ClientConfig {
	tc := torrent.NewDefaultClientConfig()
	tc.DataDir = dataDir
	tc.ListenPort = port
	tc.Seed = true
	tc.Slogger = slog.Default()
	tc.DownloadRateLimiter = newLimiter(cfg.DownloadRateLimit)
	tc.UploadRateLimiter = newLimiter(cfg.UploadRateLimit)
	tc.DefaultStorage = storage.NewFileWithCompletion(dataDir, completion)
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
	c.later.stop()
	for _, err := range c.cl.Close() {
		slog.Error("close torrent client", "error", err)
	}
	c.later.wait()
}

func (c *client) halted() bool { return c.stopped.Load() }

// halt cuts the client off from the network ahead of its close. Closing the
// client is the last step of a teardown, after jobs that may take long to
// notice they were cancelled, and a client closed before they end would hang
// them (a verify that starts on a closed torrent returns with the client lock
// held). So the client stays open and stops carrying data instead: no torrent
// moves a byte either way, every peer connection is dropped and none is
// accepted or dialled again.
func (c *client) halt() {
	// Set first: a torrent added while the sweep runs is halted by add.
	c.stopped.Store(true)
	c.later.stop()
	for _, t := range c.cl.Torrents() {
		haltTorrent(t)
	}
}

func haltTorrent(t *torrent.Torrent) {
	t.DisallowDataDownload()
	t.DisallowDataUpload()
	// Dropping the connections is what closes the sockets; with the gates alone
	// they stay open and the swarm keeps seeing the address.
	t.SetMaxEstablishedConns(0)
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

func newStorage(destination string, opts storageOpts, completion storage.PieceCompletion) storage.ClientImplCloser {
	if !opts.flat && !opts.inPlace {
		return storage.NewFileWithCompletion(destination, completion)
	}
	clientOpts := storage.NewFileClientOpts{
		ClientBaseDir:   destination,
		PieceCompletion: completion,
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
	if c.halted() {
		return nil, errNetworkDown
	}
	if len(spec.PieceLayers) == 0 {
		spec.PieceLayers = nil
	}
	st := newStorage(destination, opts, c.completion)
	spec.Storage = st
	var announce [][]string
	var lost []lostTracker
	if c.filterTrackers != nil {
		announce = cloneTiers(spec.Trackers)
		spec.Trackers, lost = c.filterTrackers(spec.Trackers)
	}

	t, isNew, err := c.cl.AddTorrentSpec(spec)
	if err != nil {
		if cerr := st.Close(); cerr != nil {
			slog.Warn("close torrent storage", "error", cerr)
		}
		return nil, err
	}
	if !isNew {
		// t.MergeSpec (called internally by AddTorrentSpec here) ignores the
		// Storage this spec carried, so st was never wired to t: it is safe,
		// and necessary, to close it ourselves rather than leave it attached
		// to nothing.
		if cerr := st.Close(); cerr != nil {
			slog.Warn("close torrent storage", "error", cerr)
		}
		return nil, fmt.Errorf("%w: %s", errTorrentAlreadyAdded, t.InfoHash().HexString())
	}
	// AddTorrentOpts.DisallowData* are declared but never read by the engine,
	// so a torrent starts fully enabled and has to be gated after it is added.
	t.DisallowDataDownload()
	t.DisallowDataUpload()
	t.SetMaxEstablishedConns(maxTorrentConns)
	if c.halted() {
		haltTorrent(t)
	}
	if len(lost) > 0 && c.retryTrackers != nil {
		c.retryTrackers(t, lost)
	}
	return &liveTorrent{t: t, storage: st, flat: opts.flat, announce: announce, gen: c.gen}, nil
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

	// announce is the tracker list as it came in, kept when the client had to
	// drop trackers it cannot reach, so that the stored torrent still has them
	// the day the proxy is switched off.
	announce [][]string
	gen      uint64
}

func (l *liveTorrent) metainfo() metainfo.MetaInfo {
	mi := l.t.Metainfo()
	if l.announce != nil {
		mi.AnnounceList = cloneTiers(l.announce)
	}
	return mi
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

// fileBytes counts chunks that are written but not hashed yet, so a file reads
// as full while its .part is still being flushed and renamed by the storage.
// filesHashed reports piece completion, which the engine publishes only after
// that rename.
func (l *liveTorrent) filesHashed() []bool {
	files := l.t.Files()
	out := make([]bool, len(files))
	for i, f := range files {
		out[i] = fileHashed(f)
	}
	return out
}

func fileHashed(f *torrent.File) bool {
	if f.Length() == 0 {
		return true
	}
	states := f.State()
	if len(states) == 0 {
		return false
	}
	for _, s := range states {
		if !s.Ok || s.Err != nil || !s.Complete || s.Marking || s.Checking {
			return false
		}
	}
	return true
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

// settlePieces waits until no piece is queued for a hash, being hashed or being
// marked in the storage. A piece check returns as soon as the hash is known,
// but the engine tells the storage and publishes the verdict a moment later, so
// the completion read right after the last check can still show pieces that
// passed as not complete. The wait ends with ctx or when the torrent is gone.
func (l *liveTorrent) settlePieces(ctx context.Context) error {
	// Subscribed before the first look, so a change between the look and the
	// wait is delivered and not missed.
	sub := l.t.SubscribePieceStateChanges()
	defer sub.Close()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		select {
		case <-l.t.Closed():
			return errNetworkDown
		default:
		}
		if !l.piecesBusy() {
			return nil
		}
		select {
		case <-ctx.Done():
		case <-l.t.Closed():
		case _, open := <-sub.Values:
			if !open {
				return errNetworkDown
			}
		}
	}
}

func (l *liveTorrent) piecesBusy() bool {
	for _, run := range l.t.PieceStateRuns() {
		if run.Marking || run.Checking {
			return true
		}
	}
	return false
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
