package cloudsaves

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"typhon/internal/account"
	"typhon/internal/library"
	"typhon/internal/savebackup"
	"typhon/internal/settings"
	"typhon/internal/uierr"
)

type remoteSave struct {
	save account.Save
	zip  []byte
}

type uploadCall struct {
	key     string
	ifMatch string
	force   bool
	device  string
	digest  string
	zip     []byte
}

type fakeClient struct {
	mu          sync.Mutex
	remote      map[string]remoteSave
	maxBytes    int64
	listCalls   int
	uploads     []uploadCall
	downloads   int
	deleted     []string
	listErr     error
	uploadErr   error
	downloadErr error
	deleteErr   map[string]error
	beforeSend  func()
	alwaysClash bool
	deadlines   []time.Duration
	tamper      func([]byte) []byte
	// requests counts every listing (under listKey) and transfer (under the
	// cloud key) at the moment it reaches the client, held or not.
	requests map[string]int
	holds    map[string]chan struct{}
	entered  chan string
	inFlight int
}

func deadlineOf(ctx context.Context) time.Duration {
	if d, ok := ctx.Deadline(); ok {
		return time.Until(d)
	}
	return -1
}

func newFakeClient() *fakeClient {
	return &fakeClient{
		remote:    map[string]remoteSave{},
		maxBytes:  100 << 20,
		deleteErr: map[string]error{},
		requests:  map[string]int{},
		holds:     map[string]chan struct{}{},
		entered:   make(chan string, 64),
	}
}

func (c *fakeClient) seed(key, device string, zipBytes []byte, digest string, at time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.remote[key] = remoteSave{
		save: account.Save{GameID: key, SizeBytes: int64(len(zipBytes)), Digest: digest, DeviceID: device, UpdatedAt: at},
		zip:  zipBytes,
	}
}

func (c *fakeClient) calls() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.listCalls + len(c.uploads) + c.downloads + len(c.deleted)
}

func (c *fakeClient) remoteOf(key string) (remoteSave, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	r, ok := c.remote[key]
	return r, ok
}

