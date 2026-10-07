package download

import (
	"context"
	"errors"
	"net/netip"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"typhon/internal/account"
	"typhon/internal/settings"
	"typhon/internal/uierr"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/storage"
	"github.com/wailsapp/wails/v3/pkg/application"
)

type emitLog struct {
	mu     sync.Mutex
	events []capturedEmit
}

// recordEmits is captureEmits for tests whose events come from more than one
// goroutine. It must be called before the manager starts anything and its
// cleanup runs after the manager is shut down, so the swap of the hook itself
// never overlaps a reader.
func recordEmits(t *testing.T) *emitLog {
	t.Helper()
	l := &emitLog{}
	original := emit
	emit = func(name string, data any) {
		l.mu.Lock()
		l.events = append(l.events, capturedEmit{name: name, data: data})
		l.mu.Unlock()
	}
	t.Cleanup(func() { emit = original })
	return l
}

func (l *emitLog) count(name string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := 0
	for _, e := range l.events {
		if e.name == name {
			n++
		}
	}
	return n
}

func (l *emitLog) networks() []NetworkState {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []NetworkState
	for _, e := range l.events {
		if st, ok := e.data.(NetworkState); ok && e.name == eventNetwork {
			out = append(out, st)
		}
	}
	return out
}

func (l *emitLog) statusesOf(id string) []Status {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []Status
	for _, e := range l.events {
		if d, ok := e.data.(Download); ok && d.ID == id {
			out = append(out, d.Status)
		}
	}
	return out
}

func (l *emitLog) mark() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.events)
}

func (l *emitLog) statusesSince(id string, from int) []Status {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []Status
	for _, e := range l.events[from:] {
		if d, ok := e.data.(Download); ok && d.ID == id {
			out = append(out, d.Status)
		}
	}
	return out
}

type fakeNetwork struct {
	mu       sync.Mutex
	ifaces   []ifaceInfo
	listErr  error
	probeErr error
	probed   []string
	dns      []netip.Addr
	dnsErr   error
	hostErr  error
	checks   []hostCheckCall

	probeEntered chan struct{}
	probeHold    chan struct{}
}

type hostCheckCall struct {
	name   string
	v4, v6 bool
}

func (f *fakeNetwork) dnsOf(ifaceInfo) ([]netip.Addr, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]netip.Addr(nil), f.dns...), f.dnsErr
}

func (f *fakeNetwork) hostCheck(ifc ifaceInfo, v4, v6 bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.checks = append(f.checks, hostCheckCall{ifc.Name, v4, v6})
	return f.hostErr
}

func (f *fakeNetwork) setDNS(servers ...string) {
	f.mu.Lock()
	f.dns = nil
	for _, s := range servers {
		f.dns = append(f.dns, netip.MustParseAddr(s))
	}
	f.mu.Unlock()
}

func (f *fakeNetwork) checked() []hostCheckCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]hostCheckCall(nil), f.checks...)
}

func (f *fakeNetwork) interfaces() ([]ifaceInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.listErr != nil {
		return nil, f.listErr
	}
	return append([]ifaceInfo(nil), f.ifaces...), nil
}

func (f *fakeNetwork) probe(ctx context.Context, addr string) error {
	f.mu.Lock()
	f.probed = append(f.probed, addr)
	err := f.probeErr
	entered, hold := f.probeEntered, f.probeHold
	f.mu.Unlock()
	if hold != nil {
		select {
		case entered <- struct{}{}:
		default:
		}
		select {
		case <-hold:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return err
}

// holdProbe makes the next probes wait until the returned release is called;
// entered receives one value per probe that starts waiting.
func (f *fakeNetwork) holdProbe() (entered <-chan struct{}, release func()) {
	enter, hold := make(chan struct{}, 4), make(chan struct{})
	f.mu.Lock()
	f.probeEntered, f.probeHold = enter, hold
	f.mu.Unlock()
	var once sync.Once
	return enter, func() { once.Do(func() { close(hold) }) }
}

func (f *fakeNetwork) set(ifaces ...ifaceInfo) {
	f.mu.Lock()
	f.ifaces = ifaces
	f.mu.Unlock()
}

func (f *fakeNetwork) setProbe(err error) {
	f.mu.Lock()
	f.probeErr = err
	f.mu.Unlock()
}

func vpnIface(ip string) ifaceInfo {
	return ifaceInfo{Name: "vpn0", Index: 7, Up: true, Addrs: []netip.Addr{netip.MustParseAddr(ip)}}
}

type memStore struct {
	mu      sync.Mutex
	cred    account.Credential
	present bool
	loadErr error
	saveErr error
	users   []string
	saved   int
	deleted int
	loads   int
	// loadHold, when set, makes Load signal loadEntered once it has read the
	// credential and then wait for loadHold to be closed, so a test can change
	// the store while a reader is in the middle of a read.
	loadEntered chan struct{}
	loadHold    chan struct{}
}

func (s *memStore) Load() (account.Credential, error) {
	s.mu.Lock()
	s.loads++
	cred, err := s.cred, error(nil)
	switch {
	case s.loadErr != nil:
		cred, err = account.Credential{}, s.loadErr
	case !s.present:
		cred, err = account.Credential{}, account.ErrNoCredential
	}
	entered, hold := s.loadEntered, s.loadHold
	s.mu.Unlock()
	if hold != nil {
		select {
		case entered <- struct{}{}:
		default:
		}
		<-hold
	}
	return cred, err
}

func (s *memStore) Save(c account.Credential) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.saveErr != nil {
		return s.saveErr
	}
	s.cred, s.present = c, true
	s.users = append(s.users, c.Username)
	s.saved++
	return nil
}

func (s *memStore) Delete() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cred, s.present = account.Credential{}, false
	s.deleted++
	return nil
}

type buildLog struct {
	mu      sync.Mutex
	plans   []netPlan
	clients []*client
	failing error
	// hold, when set, is signalled once a build starts and then blocks it
	// until release is closed.
	hold    chan struct{}
	release chan struct{}
}

func (b *buildLog) build(_ context.Context, cfg settings.Settings, metaDir string, completion storage.PieceCompletion, plan netPlan) (*client, error) {
	b.mu.Lock()
	hold, release := b.hold, b.release
	b.mu.Unlock()
	if hold != nil {
		hold <- struct{}{}
		<-release
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.plans = append(b.plans, plan)
	if b.failing != nil {
		return nil, b.failing
	}
	wrapped := nonClosingCompletion{completion}
	tc := clientConfig(cfg, metaDir, 0, wrapped)
	tc.NoDHT = true
	tc.DisableTrackers = true
	tc.DisablePEX = true
	tc.NoDefaultPortForwarding = true
	cl, err := torrent.NewClient(tc)
	if err != nil {
		closeDefaultStorage(tc)
		return nil, err
	}
	c := &client{
		cl: cl, down: tc.DownloadRateLimiter, up: tc.UploadRateLimiter, metaDir: metaDir, completion: wrapped,
		httpTrackersOnly: plan.mode == settings.NetworkProxy,
	}
	b.clients = append(b.clients, c)
	return c, nil
}

func (b *buildLog) built() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.plans)
}

func (b *buildLog) lastPlan() netPlan {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.plans[len(b.plans)-1]
}

func (b *buildLog) fail(err error) {
	b.mu.Lock()
	b.failing = err
	b.mu.Unlock()
}

func clientClosed(c *client) bool {
	select {
	case <-c.cl.Closed():
		return true
	default:
		return false
	}
}

type netRig struct {
	m      *Manager
	svc    *settings.Service
	net    *fakeNetwork
	builds *buildLog
	store  *memStore
}

func newNetRig(t *testing.T, mutate func(*settings.Settings)) *netRig {
	t.Helper()
	cfg := settings.Defaults()
	if mutate != nil {
		mutate(&cfg)
	}
	m, svc := newManagerWithSettings(t, cfg)
	r := &netRig{m: m, svc: svc, net: &fakeNetwork{}, builds: &buildLog{}, store: &memStore{}}
	r.net.setDNS("10.8.0.1")
	m.netEnv = netEnv{interfaces: r.net.interfaces, probe: r.net.probe, dns: r.net.dnsOf, hostCheck: r.net.hostCheck}
	m.buildClient = r.builds.build
	m.proxyStore = r.store
	t.Cleanup(m.teardownClient)
	return r
}

func viaInterface(s *settings.Settings) {
	s.NetworkMode = settings.NetworkInterface
	s.NetworkInterface = "vpn0"
}

func viaProxy(s *settings.Settings) {
	s.NetworkMode = settings.NetworkProxy
	s.ProxyType = settings.ProxySOCKS5
	s.ProxyHost = "127.0.0.1"
	s.ProxyPort = 1080
}

func (r *netRig) reconcile(t *testing.T) {
	t.Helper()
	r.m.reconcileNetwork(t.Context())
}

func (r *netRig) state() NetworkState {
	return r.m.NetworkStatus()
}

func (r *netRig) client() *client {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	return r.m.client
}