func (c *fakeClient) ListSaves(ctx context.Context) (account.SaveList, error) {
	leave, err := c.enter(ctx, listKey)
	defer leave()
	if err != nil {
		return account.SaveList{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.listCalls++
	if c.listErr != nil {
		return account.SaveList{}, c.listErr
	}
	list := account.SaveList{QuotaBytes: 1 << 30, MaxBytes: c.maxBytes}
	for _, r := range c.remote {
		list.Games = append(list.Games, r.save)
		list.UsedBytes += r.save.SizeBytes
	}
	slices.SortFunc(list.Games, func(a, b account.Save) int { return strings.Compare(a.GameID, b.GameID) })
	return list, nil
}

func (c *fakeClient) UploadSave(ctx context.Context, key string, body io.ReadSeeker, size int64, up account.SaveUpload) (account.Save, error) {
	leave, err := c.enter(ctx, key)
	defer leave()
	if err != nil {
		return account.Save{}, err
	}
	if c.beforeSend != nil {
		c.beforeSend()
	}
	data, err := io.ReadAll(body)
	if err != nil {
		return account.Save{}, err
	}
	if int64(len(data)) != size {
		return account.Save{}, fmt.Errorf("fake client: size %d, body %d", size, len(data))
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.deadlines = append(c.deadlines, deadlineOf(ctx))
	c.uploads = append(c.uploads, uploadCall{key: key, ifMatch: up.IfMatch, force: up.Force, device: up.DeviceID, digest: up.Digest, zip: data})
	if c.uploadErr != nil {
		return account.Save{}, c.uploadErr
	}
	cur, exists := c.remote[key]
	if !up.Force {
		switch {
		case c.alwaysClash, !exists && up.IfMatch != "":
			return account.Save{}, &account.ErrSaveConflict{}
		case exists && up.IfMatch != cur.save.Digest:
			current := cur.save
			return account.Save{}, &account.ErrSaveConflict{Current: &current}
		}
	}
	saved := account.Save{GameID: key, SizeBytes: size, Digest: up.Digest, DeviceID: up.DeviceID, UpdatedAt: time.Unix(1_800_000_000, 0).UTC()}
	c.remote[key] = remoteSave{save: saved, zip: data}
	return saved, nil
}

func (c *fakeClient) DownloadSave(ctx context.Context, key string, dst io.Writer, maxBytes int64) (account.SaveDownload, error) {
	leave, err := c.enter(ctx, key)
	defer leave()
	if err != nil {
		return account.SaveDownload{}, err
	}
	c.mu.Lock()
	c.downloads++
	c.deadlines = append(c.deadlines, deadlineOf(ctx))
	r, ok := c.remote[key]
	err, tamper := c.downloadErr, c.tamper
	c.mu.Unlock()
	if err != nil {
		return account.SaveDownload{}, err
	}
	if !ok {
		return account.SaveDownload{}, &account.Error{Code: account.CodeSaveNotFound, Status: 404}
	}
	data := r.zip
	if tamper != nil {
		data = tamper(data)
	}
	if int64(len(data)) > maxBytes {
		return account.SaveDownload{}, &account.Error{Code: account.CodeSaveTooLarge}
	}
	if _, err := dst.Write(data); err != nil {
		return account.SaveDownload{}, err
	}
	return account.SaveDownload{Digest: r.save.Digest, Size: int64(len(data))}, nil
}

func (c *fakeClient) DeleteSave(_ context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.deleted = append(c.deleted, key)
	if err := c.deleteErr[key]; err != nil {
		return err
	}
	if _, ok := c.remote[key]; !ok {
		return &account.Error{Code: account.CodeSaveNotFound, Status: 404}
	}
	delete(c.remote, key)
	return nil
}

type fakeLib struct {
	mu      sync.Mutex
	games   []library.Game
	saves   map[string]string
	results map[string]library.SavesResult
	running map[string]bool
	// starting counts the launches of a game the library is holding in its
	// launch hooks: like the library, GetRunningGames reports such a game.
	starting map[string]int
	// gamesGate, while set, holds GetGames (holdGames).
	gamesGate    chan struct{}
	gamesEntered chan struct{}
}

func newFakeLib() *fakeLib {
	return &fakeLib{
		saves:    map[string]string{},
		results:  map[string]library.SavesResult{},
		running:  map[string]bool{},
		starting: map[string]int{},
	}
}

func (l *fakeLib) GetGames() []library.Game {
	l.mu.Lock()
	gate, entered := l.gamesGate, l.gamesEntered
	games := slices.Clone(l.games)
	l.mu.Unlock()
	if gate != nil {
		entered <- struct{}{}
		<-gate
	}
	return games
}

// holdGames keeps every GetGames call open until release, announcing each
// one on the returned channel.
func (l *fakeLib) holdGames() (entered <-chan struct{}, release func()) {
	gate, in := make(chan struct{}), make(chan struct{}, 16)
	l.mu.Lock()
	l.gamesGate, l.gamesEntered = gate, in
	l.mu.Unlock()
	var once sync.Once
	return in, func() {
		once.Do(func() {
			l.mu.Lock()
			l.gamesGate, l.gamesEntered = nil, nil
			l.mu.Unlock()
			close(gate)
		})
	}
}

func (l *fakeLib) LocateSaves(_ context.Context, id string) (library.SavesResult, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if res, ok := l.results[id]; ok {
		return res, nil
	}
	return library.SavesResult{Path: l.saves[id]}, nil
}

func (l *fakeLib) GetRunningGames() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	var ids []string
	for id, on := range l.running {
		if on {
			ids = append(ids, id)
		}
	}
	for id, n := range l.starting {
		if n > 0 && !l.running[id] {
			ids = append(ids, id)
		}
	}
	return ids
}

func (l *fakeLib) setRunning(id string, on bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.running[id] = on
}

func (l *fakeLib) processRunning(id string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.running[id]
}

func (l *fakeLib) launching(id string, delta int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.starting[id] += delta
}

type snapshotCall struct {
	gameID string
	src    string
	kind   savebackup.Kind
	files  map[string]string
}

type fakeBackups struct {
	mu        sync.Mutex
	root      string
	lib       *fakeLib
	snapshots []snapshotCall
	restores  []string
	// paths names the method of every restore that went through:
	// "restore" or "before launch".
	paths       []string
	snapshotErr error
	afterErr    error
	restoreErr  error
	copies      map[string]string
}

func newFakeBackups(root string, lib *fakeLib) *fakeBackups {
	return &fakeBackups{root: root, lib: lib, copies: map[string]string{}}
}

func (b *fakeBackups) SnapshotPath(_ context.Context, gameID, src string, kind savebackup.Kind) (savebackup.Snapshot, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.snapshotErr != nil {
		return savebackup.Snapshot{}, b.snapshotErr
	}
	id := fmt.Sprintf("snap-%d", len(b.snapshots)+1)
	copyDir := filepath.Join(b.root, id)
	files, err := readTreeErr(src)
	if err != nil {
		return savebackup.Snapshot{}, err
	}
	if err := writeTreeErr(copyDir, files); err != nil {
		return savebackup.Snapshot{}, err
	}
	b.snapshots = append(b.snapshots, snapshotCall{gameID: gameID, src: src, kind: kind, files: files})
	b.copies[id] = copyDir
	return savebackup.Snapshot{ID: id, GameID: gameID, Kind: kind, Path: copyDir}, b.afterErr
}

// errFakeRunning is what savebackup answers for a game it must not restore.
var errFakeRunning = uierr.New(codeBackupRunning, "the game is running")

// Restore refuses a game the library reports as running or starting, as
// savebackup.Restore does.
func (b *fakeBackups) Restore(_ context.Context, gameID, snapshotID string) error {
	if slices.Contains(b.lib.GetRunningGames(), gameID) {
		return errFakeRunning
	}
	return b.restore(gameID, snapshotID, "restore")
}

// RestoreBeforeLaunch refuses only a game whose process runs, as
// savebackup.RestoreBeforeLaunch does.
func (b *fakeBackups) RestoreBeforeLaunch(_ context.Context, gameID, snapshotID string) error {
	if b.lib.processRunning(gameID) {
		return errFakeRunning
	}
	return b.restore(gameID, snapshotID, "before launch")
}

func (b *fakeBackups) restore(gameID, snapshotID, path string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.restoreErr != nil {
		return b.restoreErr
	}
	copyDir, ok := b.copies[snapshotID]
	if !ok {
		return fmt.Errorf("fake backups: no snapshot %q", snapshotID)
	}
	files, err := readTreeErr(copyDir)
	if err != nil {
		return err
	}
	dest := b.lib.saves[gameID]
	if err := os.RemoveAll(dest); err != nil {
		return err
	}
	if err := writeTreeErr(dest, files); err != nil {
		return err
	}
	b.restores = append(b.restores, snapshotID)
	b.paths = append(b.paths, path)
	return nil
}

func (b *fakeBackups) restorePaths() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return slices.Clone(b.paths)
}

func readTreeErr(root string) (map[string]string, error) {
	var rels []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rels = append(rels, rel)
		return nil
	})
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, rel := range rels {
		data, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			return nil, err
		}
		out[filepath.ToSlash(rel)] = string(data)
	}
	return out, nil
}

func writeTreeErr(root string, files map[string]string) error {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return err
	}
	for rel, content := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			return err
		}
	}
	return nil
}

func readTree(t *testing.T, root string) map[string]string {
	t.Helper()
	out, err := readTreeErr(root)
	if err != nil {
		t.Fatalf("read tree %s: %v", root, err)
	}
	return out
}

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	if err := writeTreeErr(root, files); err != nil {
		t.Fatalf("write tree %s: %v", root, err)
	}
}

// contractDigest computes the digest the way the API contract spells it,
// independently of the implementation under test.
func contractDigest(files map[string]string) string {
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	slices.Sort(names)
	h := sha256.New()
	for _, n := range names {
		sum := sha256.Sum256([]byte(files[n]))
		h.Write([]byte(n + "\n" + hex.EncodeToString(sum[:]) + "\n"))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func zipOf(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	slices.Sort(names)
	for _, n := range names {
		w, err := zw.Create(n)
		if err != nil {
			t.Fatalf("zip create %q: %v", n, err)
		}
		if _, err := w.Write([]byte(files[n])); err != nil {
			t.Fatalf("zip write %q: %v", n, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}

func unzip(t *testing.T, data []byte) map[string]string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("open uploaded zip: %v", err)
	}
	out := map[string]string{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open %q: %v", f.Name, err)
		}
		b, err := io.ReadAll(rc)
		if err != nil {
			t.Fatalf("read %q: %v", f.Name, err)
		}
		if err := rc.Close(); err != nil {
			t.Fatalf("close %q: %v", f.Name, err)
		}
		out[f.Name] = string(b)
	}
	return out
}

type harness struct {
	t       *testing.T
	dir     string
	client  *fakeClient
	lib     *fakeLib
	backups *fakeBackups
	svc     *Service
	events  chan Event

	mu      sync.Mutex
	cfg     settings.Settings
	token   string
	clock   time.Time
	acctErr error
	// stored is the account of the stored credential; storedCalls counts
	// who asked for it.
	stored      string
	storedCalls int
}

const (
	gameLocal = "local-1"
	gameCloud = "cloud-1"
	ownerID   = "account-1"
)

func newHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{
		t:      t,
		dir:    t.TempDir(),
		client: newFakeClient(),
		lib:    newFakeLib(),
		events: make(chan Event, 256),
		cfg:    settings.Defaults(),
		token:  "token",
		stored: ownerID,
		clock:  time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC),
	}
	h.cfg.CloudSaves = true
	h.backups = newFakeBackups(filepath.Join(t.TempDir(), "backups"), h.lib)
	svc, err := NewService(h.dir, h.config, h.tokenFn, h.accountFn, h.storedFn, h.client, h.lib, h.backups)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	svc.now = h.tick
	svc.emit = func(ev Event) { h.events <- ev }
	h.svc = svc
	h.addGame(gameLocal, gameCloud)
	return h
}