func TestDirectReconcileBuildsTheClient(t *testing.T) {
	r := newNetRig(t, nil)
	log := recordEmits(t)
	r.reconcile(t)

	if st := r.state(); st.State != NetworkOK || st.Mode != settings.NetworkDirect || st.Code != "" {
		t.Fatalf("state = %+v", st)
	}
	if r.client() == nil {
		t.Fatal("no client after the check")
	}
	r.reconcile(t)
	r.reconcile(t)
	if n := r.builds.built(); n != 1 {
		t.Fatalf("client built %d times, want once: an unchanged route must not rebuild it", n)
	}
	if got := len(log.networks()); got != 1 {
		t.Fatalf("network events = %+v, want exactly one", log.networks())
	}
	if r.builds.lastPlan().mode != settings.NetworkDirect {
		t.Fatalf("plan = %+v", r.builds.lastPlan())
	}
}

func TestNetworkStatusBeforeAnyCheckReportsTheConfiguredMode(t *testing.T) {
	r := newNetRig(t, viaProxy)
	st := r.state()
	if st.Mode != settings.NetworkProxy || st.State != NetworkOK {
		t.Fatalf("state = %+v", st)
	}
}

func parkFixture(t *testing.T, r *netRig) (ids map[string]*fakeTorrent, pendingHash string) {
	t.Helper()
	ids = map[string]*fakeTorrent{}
	add := func(id string, status Status, seeding bool) {
		d := r.m.addTestItem(id, status)
		eng := &fakeTorrent{size: 100}
		r.m.mu.Lock()
		d.Seeding = seeding
		d.Downloaded = 40
		d.Peers, d.Seeders, d.DownloadSpeed = 3, 1, 999
		r.m.engines[id] = eng
		r.m.mu.Unlock()
		ids[id] = eng
	}
	add("dl", StatusDownloading, false)
	add("verifying", StatusVerifying, false)
	add("paused", StatusPaused, false)
	add("seed", StatusCompleted, true)
	add("failed", StatusFailed, false)
	r.m.addTestItem("done", StatusCompleted)

	cl := r.client()
	lt, err := cl.addMagnet("magnet:?xt=urn:btih:a748597437835a2fd0d2e06f8edd86fee316a84d&dn=x", cl.metaDir, storageOpts{})
	if err != nil {
		t.Fatalf("add pending torrent: %v", err)
	}
	pendingHash = lt.t.InfoHash().HexString()
	r.m.mu.Lock()
	r.m.pending[pendingHash] = &pending{torrent: lt, source: "magnet"}
	r.m.mu.Unlock()
	return ids, pendingHash
}

func TestLostInterfaceParksDownloadsInsteadOfFailingThem(t *testing.T) {
	r := newNetRig(t, viaInterface)
	log := recordEmits(t)
	r.net.set(vpnIface("10.8.0.2"))
	r.reconcile(t)
	if st := r.state(); st.State != NetworkOK || st.Address != "10.8.0.2" {
		t.Fatalf("state = %+v", st)
	}
	if p := r.builds.lastPlan(); p.mode != settings.NetworkInterface || p.iface != "vpn0" || p.ip4 != netip.MustParseAddr("10.8.0.2") {
		t.Fatalf("plan = %+v", p)
	}
	engines, _ := parkFixture(t, r)
	old := r.client()

	// A verify job in flight, as after a restart: the teardown has to stop it
	// and wait for it.
	jobCtx, ok := r.m.beginJob(t.Context(), "verifying")
	if !ok {
		t.Fatal("beginJob")
	}
	jobStopped := make(chan struct{})
	go func() {
		<-jobCtx.Done()
		r.m.endJob("verifying")
		close(jobStopped)
	}()
	fetchCancelled := make(chan struct{})
	r.m.mu.Lock()
	r.m.fetching["magnet:x"] = fetchEntry{id: 1, cancel: func() { close(fetchCancelled) }}
	r.m.mu.Unlock()

	r.net.set()
	r.reconcile(t)

	st := r.state()
	if st.State != NetworkDown || st.Code != "download.net_interface_missing" || st.Mode != settings.NetworkInterface || st.Reason == "" {
		t.Fatalf("state = %+v", st)
	}
	if strings.Contains(st.Reason, "typhon:") {
		t.Fatalf("the reason still carries the code prefix: %q", st.Reason)
	}
	if r.client() != nil {
		t.Fatal("the client is still installed")
	}
	if !clientClosed(old) {
		t.Fatal("the old client was not closed")
	}
	select {
	case <-jobStopped:
	default:
		t.Fatal("the in-flight job was not stopped and waited for")
	}
	select {
	case <-fetchCancelled:
	default:
		t.Fatal("the in-flight metadata fetch was not cancelled")
	}
	r.m.mu.Lock()
	gotEngines, gotPending := len(r.m.engines), len(r.m.pending)
	r.m.mu.Unlock()
	if gotEngines != 0 || gotPending != 0 {
		t.Fatalf("engines = %d, pending = %d, want both dropped", gotEngines, gotPending)
	}
	for id, eng := range engines {
		if !eng.wasDropped() {
			t.Errorf("engine of %s was not dropped", id)
		}
	}

	want := map[string]Status{
		"dl": StatusQueued, "verifying": StatusQueued, "paused": StatusPaused,
		"seed": StatusCompleted, "failed": StatusFailed, "done": StatusCompleted,
	}
	assertStatuses(t, r.m, want)
	seed, err := r.m.Get("seed")
	if err != nil || !seed.Seeding {
		t.Fatalf("a seeding download keeps its wish to seed: %+v, %v", seed, err)
	}
	dl := mustGet(t, r.m, "dl")
	if dl.Downloaded != 40 || dl.DownloadSpeed != 0 || dl.Peers != 0 || dl.Seeders != 0 || dl.Error != "" {
		t.Fatalf("a parked download keeps its progress and drops the live numbers: %+v", dl)
	}
	if log.count(eventFailed) != 0 {
		t.Fatal("a lost route must not emit download:failed")
	}
	downs := 0
	for _, n := range log.networks() {
		if n.State == NetworkDown {
			downs++
		}
	}
	if downs != 1 {
		t.Fatalf("network events = %+v, want one down event", log.networks())
	}

	records, err := r.m.store.load()
	if err != nil {
		t.Fatalf("load store: %v", err)
	}
	for _, rec := range records {
		if rec.ID == "dl" && rec.Status != StatusQueued {
			t.Fatalf("stored status of a parked download = %s, want queued", rec.Status)
		}
	}
}

func TestParkingSurvivesAFailingPersist(t *testing.T) {
	r := newNetRig(t, viaInterface)
	r.net.set(vpnIface("10.8.0.2"))
	r.reconcile(t)
	r.m.addTestDownload("dl")
	breakStore(t, r.m)

	r.net.set()
	r.reconcile(t)

	if st := r.state(); st.State != NetworkDown {
		t.Fatalf("state = %+v", st)
	}
	assertStatuses(t, r.m, map[string]Status{"dl": StatusQueued})
	if st := r.m.degradedStatus(); !st.Degraded {
		t.Fatal("a store that cannot be written must surface as degraded")
	}
}

func TestRequestsWhileDownReportTheNetworkNotTheClient(t *testing.T) {
	r := newNetRig(t, viaInterface)
	r.reconcile(t)
	if st := r.state(); st.State != NetworkDown {
		t.Fatalf("state = %+v", st)
	}
	dir := t.TempDir()

	cases := map[string]func() error{
		"FetchMetadata": func() error {
			_, err := r.m.FetchMetadata("magnet:?xt=urn:btih:a748597437835a2fd0d2e06f8edd86fee316a84d")
			return err
		},
		"StartDownloadFrom": func() error {
			_, err := r.m.StartDownloadFrom("a748597437835a2fd0d2e06f8edd86fee316a84d", dir, nil, Origin{})
			return err
		},
		"AddTask": func() error {
			_, err := r.m.AddTask(t.Context(), AddRequest{Source: "magnet:?xt=urn:btih:a748597437835a2fd0d2e06f8edd86fee316a84d", Destination: dir})
			return err
		},
		"InspectReuse": func() error {
			_, err := r.m.InspectReuse(t.Context(), ReuseRequest{Path: dir, InfoHash: "a748597437835a2fd0d2e06f8edd86fee316a84d"}, nil)
			return err
		},
	}
	for name, call := range cases {
		err := call()
		if !errors.Is(err, errNetworkDown) {
			t.Errorf("%s while down: %v, want errNetworkDown", name, err)
		}
		if uierr.Code(err) != "download.network_down" {
			t.Errorf("%s code = %q", name, uierr.Code(err))
		}
	}

	bare := newTestManager(t, 1)
	if _, err := bare.FetchMetadata("magnet:?xt=urn:btih:a748597437835a2fd0d2e06f8edd86fee316a84d"); !errors.Is(err, errNoClient) || errors.Is(err, errNetworkDown) {
		t.Fatalf("a client that never started is not a network failure: %v", err)
	}
}