func (h *harness) config() settings.Settings {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.cfg
}

func (h *harness) tokenFn() (string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.token, nil
}

func (h *harness) accountFn(string) (string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.acctErr != nil {
		return "", h.acctErr
	}
	return ownerID, nil
}

func (h *harness) storedFn(string) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.storedCalls++
	return h.stored
}

func (h *harness) setStored(owner string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.stored = owner
}

func (h *harness) storedAsked() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.storedCalls
}

func (h *harness) setAccountErr(err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.acctErr = err
}

func (h *harness) tick() time.Time {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clock = h.clock.Add(time.Second)
	return h.clock
}

func (h *harness) setEnabled(on bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cfg.CloudSaves = on
}

func (h *harness) setToken(tok string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.token = tok
}

func (h *harness) addGame(local, cloud string) string {
	h.t.Helper()
	saves := filepath.Join(h.t.TempDir(), "Saves-"+local)
	if err := os.MkdirAll(saves, 0o700); err != nil {
		h.t.Fatalf("mkdir saves: %v", err)
	}
	h.lib.mu.Lock()
	h.lib.games = append(h.lib.games, library.Game{ID: local, Title: local, CanonicalGameID: cloud})
	h.lib.saves[local] = saves
	h.lib.mu.Unlock()
	return saves
}

func (h *harness) markGame(id string, uninstalled, archived bool) {
	h.lib.mu.Lock()
	defer h.lib.mu.Unlock()
	for i := range h.lib.games {
		if h.lib.games[i].ID == id {
			h.lib.games[i].Uninstalled = uninstalled
			h.lib.games[i].Archived = archived
		}
	}
}

func (h *harness) saves() string {
	return h.lib.saves[gameLocal]
}

func (h *harness) pass() {
	h.t.Helper()
	h.svc.pass(context.Background(), nil)
}