func TestRestoreWhileDownWaitsForTheNetwork(t *testing.T) {
	r := newNetRig(t, viaInterface)
	r.reconcile(t)
	r.m.addTestItem("q", StatusQueued)
	r.m.mu.Lock()
	r.m.wg.Add(1)
	r.m.mu.Unlock()
	r.m.restore()

	assertStatuses(t, r.m, map[string]Status{"q": StatusQueued})
	r.m.mu.Lock()
	all := r.m.resume != nil && r.m.resume.all
	r.m.mu.Unlock()
	if !all {
		t.Fatal("a restore that finds the network down must be remembered for when it is back")
	}
}

func TestResumeAndForceStartWhileDownQueueTheDownload(t *testing.T) {
	r := newNetRig(t, viaInterface)
	r.reconcile(t)
	r.m.addTestItem("paused", StatusPaused)
	failed := r.m.addTestItem("failed", StatusFailed)
	r.m.mu.Lock()
	failed.Error = "old failure"
	r.m.mu.Unlock()

	if err := r.m.Resume("paused"); err != nil {
		t.Fatalf("Resume while down: %v", err)
	}
	if err := r.m.ForceStart("failed"); err != nil {
		t.Fatalf("ForceStart while down: %v", err)
	}
	assertStatuses(t, r.m, map[string]Status{"paused": StatusQueued, "failed": StatusQueued})
	if d := mustGet(t, r.m, "failed"); d.Error != "" {
		t.Fatalf("error = %q, want it cleared", d.Error)
	}
	r.m.mu.Lock()
	ids := r.m.resume.ids
	r.m.mu.Unlock()
	if e, ok := ids["paused"]; !ok || e.trusted || e.force {
		t.Fatalf("resume ids = %v: a resumed download comes back with a check and without force", ids)
	}
	if e, ok := ids["failed"]; !ok || e.trusted || !e.force {
		t.Fatalf("resume ids = %v: a forced download keeps the force", ids)
	}
}

func TestResumeWhileDownRollsBackOnAFailingPersist(t *testing.T) {
	r := newNetRig(t, viaInterface)
	r.reconcile(t)
	r.m.addTestItem("paused", StatusPaused)
	breakStore(t, r.m)

	if err := r.m.Resume("paused"); err == nil {
		t.Fatal("Resume must return the persist failure")
	}
	assertStatuses(t, r.m, map[string]Status{"paused": StatusPaused})
	r.m.mu.Lock()
	queued := r.m.resume != nil && len(r.m.resume.ids) > 0
	r.m.mu.Unlock()
	if queued {
		t.Fatal("a rolled back download must not be scheduled to come back")
	}
}

func TestSeedingToggledWhileDownComesBackWithTheClient(t *testing.T) {
	r := newNetRig(t, func(s *settings.Settings) { viaInterface(s); s.SeedAfterDownload = false })
	r.net.set(vpnIface("10.8.0.2"))
	r.reconcile(t)
	r.m.addTestItem("done", StatusCompleted)
	r.net.set()
	r.reconcile(t)

	next := r.svc.GetSettings()
	next.SeedAfterDownload = true
	if err := r.svc.SaveSettings(next); err != nil {
		t.Fatal(err)
	}
	r.m.applySettings(next)

	d, err := r.m.Get("done")
	if err != nil || !d.Seeding {
		t.Fatalf("done = %+v, %v: the wish to seed is kept while nothing can seed", d, err)
	}
	r.m.mu.Lock()
	_, queued := r.m.resume.ids["done"]
	r.m.mu.Unlock()
	if !queued {
		t.Fatal("the download must be restored to seed once the network is back")
	}
}

func TestStartInDownIsNotAFailure(t *testing.T) {
	dir := t.TempDir()
	first := mustManagerAt(t, dir)
	first.addTestItem("dl", StatusDownloading)
	first.addTestItem("seed", StatusCompleted)

	cfg := settings.Defaults()
	viaInterface(&cfg)
	svc, err := settings.NewServiceAt(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.SaveSettings(cfg); err != nil {
		t.Fatal(err)
	}
	log := recordEmits(t)
	m, err := newManagerAt(dir, svc)
	if err != nil {
		t.Fatalf("newManagerAt: %v", err)
	}
	fake, builds := &fakeNetwork{}, &buildLog{}
	m.netEnv = netEnv{interfaces: fake.interfaces, probe: fake.probe, dns: fake.dnsOf, hostCheck: fake.hostCheck}
	m.buildClient = builds.build
	m.netInterval = time.Millisecond
	if err := first.ServiceShutdown(); err != nil {
		t.Fatal(err)
	}

	if err := m.ServiceStartup(t.Context(), application.ServiceOptions{}); err != nil {
		t.Fatalf("startup: %v", err)
	}
	t.Cleanup(func() {
		if err := m.ServiceShutdown(); err != nil {
			t.Errorf("shutdown: %v", err)
		}
	})

	waitUntil(t, "the first check to conclude", func() bool { return m.NetworkStatus().Code == "download.net_interface_missing" })
	st := m.NetworkStatus()
	if st.State != NetworkDown || st.Mode != settings.NetworkInterface {
		t.Fatalf("state = %+v", st)
	}
	if builds.built() != 0 {
		t.Fatal("a client was built with no adapter to bind to")
	}
	assertStatuses(t, m, map[string]Status{"dl": StatusQueued, "seed": StatusCompleted})
	if log.count(eventFailed) != 0 {
		t.Fatal("a start without the network must not fail any download")
	}
	m.mu.Lock()
	all := m.resume != nil && m.resume.all
	m.mu.Unlock()
	if !all {
		t.Fatal("nothing ran in this process, so everything is restored the way a start restores it")
	}

	fake.set(vpnIface("10.8.0.2"))
	waitUntil(t, "the monitor to bring the network up", func() bool { return m.NetworkStatus().State == NetworkOK })
	if builds.built() != 1 {
		t.Fatalf("built %d clients, want 1", builds.built())
	}
	m.mu.Lock()
	up := m.client != nil
	m.mu.Unlock()
	if !up {
		t.Fatal("no client after the network came back")
	}
}

func TestMonitorTakesTheClientDownAndBack(t *testing.T) {
	r := newNetRig(t, viaInterface)
	r.net.set(vpnIface("10.8.0.2"))
	r.m.netInterval = time.Millisecond
	if err := r.m.ServiceStartup(t.Context(), application.ServiceOptions{}); err != nil {
		t.Fatalf("startup: %v", err)
	}
	t.Cleanup(func() {
		if err := r.m.ServiceShutdown(); err != nil {
			t.Errorf("shutdown: %v", err)
		}
	})
	waitUntil(t, "the network to come up", func() bool { return r.state().State == NetworkOK })
	first := r.client()

	r.net.set(ifaceInfo{Name: "vpn0", Index: 7, Up: false, Addrs: []netip.Addr{netip.MustParseAddr("10.8.0.2")}})
	waitUntil(t, "the adapter going down to be noticed", func() bool { return r.state().Code == "download.net_interface_down" })
	if !clientClosed(first) {
		t.Fatal("traffic may not keep flowing on a client whose adapter is down")
	}

	r.net.set(vpnIface("10.8.0.2"))
	waitUntil(t, "the network to come back", func() bool { return r.state().State == NetworkOK })
	if r.client() == first || r.client() == nil {
		t.Fatal("the network came back without a new client")
	}
}

func TestInterfaceChecks(t *testing.T) {
	cases := []struct {
		name  string
		setup func(*fakeNetwork)
		code  string
	}{
		{"adapter missing", func(f *fakeNetwork) { f.set(ifaceInfo{Name: "eth0", Up: true}) }, "download.net_interface_missing"},
		{"adapter down", func(f *fakeNetwork) {
			f.set(ifaceInfo{Name: "vpn0", Index: 7, Up: false, Addrs: []netip.Addr{netip.MustParseAddr("10.8.0.2")}})
		}, "download.net_interface_down"},
		{"adapter without an address", func(f *fakeNetwork) { f.set(ifaceInfo{Name: "vpn0", Index: 7, Up: true}) }, "download.net_interface_no_address"},
		{"only a link local address", func(f *fakeNetwork) {
			f.set(ifaceInfo{Name: "vpn0", Index: 7, Up: true, Addrs: []netip.Addr{netip.MustParseAddr("169.254.3.3"), netip.MustParseAddr("fe80::1")}})
		}, "download.net_interface_no_address"},
		{"listing fails", func(f *fakeNetwork) { f.mu.Lock(); f.listErr = errors.New("access denied"); f.mu.Unlock() }, "download.net_interface_list_failed"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newNetRig(t, viaInterface)
			c.setup(r.net)
			r.reconcile(t)
			st := r.state()
			if st.State != NetworkDown || st.Code != c.code {
				t.Fatalf("state = %+v, want down with %s", st, c.code)
			}
			if r.client() != nil || r.builds.built() != 0 {
				t.Fatal("a client may not be built while the route is missing")
			}
		})
	}
}

func TestInterfaceBindsTheAddressTheAdapterHas(t *testing.T) {
	r := newNetRig(t, viaInterface)
	r.net.set(ifaceInfo{Name: "VPN0", Index: 7, Up: true, Addrs: []netip.Addr{
		netip.MustParseAddr("fe80::1"), netip.MustParseAddr("10.8.0.2"), netip.MustParseAddr("2001:db8::2"),
	}})
	r.reconcile(t)
	p := r.builds.lastPlan()
	if p.ip4 != netip.MustParseAddr("10.8.0.2") || p.ip6 != netip.MustParseAddr("2001:db8::2") || p.index != 7 {
		t.Fatalf("plan = %+v", p)
	}
	if st := r.state(); st.Address != "10.8.0.2, 2001:db8::2" {
		t.Fatalf("address = %q", st.Address)
	}
}

func TestAddressChangeRebuildsButAnExtraAddressDoesNot(t *testing.T) {
	r := newNetRig(t, viaInterface)
	r.net.set(vpnIface("10.8.0.2"))
	r.reconcile(t)
	first := r.client()

	r.net.set(ifaceInfo{Name: "vpn0", Index: 7, Up: true, Addrs: []netip.Addr{netip.MustParseAddr("10.8.0.2"), netip.MustParseAddr("10.8.0.3"), netip.MustParseAddr("2001:db8::2")}})
	r.reconcile(t)
	if r.client() != first || r.builds.built() != 1 {
		t.Fatal("an adapter that gained addresses keeps the client bound to the one it has")
	}

	r.net.set(vpnIface("10.9.0.7"))
	r.reconcile(t)
	if r.client() == first || !clientClosed(first) {
		t.Fatal("the bound address is gone: the client must be rebuilt")
	}
	if p := r.builds.lastPlan(); p.ip4 != netip.MustParseAddr("10.9.0.7") {
		t.Fatalf("plan = %+v", p)
	}
	if st := r.state(); st.State != NetworkOK || st.Address != "10.9.0.7" {
		t.Fatalf("state = %+v", st)
	}
}

func TestClientStartFailureIsDownWithItsOwnCode(t *testing.T) {
	r := newNetRig(t, nil)
	r.builds.fail(errors.New("bind failed"))
	r.reconcile(t)
	st := r.state()
	if st.State != NetworkDown || st.Code != "download.no_client" {
		t.Fatalf("state = %+v", st)
	}
	r.builds.fail(nil)
	r.reconcile(t)
	if st := r.state(); st.State != NetworkOK {
		t.Fatalf("the monitor retries a client that failed to start: %+v", st)
	}
}

func TestProxyChecks(t *testing.T) {
	t.Run("unreachable", func(t *testing.T) {
		r := newNetRig(t, viaProxy)
		r.net.setProbe(errors.New("connection refused"))
		r.reconcile(t)
		if st := r.state(); st.State != NetworkDown || st.Code != "download.proxy_unreachable" || st.Mode != settings.NetworkProxy {
			t.Fatalf("state = %+v", st)
		}
		if r.builds.built() != 0 {
			t.Fatal("no client may be built while the proxy does not answer")
		}
		r.net.setProbe(nil)
		r.reconcile(t)
		st := r.state()
		if st.State != NetworkOK || st.Address != "127.0.0.1:1080" {
			t.Fatalf("state = %+v", st)
		}
		if got := r.net.probed; len(got) == 0 || got[0] != "127.0.0.1:1080" {
			t.Fatalf("probed %v", got)
		}
	})
	t.Run("password from the store", func(t *testing.T) {
		r := newNetRig(t, func(s *settings.Settings) { viaProxy(s); s.ProxyUsername = "user" })
		r.store.present, r.store.cred = true, account.Credential{Token: "s3cret", Username: "user"}
		r.reconcile(t)
		p := r.builds.lastPlan()
		if p.puser != "user" || p.ppass != "s3cret" || p.ptype != settings.ProxySOCKS5 || p.phost != "127.0.0.1" || p.pport != 1080 {
			t.Fatalf("plan = %+v", p)
		}
		if st := r.state(); strings.Contains(st.Address, "s3cret") || strings.Contains(st.Reason, "s3cret") {
			t.Fatalf("the password leaked into the state: %+v", st)
		}
	})
	t.Run("a user without a saved password", func(t *testing.T) {
		r := newNetRig(t, func(s *settings.Settings) { viaProxy(s); s.ProxyUsername = "user" })
		r.reconcile(t)
		if p := r.builds.lastPlan(); p.puser != "user" || p.ppass != "" {
			t.Fatalf("plan = %+v", p)
		}
	})
	t.Run("store failure is down and not an empty password", func(t *testing.T) {
		r := newNetRig(t, func(s *settings.Settings) { viaProxy(s); s.ProxyUsername = "user" })
		r.store.loadErr = errors.New("keychain locked")
		r.reconcile(t)
		if st := r.state(); st.State != NetworkDown || st.Code != "download.proxy_credentials_failed" {
			t.Fatalf("state = %+v", st)
		}
		if r.builds.built() != 0 {
			t.Fatal("a proxy that needs a login must not be used without one")
		}
	})
	t.Run("no login means no store", func(t *testing.T) {
		r := newNetRig(t, viaProxy)
		r.m.proxyStore = nil
		r.reconcile(t)
		if st := r.state(); st.State != NetworkOK {
			t.Fatalf("state = %+v", st)
		}
	})
	t.Run("login without a store", func(t *testing.T) {
		r := newNetRig(t, func(s *settings.Settings) { viaProxy(s); s.ProxyUsername = "user" })
		r.m.proxyStore = nil
		r.reconcile(t)
		if st := r.state(); st.State != NetworkDown || st.Code != "download.proxy_credentials_failed" {
			t.Fatalf("state = %+v", st)
		}
	})
}

func TestSettingsChangeRebuildsTheClientWithoutARestart(t *testing.T) {
	r := newNetRig(t, nil)
	unsubscribe := r.svc.Subscribe(r.m.applySettings)
	t.Cleanup(unsubscribe)
	r.m.mu.Lock()
	r.m.netKey = netKeyOf(r.svc.GetSettings())
	r.m.mu.Unlock()
	r.net.set(vpnIface("10.8.0.2"))
	r.reconcile(t)
	first := r.client()

	next := r.svc.GetSettings()
	next.Theme = "light"
	if err := r.svc.SaveSettings(next); err != nil {
		t.Fatal(err)
	}
	if len(r.m.netKick) != 0 {
		t.Fatal("an unrelated setting must not wake the network monitor")
	}

	viaInterface(&next)
	if err := r.svc.SaveSettings(next); err != nil {
		t.Fatal(err)
	}
	if len(r.m.netKick) != 1 {
		t.Fatal("a network setting must wake the network monitor")
	}
	r.reconcile(t)
	if r.client() == first || !clientClosed(first) {
		t.Fatal("the client was not rebuilt for the new mode")
	}
	if st := r.state(); st.State != NetworkOK || st.Mode != settings.NetworkInterface || st.Address != "10.8.0.2" {
		t.Fatalf("state = %+v", st)
	}

	viaProxy(&next)
	next.NetworkMode = settings.NetworkProxy
	if err := r.svc.SaveSettings(next); err != nil {
		t.Fatal(err)
	}
	r.reconcile(t)
	if p := r.builds.lastPlan(); p.mode != settings.NetworkProxy {
		t.Fatalf("plan = %+v", p)
	}

	next.NetworkMode = settings.NetworkDirect
	if err := r.svc.SaveSettings(next); err != nil {
		t.Fatal(err)
	}
	r.reconcile(t)
	if st := r.state(); st.State != NetworkOK || st.Mode != settings.NetworkDirect || st.Address != "" {
		t.Fatalf("state = %+v", st)
	}
}

func TestProxyPasswordRoundTrip(t *testing.T) {
	r := newNetRig(t, func(s *settings.Settings) { viaProxy(s); s.ProxyUsername = "user" })
	r.reconcile(t)
	first := r.client()

	if has, err := r.m.HasProxyPassword(); err != nil || has {
		t.Fatalf("HasProxyPassword = %v, %v before anything is saved", has, err)
	}
	if err := r.m.SetProxyPassword("user", "hunter2"); err != nil {
		t.Fatalf("SetProxyPassword: %v", err)
	}
	if has, err := r.m.HasProxyPassword(); err != nil || !has {
		t.Fatalf("HasProxyPassword = %v, %v after saving", has, err)
	}
	if r.store.cred.Token != "hunter2" || r.store.cred.Username != "user" {
		t.Fatalf("stored credential = %+v", r.store.cred)
	}
	if len(r.m.netKick) != 1 {
		t.Fatal("a new password must wake the monitor")
	}
	r.reconcile(t)
	if r.client() == first || r.builds.lastPlan().ppass != "hunter2" {
		t.Fatal("the client was not rebuilt with the new password")
	}

	if err := r.m.SetProxyPassword("user", ""); err != nil {
		t.Fatalf("clearing the password: %v", err)
	}
	if r.store.deleted != 1 {
		t.Fatal("an empty password removes the stored one")
	}
	r.reconcile(t)
	if r.builds.lastPlan().ppass != "" {
		t.Fatal("the client kept a cleared password")
	}

	if err := r.m.SetProxyPassword("user", strings.Repeat("x", 256)); !errors.Is(err, errProxyPasswordSize) {
		t.Fatalf("an oversized password: %v", err)
	}
	r.store.loadErr = errors.New("locked")
	if _, err := r.m.HasProxyPassword(); !errors.Is(err, errProxyCredentials) {
		t.Fatalf("HasProxyPassword on a failing store: %v", err)
	}
	r.m.proxyStore = nil
	if err := r.m.SetProxyPassword("user", "x"); !errors.Is(err, errProxyCredentials) {
		t.Fatalf("SetProxyPassword without a store: %v", err)
	}
}