func (h *harness) drain() []Event {
	var out []Event
	for {
		select {
		case ev := <-h.events:
			out = append(out, ev)
		default:
			return out
		}
	}
}

func (h *harness) status(local string) GameStatus {
	h.t.Helper()
	for _, g := range h.svc.Status().Games {
		if g.GameID == local {
			return g
		}
	}
	h.t.Fatalf("no status for %s in %+v", local, h.svc.Status().Games)
	return GameStatus{}
}

func (h *harness) recorded(key string) gameState {
	return h.svc.gameState(key)
}

// listKey is the key under which the listing request is counted and held.
const listKey = "<list>"

// enter counts a request and, while its key is held, keeps it open until the
// hold is released or ctx ends, as a slow transfer on a live network would.
func (c *fakeClient) enter(ctx context.Context, key string) (leave func(), err error) {
	c.mu.Lock()
	c.requests[key]++
	c.inFlight++
	gate := c.holds[key]
	c.mu.Unlock()
	leave = func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.inFlight--
	}
	if gate == nil {
		return leave, nil
	}
	c.entered <- key
	select {
	case <-gate:
		return leave, nil
	case <-ctx.Done():
		return leave, ctx.Err()
	}
}

// hold keeps every later request for key open until release is called.
func (c *fakeClient) hold(key string) (release func()) {
	gate := make(chan struct{})
	c.mu.Lock()
	c.holds[key] = gate
	c.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			c.mu.Lock()
			delete(c.holds, key)
			c.mu.Unlock()
			close(gate)
		})
	}
}

// waitHeld returns once a request for key is waiting on its hold.
func (c *fakeClient) waitHeld(t *testing.T, key string) {
	t.Helper()
	timeout := time.NewTimer(10 * time.Second)
	defer timeout.Stop()
	for {
		select {
		case got := <-c.entered:
			if got == key {
				return
			}
		case <-timeout.C:
			t.Fatalf("no request for %s reached the cloud within 10s", key)
		}
	}
}

func (c *fakeClient) requestsFor(key string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.requests[key]
}

func (c *fakeClient) inFlightNow() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.inFlight
}

func (c *fakeClient) transfers() (uploads, downloads int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.uploads), c.downloads
}

func (b *fakeBackups) restoreCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.restores)
}

// restart replaces the service with a new one on the same directory and the
// same fakes, as a relaunch of the app would.
func (h *harness) restart() {
	h.t.Helper()
	if err := h.svc.ServiceShutdown(); err != nil {
		h.t.Fatalf("shutdown before restart: %v", err)
	}
	svc, err := NewService(h.dir, h.config, h.tokenFn, h.accountFn, h.storedFn, h.client, h.lib, h.backups)
	if err != nil {
		h.t.Fatalf("new service on the same directory: %v", err)
	}
	svc.now = h.tick
	svc.emit = func(ev Event) { h.events <- ev }
	h.svc = svc
	h.open()
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	if cond() {
		return
	}
	ticker := time.NewTicker(2 * time.Millisecond)
	defer ticker.Stop()
	timeout := time.NewTimer(10 * time.Second)
	defer timeout.Stop()
	for {
		select {
		case <-ticker.C:
			if cond() {
				return
			}
		case <-timeout.C:
			t.Fatalf("timed out waiting for %s", what)
		}
	}
}

// launchCheckRuns reports whether a launch check of gameID has not ended.
func (s *Service) launchCheckRuns(gameID string) bool {
	_, runs := s.launchWaiters(gameID)
	return runs
}

// launchWaiters reports how many launches of gameID wait on its check.
func (s *Service) launchWaiters(gameID string) (waiters int, runs bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.launches {
		if c.gameID == gameID {
			return c.waiters, true
		}
	}
	return 0, false
}

// advance moves the service's clock forward by d.
func (h *harness) advance(d time.Duration) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clock = h.clock.Add(d)
}

// permitHeld reports whether a launch permission of gameID is held.
func (s *Service) permitHeld(gameID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.permits[gameID]
	return ok
}