func TestPasswordChangeOutsideProxyModeKeepsTheClient(t *testing.T) {
	r := newNetRig(t, nil)
	r.reconcile(t)
	first := r.client()
	if err := r.m.SetProxyPassword("", "hunter2"); err != nil {
		t.Fatal(err)
	}
	r.reconcile(t)
	if r.client() != first {
		t.Fatal("a proxy password has nothing to do with a direct client")
	}
}

func TestTestProxyUsesTheSavedSettings(t *testing.T) {
	r := newNetRig(t, nil)
	if err := r.m.TestProxy(t.Context()); !errors.Is(err, errProxyNotSet) {
		t.Fatalf("TestProxy with no proxy configured: %v", err)
	}
	srv := startSOCKS(t, &socksServer{user: "u", pass: "p"})
	host, port := hostPort(t, srv.addr())
	next := r.svc.GetSettings()
	next.ProxyHost = host
	next.ProxyPort = port
	next.ProxyUsername = "u"
	if err := r.svc.SaveSettings(next); err != nil {
		t.Fatal(err)
	}
	if err := r.m.TestProxy(t.Context()); !errors.Is(err, errProxyAuthFailed) {
		t.Fatalf("no saved password: %v, want errProxyAuthFailed", err)
	}
	if err := r.m.SetProxyPassword("u", "p"); err != nil {
		t.Fatal(err)
	}
	if err := r.m.TestProxy(t.Context()); err != nil {
		t.Fatalf("TestProxy with the right login: %v", err)
	}
	r.m.clearPasswordCache()
	r.store.loadErr = errors.New("locked")
	if err := r.m.TestProxy(t.Context()); !errors.Is(err, errProxyCredentials) {
		t.Fatalf("TestProxy with a failing store: %v", err)
	}
}

func TestListNetworkInterfaces(t *testing.T) {
	r := newNetRig(t, nil)
	r.net.set(
		ifaceInfo{Name: "Loopback", Loopback: true, Up: true},
		vpnIface("10.8.0.2"),
		ifaceInfo{Name: "Ethernet", Up: true, Addrs: []netip.Addr{netip.MustParseAddr("192.168.0.4")}},
	)
	got, err := r.m.ListNetworkInterfaces()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "vpn0" || !got[0].VPNLike || got[1].Name != "Ethernet" || got[1].VPNLike {
		t.Fatalf("interfaces = %+v", got)
	}
	r.net.mu.Lock()
	r.net.listErr = errors.New("denied")
	r.net.mu.Unlock()
	if _, err := r.m.ListNetworkInterfaces(); !errors.Is(err, errNetIfaceList) {
		t.Fatalf("listing failure: %v", err)
	}
}

func TestNetworkEventOnlyOnChange(t *testing.T) {
	r := newNetRig(t, viaProxy)
	log := recordEmits(t)
	r.net.setProbe(errors.New("connection refused"))
	for range 5 {
		r.reconcile(t)
	}
	if got := len(log.networks()); got != 1 {
		t.Fatalf("network events = %+v, want one for a route that stays down", log.networks())
	}
	r.net.setProbe(errors.New("i/o timeout"))
	r.reconcile(t)
	if got := len(log.networks()); got != 1 {
		t.Fatalf("a different wording of the same reason must not raise an event: %+v", log.networks())
	}
	if !strings.Contains(r.state().Reason, "i/o timeout") {
		t.Fatalf("the stored reason was not refreshed: %+v", r.state())
	}
	r.net.setProbe(nil)
	r.reconcile(t)
	if got := log.networks(); len(got) != 2 || got[1].State != NetworkOK {
		t.Fatalf("network events = %+v", got)
	}
}

func TestMetadataTimeoutThroughAProxyHasItsOwnCode(t *testing.T) {
	old := metadataTimeout
	metadataTimeout = 20 * time.Millisecond
	t.Cleanup(func() { metadataTimeout = old })
	const uri = "magnet:?xt=urn:btih:a748597437835a2fd0d2e06f8edd86fee316a84d"

	for _, c := range []struct {
		name  string
		proxy bool
		want  error
	}{
		{"direct", false, errNoMetadata},
		{"proxy", true, errNoMetadataProxy},
	} {
		t.Run(c.name, func(t *testing.T) {
			m := newTestManager(t, 1)
			m.client = offlineClient(t)
			m.client.httpTrackersOnly = c.proxy
			_, err := m.FetchMetadata(uri)
			if !errors.Is(err, c.want) {
				t.Fatalf("FetchMetadata = %v, want %v", err, c.want)
			}
			if got, want := uierr.Code(err), uierr.Code(c.want); got != want {
				t.Fatalf("code = %q, want %q", got, want)
			}
		})
	}
}

func TestFetchCancelledTellsAReplacedClientFromACancelledCall(t *testing.T) {
	m := newTestManager(t, 1)
	first := offlineClient(t)
	m.client = first
	if err := m.fetchCancelled(first); !errors.Is(err, errNoMetadata) {
		t.Fatalf("a cancelled call: %v", err)
	}
	m.client = offlineClient(t)
	if err := m.fetchCancelled(first); !errors.Is(err, errNetworkDown) {
		t.Fatalf("a replaced client: %v", err)
	}
	m.client = nil
	if err := m.fetchCancelled(first); !errors.Is(err, errNetworkDown) {
		t.Fatalf("a closed client: %v", err)
	}
}

func TestWorkOfAReplacedClientDoesNotTouchTheNewState(t *testing.T) {
	r := newNetRig(t, viaInterface)
	r.net.set(vpnIface("10.8.0.2"))
	r.reconcile(t)
	oldGen := func() uint64 { r.m.mu.Lock(); defer r.m.mu.Unlock(); return r.m.gen }()
	realDownload(t, r.m, "dl", StatusQueued, false)

	r.net.set(vpnIface("10.8.0.9"))
	r.reconcile(t)
	if newGen := func() uint64 { r.m.mu.Lock(); defer r.m.mu.Unlock(); return r.m.gen }(); newGen <= oldGen {
		t.Fatalf("generation %d did not move past %d", newGen, oldGen)
	}
	waitUntil(t, "the download to be restored on the new client", func() bool { return hasEngine(r.m, "dl") })

	t.Run("a restore job that finishes late drops its engine", func(t *testing.T) {
		stale := &fakeTorrent{size: 100}
		r.m.settleRestored(t.Context(), restoreJob{id: "dl", gen: oldGen}, stale, nil)
		if !stale.wasDropped() {
			t.Fatal("the engine of the old client was kept")
		}
		r.m.mu.Lock()
		current := r.m.engines["dl"]
		r.m.mu.Unlock()
		if current == nil || current == engineTorrent(stale) {
			t.Fatal("an engine of the old client replaced the one of the new client")
		}
	})

	t.Run("a write error from the old client fails nothing", func(t *testing.T) {
		r.m.onWriteError("dl", oldGen)(errors.New("disk full"))
		r.m.wg.Wait()
		if st := r.m.statusOf(t, "dl"); st == StatusFailed {
			t.Fatal("the download was failed by an engine that no longer exists")
		}
	})

	t.Run("a write error from the current client does", func(t *testing.T) {
		r.m.mu.Lock()
		gen := r.m.gen
		r.m.mu.Unlock()
		r.m.onWriteError("dl", gen)(errors.New("disk full"))
		waitUntil(t, "the write error to fail the download", func() bool { return r.m.statusOf(t, "dl") == StatusFailed })
	})
}

func TestTrustedRestoreSkipsTheRecheck(t *testing.T) {
	cases := []struct {
		name         string
		trusted      bool
		wantVerified bool
	}{
		{"after our own teardown", true, false},
		{"after a start", false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := newTestManager(t, 2)
			m.addTestItem("a", StatusQueued)
			eng := &fakeTorrent{size: 100}
			m.settleRestored(t.Context(), restoreJob{id: "a", trusted: c.trusted}, eng, nil)

			eng.mu.Lock()
			verified := eng.verified
			eng.mu.Unlock()
			if verified != c.wantVerified {
				t.Fatalf("verified = %v, want %v", verified, c.wantVerified)
			}
			m.mu.Lock()
			_, installed := m.engines["a"]
			m.mu.Unlock()
			if !installed {
				t.Fatal("the engine was not installed")
			}
			if st := m.statusOf(t, "a"); st != StatusDownloading {
				t.Fatalf("status = %s, want downloading: the queue starts the restored download", st)
			}
		})
	}
}

func TestTrustedPausedDownloadStaysPaused(t *testing.T) {
	m := newTestManager(t, 2)
	m.addTestItem("a", StatusPaused)
	eng := &fakeTorrent{size: 100}
	m.settleRestored(t.Context(), restoreJob{id: "a", paused: true, trusted: true}, eng, nil)
	if st := m.statusOf(t, "a"); st != StatusPaused {
		t.Fatalf("status = %s", st)
	}
	if eng.isUploading() {
		t.Fatal("a paused download must not upload")
	}
}

func realDownload(t *testing.T, m *Manager, id string, status Status, seeding bool) (*Download, *metainfo.MetaInfo) {
	t.Helper()
	mi, dataDir := makeSeedData(t, 512<<10)
	hash := mi.HashInfoBytes().HexString()
	if err := m.store.saveMetainfo(hash, mi); err != nil {
		t.Fatalf("save metainfo: %v", err)
	}
	d := m.addTestItem(id, status)
	m.mu.Lock()
	d.InfoHash = hash
	d.Destination = dataDir
	d.InPlace = true
	d.Seeding = seeding
	d.Files = []FileState{{Path: "payload.bin", Size: 512 << 10, Selected: true}}
	d.Total = 512 << 10
	m.mu.Unlock()
	return d, mi
}

func attachReal(t *testing.T, m *Manager, cl *client, id string, mi *metainfo.MetaInfo, dir string) {
	t.Helper()
	lt, err := cl.addMetainfo(mi, dir, storageOpts{inPlace: true})
	if err != nil {
		t.Fatalf("attach %s: %v", id, err)
	}
	m.mu.Lock()
	m.engines[id] = lt
	// The engine was settled on this client, as a restore or an add would have.
	m.markVerifiedLocked(id)
	m.mu.Unlock()
}

func hasEngine(m *Manager, id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.engines[id] != nil
}

func TestDownloadsComeBackWithoutARecheckAfterOurOwnTeardown(t *testing.T) {
	r := newNetRig(t, func(s *settings.Settings) { viaInterface(s); s.SeedAfterDownload = true })
	log := recordEmits(t)
	r.net.set(vpnIface("10.8.0.2"))
	r.reconcile(t)
	dl, dlMI := realDownload(t, r.m, "dl", StatusQueued, false)
	seed, seedMI := realDownload(t, r.m, "seed", StatusCompleted, true)
	paused, pausedMI := realDownload(t, r.m, "paused", StatusPaused, false)
	cl := r.client()
	attachReal(t, r.m, cl, "dl", dlMI, dl.Destination)
	attachReal(t, r.m, cl, "seed", seedMI, seed.Destination)
	attachReal(t, r.m, cl, "paused", pausedMI, paused.Destination)
	r.m.mu.Lock()
	r.m.engines["seed"].allowUpload()
	r.m.schedule()
	r.m.mu.Unlock()
	assertStatuses(t, r.m, map[string]Status{"dl": StatusDownloading, "paused": StatusPaused, "seed": StatusCompleted})

	r.net.set()
	r.reconcile(t)
	assertStatuses(t, r.m, map[string]Status{"dl": StatusQueued, "paused": StatusPaused, "seed": StatusCompleted})

	mark := log.mark()
	r.net.set(vpnIface("10.8.0.9"))
	r.reconcile(t)
	waitUntil(t, "the downloads to be restored", func() bool {
		return hasEngine(r.m, "dl") && hasEngine(r.m, "seed") && hasEngine(r.m, "paused")
	})
	waitUntil(t, "the queue to start the restored download", func() bool { return r.m.statusOf(t, "dl") == StatusDownloading })

	for _, id := range []string{"dl", "paused"} {
		for _, st := range log.statusesSince(id, mark) {
			if st == StatusVerifying {
				t.Fatalf("%s went through verifying: a client we closed ourselves is trusted, a full recheck of a large download at every reconnect is what this avoids", id)
			}
		}
	}
	assertStatuses(t, r.m, map[string]Status{"dl": StatusDownloading, "paused": StatusPaused, "seed": StatusCompleted})
	if d := mustGet(t, r.m, "seed"); !d.Seeding {
		t.Fatal("the seeding download was not restored to seed")
	}
	hashes := map[string]bool{}
	for _, tor := range r.client().cl.Torrents() {
		hashes[tor.InfoHash().HexString()] = true
	}
	for _, d := range []*Download{dl, seed, paused} {
		if !hashes[d.InfoHash] {
			t.Errorf("%s is not on the new client", d.ID)
		}
	}
	r.m.mu.Lock()
	uploading := r.m.engines["paused"]
	r.m.mu.Unlock()
	if uploading == nil {
		t.Fatal("no engine for the paused download")
	}
}

func TestDownloadsRestoredAfterAStartAreRechecked(t *testing.T) {
	r := newNetRig(t, viaInterface)
	log := recordEmits(t)
	r.reconcile(t)
	r.m.mu.Lock()
	r.m.resume = &resumeSet{all: true}
	r.m.mu.Unlock()
	realDownload(t, r.m, "dl", StatusQueued, false)

	r.net.set(vpnIface("10.8.0.2"))
	r.reconcile(t)
	waitUntil(t, "the download to be restored", func() bool { return hasEngine(r.m, "dl") })
	waitUntil(t, "the recheck to finish", func() bool {
		return r.m.statusOf(t, "dl") == StatusDownloading
	})
	seen := false
	for _, st := range log.statusesOf("dl") {
		if st == StatusVerifying {
			seen = true
		}
	}
	if !seen {
		t.Fatal("nothing was checked in this process yet: the download has to be rechecked like after any start")
	}
}

func TestTeardownWithNothingRunningIsHarmless(t *testing.T) {
	r := newNetRig(t, nil)
	r.m.teardownClient()
	r.reconcile(t)
	r.m.teardownClient()
	r.m.teardownClient()
	if r.client() != nil {
		t.Fatal("client left after a teardown")
	}
}

func TestSettingsChangeRacesTheMonitor(t *testing.T) {
	r := newNetRig(t, nil)
	recordEmits(t)
	r.net.set(vpnIface("10.8.0.2"))
	r.m.netInterval = time.Millisecond
	if err := r.m.ServiceStartup(t.Context(), application.ServiceOptions{}); err != nil {
		t.Fatalf("startup: %v", err)
	}
	r.m.addTestDownload("dl")
	shutdown := sync.OnceFunc(func() {
		if err := r.m.ServiceShutdown(); err != nil {
			t.Errorf("shutdown: %v", err)
		}
	})
	t.Cleanup(shutdown)

	modes := []func(*settings.Settings){
		func(s *settings.Settings) { s.NetworkMode = settings.NetworkDirect },
		viaInterface,
		viaProxy,
	}
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		for i := range 60 {
			if i%2 == 0 {
				r.net.set(vpnIface("10.8.0.2"))
			} else {
				r.net.set()
			}
			r.net.setProbe(map[bool]error{true: nil, false: errors.New("down")}[i%3 != 0])
		}
	}()
	go func() {
		defer wg.Done()
		for i := range 60 {
			next := r.svc.GetSettings()
			modes[i%len(modes)](&next)
			if err := r.svc.SaveSettings(next); err != nil {
				t.Errorf("SaveSettings: %v", err)
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		for range 200 {
			r.m.NetworkStatus()
			r.m.List()
			r.m.reconcileNetwork(t.Context())
		}
	}()
	wg.Wait()

	final := r.svc.GetSettings()
	final.NetworkMode = settings.NetworkInterface
	final.NetworkInterface = "vpn0"
	r.net.set(vpnIface("10.8.0.2"))
	if err := r.svc.SaveSettings(final); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "the last settings to be applied", func() bool {
		st := r.m.NetworkStatus()
		r.m.mu.Lock()
		defer r.m.mu.Unlock()
		return st.State == NetworkOK && st.Mode == settings.NetworkInterface && r.m.client != nil && r.m.netActive != nil &&
			r.m.netActive.mode == settings.NetworkInterface
	})
	shutdown()

	r.builds.mu.Lock()
	defer r.builds.mu.Unlock()
	open := 0
	for _, c := range r.builds.clients {
		if !clientClosed(c) {
			open++
		}
	}
	if open != 0 {
		t.Fatalf("%d of %d clients were left open", open, len(r.builds.clients))
	}
}

func TestTeardownEndsAPendingMetadataFetch(t *testing.T) {
	r := newNetRig(t, nil)
	r.reconcile(t)
	cl := r.client()

	result := make(chan error, 1)
	go func() {
		_, err := r.m.FetchMetadata("magnet:?xt=urn:btih:a748597437835a2fd0d2e06f8edd86fee316a84d")
		result <- err
	}()
	waitUntil(t, "the fetch to add its torrent", func() bool { return len(cl.cl.Torrents()) == 1 })

	r.m.teardownClient()

	select {
	case err := <-result:
		if !errors.Is(err, errNetworkDown) {
			t.Fatalf("FetchMetadata = %v, want errNetworkDown: the client it ran on is gone", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the metadata fetch outlived its client")
	}
	if !clientClosed(cl) {
		t.Fatal("client not closed")
	}
	r.m.mu.Lock()
	leftover := len(r.m.reserved) + len(r.m.fetching) + len(r.m.pending)
	r.m.mu.Unlock()
	if leftover != 0 {
		t.Fatalf("the fetch left %d entries behind", leftover)
	}
}

func TestRequestsWhileTheClientIsBeingReplacedReportTheNetwork(t *testing.T) {
	r := newNetRig(t, viaInterface)
	r.net.set(vpnIface("10.8.0.2"))
	r.reconcile(t)
	dir := t.TempDir()
	r.m.addTestItem("paused", StatusPaused)
	old := metadataTimeout
	metadataTimeout = 20 * time.Millisecond
	t.Cleanup(func() { metadataTimeout = old })

	hold, release := make(chan struct{}), make(chan struct{})
	r.builds.mu.Lock()
	r.builds.hold, r.builds.release = hold, release
	r.builds.mu.Unlock()
	r.net.set(vpnIface("10.9.0.7"))
	done := make(chan struct{})
	go func() {
		defer close(done)
		r.reconcile(t)
	}()
	<-hold

	if _, err := r.m.FetchMetadata("magnet:?xt=urn:btih:a748597437835a2fd0d2e06f8edd86fee316a84d"); !errors.Is(err, errNetworkDown) {
		t.Errorf("FetchMetadata while the client is replaced: %v, want errNetworkDown", err)
	}
	if _, err := r.m.StartDownloadFrom("a748597437835a2fd0d2e06f8edd86fee316a84d", dir, nil, Origin{}); !errors.Is(err, errNetworkDown) {
		t.Errorf("StartDownloadFrom while the client is replaced: %v, want errNetworkDown", err)
	}
	if err := r.m.Resume("paused"); err != nil {
		t.Errorf("Resume while the client is replaced: %v", err)
	}
	if st := r.state(); st.State != NetworkOK {
		t.Errorf("a replacement is not an outage: %+v", st)
	}

	close(release)
	<-done
	if _, err := r.m.FetchMetadata("magnet:?xt=urn:btih:a748597437835a2fd0d2e06f8edd86fee316a84d&x.pe=203.0.113.9:1"); errors.Is(err, errNetworkDown) || errors.Is(err, errNoClient) {
		t.Errorf("FetchMetadata after the replacement: %v", err)
	}
}

func TestShutdownDoesNotSitOutARestoreOnTheReplacementClient(t *testing.T) {
	dir := t.TempDir()
	first := mustManagerAt(t, dir)
	d := first.addTestItem("m", StatusQueued)
	first.mu.Lock()
	d.Source = "magnet:?xt=urn:btih:a748597437835a2fd0d2e06f8edd86fee316a84d"
	d.InfoHash = "a748597437835a2fd0d2e06f8edd86fee316a84d"
	if err := first.persistLocked(); err != nil {
		first.mu.Unlock()
		t.Fatal(err)
	}
	first.mu.Unlock()
	if err := first.ServiceShutdown(); err != nil {
		t.Fatal(err)
	}

	cfg := settings.Defaults()
	viaInterface(&cfg)
	svc, err := settings.NewServiceAt(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.SaveSettings(cfg); err != nil {
		t.Fatal(err)
	}
	m, err := newManagerAt(dir, svc)
	if err != nil {
		t.Fatal(err)
	}
	fake, builds := &fakeNetwork{}, &buildLog{}
	m.netEnv = netEnv{interfaces: fake.interfaces, probe: fake.probe, dns: fake.dnsOf, hostCheck: fake.hostCheck}
	m.buildClient = builds.build
	m.netInterval = time.Millisecond
	if err := m.ServiceStartup(t.Context(), application.ServiceOptions{}); err != nil {
		t.Fatalf("startup: %v", err)
	}
	fake.set(vpnIface("10.8.0.2"))
	// The magnet has no peers to ask, so the restore waits for metadata for as
	// long as metadataTimeout, on the client the monitor just built.
	waitUntil(t, "the restore to start waiting for metadata", func() bool { return m.statusOf(t, "m") == StatusMetadata })

	done := make(chan error, 1)
	go func() { done <- m.ServiceShutdown() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("shutdown: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("ServiceShutdown is still waiting for a restore that belongs to the replacement client")
	}
}

func TestTeardownTrustsOnlyDownloadsSettledOnThisClient(t *testing.T) {
	r := newNetRig(t, viaInterface)
	r.net.set(vpnIface("10.8.0.2"))
	r.reconcile(t)
	add := func(id string, status Status, seeding, settled bool) {
		d := r.m.addTestItem(id, status)
		r.m.mu.Lock()
		d.Seeding = seeding
		r.m.engines[id] = &fakeTorrent{size: 100}
		if settled {
			r.m.markVerifiedLocked(id)
		}
		r.m.mu.Unlock()
	}
	add("settled", StatusDownloading, false, true)
	add("never restored", StatusDownloading, false, false)
	add("checking", StatusVerifying, false, true)
	add("metadata", StatusMetadata, false, true)
	add("paused", StatusPaused, false, true)
	add("seed", StatusCompleted, true, true)
	r.m.addTestItem("queued without engine", StatusQueued)

	r.net.set()
	r.reconcile(t)

	r.m.mu.Lock()
	ids := r.m.resume.ids
	afterwards := len(r.m.verified)
	r.m.mu.Unlock()
	want := map[string]bool{
		"settled": true, "never restored": false, "checking": false, "metadata": false,
		"paused": true, "seed": true, "queued without engine": false,
	}
	for id, trusted := range want {
		e, ok := ids[id]
		if !ok || e.trusted != trusted {
			t.Errorf("%s: entry %+v (listed %v), want trusted=%v", id, e, ok, trusted)
		}
	}
	if afterwards != 0 {
		t.Fatalf("%d downloads still marked settled on a client that is gone", afterwards)
	}
}

func TestUnsettledDownloadsAreRecheckedAfterTheNetworkFlaps(t *testing.T) {
	r := newNetRig(t, viaInterface)
	log := recordEmits(t)
	r.net.set(vpnIface("10.8.0.2"))
	r.reconcile(t)
	// A start whose restore had not reached these yet, or whose recheck the
	// flap interrupted: no engine settled in this process for either.
	realDownload(t, r.m, "not reached", StatusQueued, false)
	realDownload(t, r.m, "was checking", StatusVerifying, false)

	r.net.set()
	r.reconcile(t)
	mark := log.mark()
	r.net.set(vpnIface("10.8.0.9"))
	r.reconcile(t)
	waitUntil(t, "both to be restored", func() bool { return hasEngine(r.m, "not reached") && hasEngine(r.m, "was checking") })
	waitUntil(t, "the rechecks to finish", func() bool {
		return r.m.statusOf(t, "not reached") != StatusVerifying && r.m.statusOf(t, "was checking") != StatusVerifying
	})
	for _, id := range []string{"not reached", "was checking"} {
		seen := false
		for _, st := range log.statusesSince(id, mark) {
			if st == StatusVerifying {
				seen = true
			}
		}
		if !seen {
			t.Errorf("%s came back without a recheck although this process never checked it", id)
		}
	}
}

func TestForceStartWhileDownKeepsTheForce(t *testing.T) {
	r := newNetRig(t, viaInterface)
	r.reconcile(t)
	r.m.addTestItem("forced", StatusFailed)
	r.m.addTestItem("plain", StatusPaused)
	if err := r.m.ForceStart("forced"); err != nil {
		t.Fatal(err)
	}
	if err := r.m.Resume("plain"); err != nil {
		t.Fatal(err)
	}
	for _, all := range []bool{false, true} {
		r.m.mu.Lock()
		r.m.resume.all = all
		snapshotOf := *r.m.resume
		r.m.resume = &snapshotOf
		jobs := r.m.resumeJobsLocked()
		r.m.resume = &snapshotOf
		r.m.mu.Unlock()
		byID := map[string]restoreJob{}
		for _, j := range jobs {
			byID[j.id] = j
		}
		if all {
			if !byID["forced"].force || byID["plain"].force || byID["forced"].trusted {
				t.Fatalf("all=%v: jobs = %+v", all, byID)
			}
			continue
		}
		// A failed download is left out of a partial resume, unless it was
		// asked for by name: ForceStart moved it to the queue.
		if !byID["forced"].force || byID["plain"].force {
			t.Fatalf("all=%v: jobs = %+v", all, byID)
		}
	}
}

func TestMetainfoForEndsWhenTheClientIsClosedUnderIt(t *testing.T) {
	m := newTestManager(t, 1)
	cl := offlineClient(t)
	result := make(chan error, 1)
	go func() {
		_, err := m.metainfoFor(t.Context(), cl, "magnet:?xt=urn:btih:a748597437835a2fd0d2e06f8edd86fee316a84d", "")
		result <- err
	}()
	waitUntil(t, "the torrent to be added", func() bool { return len(cl.cl.Torrents()) == 1 })
	cl.close()
	select {
	case err := <-result:
		if !errors.Is(err, errNetworkDown) {
			t.Fatalf("metainfoFor = %v, want errNetworkDown at once and not after the metadata timeout", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("metainfoFor sat out the metadata timeout on a closed client")
	}
}

func TestProxyPasswordIsReadOnceAndBelongsToItsUser(t *testing.T) {
	r := newNetRig(t, func(s *settings.Settings) { viaProxy(s); s.ProxyUsername = "user" })
	unsubscribe := r.svc.Subscribe(r.m.applySettings)
	t.Cleanup(unsubscribe)
	r.m.mu.Lock()
	r.m.netKey = netKeyOf(r.svc.GetSettings())
	r.m.mu.Unlock()
	if err := r.m.SetProxyPassword("user", "s3cret"); err != nil {
		t.Fatal(err)
	}
	loads := func() int { r.store.mu.Lock(); defer r.store.mu.Unlock(); return r.store.loads }
	base := loads()

	for range 6 {
		r.reconcile(t)
	}
	if got := loads() - base; got != 1 {
		t.Fatalf("the credential store was read %d times over 6 checks, want once: the monitor must not poll a keychain", got)
	}
	if r.builds.lastPlan().ppass != "s3cret" {
		t.Fatalf("plan = %+v", r.builds.lastPlan())
	}

	// The user name changes: the saved password belongs to the old one.
	next := r.svc.GetSettings()
	next.ProxyUsername = "someone"
	if err := r.svc.SaveSettings(next); err != nil {
		t.Fatal(err)
	}
	r.reconcile(t)
	st := r.state()
	if st.State != NetworkDown || st.Code != "download.proxy_credentials_mismatch" {
		t.Fatalf("state = %+v", st)
	}
	if has, err := r.m.HasProxyPassword(); err != nil || has {
		t.Fatalf("HasProxyPassword = %v, %v for another user name", has, err)
	}
	if err := r.m.TestProxy(t.Context()); !errors.Is(err, errProxyMismatch) {
		t.Fatalf("TestProxy = %v, want errProxyMismatch", err)
	}
	built := r.builds.built()
	r.reconcile(t)
	if r.builds.built() != built {
		t.Fatal("a client was built with a password that is not this user's")
	}

	// Entering the password again for the new name fixes it.
	if err := r.m.SetProxyPassword("someone", "other"); err != nil {
		t.Fatal(err)
	}
	r.reconcile(t)
	if st := r.state(); st.State != NetworkOK || r.builds.lastPlan().ppass != "other" || r.builds.lastPlan().puser != "someone" {
		t.Fatalf("state = %+v, plan = %+v", st, r.builds.lastPlan())
	}
}

func (r *netRig) followSettings(t *testing.T) {
	t.Helper()
	unsubscribe := r.svc.Subscribe(r.m.applySettings)
	t.Cleanup(unsubscribe)
	r.m.mu.Lock()
	r.m.netKey = netKeyOf(r.svc.GetSettings())
	r.m.mu.Unlock()
}

func (r *netRig) saveProxyUser(t *testing.T, user string) {
	t.Helper()
	next := r.svc.GetSettings()
	next.ProxyUsername = user
	if err := r.svc.SaveSettings(next); err != nil {
		t.Fatalf("save settings: %v", err)
	}
}

// TestProxyPasswordWithANewUserNameFollowsTheOrderOfTheWindow is what one press
// of Apply does on a clean install: the password goes in first, together with
// the name typed next to it, and the settings with that name come second.
func TestProxyPasswordWithANewUserNameFollowsTheOrderOfTheWindow(t *testing.T) {
	r := newNetRig(t, nil)
	r.followSettings(t)
	r.reconcile(t)

	if err := r.m.SetProxyPassword("bob", "hunter2"); err != nil {
		t.Fatalf("SetProxyPassword: %v", err)
	}
	next := r.svc.GetSettings()
	viaProxy(&next)
	next.ProxyUsername = "bob"
	if err := r.svc.SaveSettings(next); err != nil {
		t.Fatal(err)
	}
	r.reconcile(t)

	st := r.state()
	if st.State != NetworkOK || st.Mode != settings.NetworkProxy {
		t.Fatalf("state = %+v, want the proxy client up on the first Apply", st)
	}
	if p := r.builds.lastPlan(); p.puser != "bob" || p.ppass != "hunter2" {
		t.Fatalf("plan = %+v", p)
	}
	if has, err := r.m.HasProxyPassword(); err != nil || !has {
		t.Fatalf("HasProxyPassword = %v, %v after Apply", has, err)
	}
}

// TestProxyPasswordForAnotherUserNameLeavesTheRunningClientAlone covers the
// monitor tick that lands between the two calls of Apply: the proxy that
// works for the old name must not be taken down for a password that belongs
// to a name the settings do not carry yet.
func TestProxyPasswordForAnotherUserNameLeavesTheRunningClientAlone(t *testing.T) {
	r := newNetRig(t, func(s *settings.Settings) { viaProxy(s); s.ProxyUsername = "alice" })
	r.followSettings(t)
	r.store.present, r.store.cred = true, account.Credential{Token: "old", Username: "alice"}
	r.reconcile(t)
	first := r.client()
	if first == nil {
		t.Fatalf("no client to start from: %+v", r.state())
	}
	built := r.builds.built()

	if err := r.m.SetProxyPassword("bob", "new"); err != nil {
		t.Fatal(err)
	}
	r.reconcile(t)
	if st := r.state(); st.State != NetworkOK || r.client() != first || clientClosed(first) || r.builds.built() != built {
		t.Fatalf("state = %+v: a password for a name the settings do not carry yet took the working client down", st)
	}

	r.saveProxyUser(t, "bob")
	r.reconcile(t)
	st := r.state()
	if st.State != NetworkOK {
		t.Fatalf("state = %+v after the settings caught up", st)
	}
	if p := r.builds.lastPlan(); p.puser != "bob" || p.ppass != "new" {
		t.Fatalf("plan = %+v", p)
	}
}

// TestProxyPasswordAfterTheNewUserNameRecoversInOneCheck is the other order:
// the settings with the new name land first, the old password does not belong
// to it, and the password that follows has to bring the client back at once.
func TestProxyPasswordAfterTheNewUserNameRecoversInOneCheck(t *testing.T) {
	r := newNetRig(t, func(s *settings.Settings) { viaProxy(s); s.ProxyUsername = "alice" })
	r.followSettings(t)
	r.store.present, r.store.cred = true, account.Credential{Token: "old", Username: "alice"}
	r.reconcile(t)

	r.saveProxyUser(t, "bob")
	r.reconcile(t)
	if st := r.state(); st.State != NetworkDown || st.Code != "download.proxy_credentials_mismatch" {
		t.Fatalf("state = %+v, want the mismatch while there is no password for bob", st)
	}

	if err := r.m.SetProxyPassword("bob", "new"); err != nil {
		t.Fatal(err)
	}
	if len(r.m.netKick) != 1 {
		t.Fatal("the password that ends a mismatch must wake the monitor")
	}
	r.reconcile(t)
	if st := r.state(); st.State != NetworkOK {
		t.Fatalf("state = %+v, want the client back after one check", st)
	}
	if p := r.builds.lastPlan(); p.puser != "bob" || p.ppass != "new" {
		t.Fatalf("plan = %+v", p)
	}
}

// TestPasswordReadBeforeASaveDoesNotOutliveIt: the monitor reads the store
// while the window saves a new password. What the monitor read is the old one,
// and it must not stay in the cache once the save has cleared it.
func TestPasswordReadBeforeASaveDoesNotOutliveIt(t *testing.T) {
	r := newNetRig(t, func(s *settings.Settings) { viaProxy(s); s.ProxyUsername = "user" })
	r.store.present, r.store.cred = true, account.Credential{Token: "old", Username: "user"}
	entered := make(chan struct{}, 1)
	hold := make(chan struct{})
	r.store.mu.Lock()
	r.store.loadEntered, r.store.loadHold = entered, hold
	r.store.mu.Unlock()

	read := make(chan string, 1)
	go func() {
		pass, err := r.m.proxyPassword("user")
		if err != nil {
			pass = "error: " + err.Error()
		}
		read <- pass
	}()
	<-entered
	r.store.mu.Lock()
	r.store.loadHold = nil
	r.store.mu.Unlock()

	if err := r.m.SetProxyPassword("user", "new"); err != nil {
		t.Fatal(err)
	}
	close(hold)
	if got := <-read; got != "old" {
		t.Fatalf("the interrupted read returned %q", got)
	}
	pass, err := r.m.proxyPassword("user")
	if err != nil || pass != "new" {
		t.Fatalf("password after the save = %q, %v, want the saved one and not the stale read", pass, err)
	}
}
