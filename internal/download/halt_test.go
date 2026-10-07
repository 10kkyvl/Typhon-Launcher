package download

import (
	"errors"
	"testing"
	"time"

	"typhon/internal/settings"
)

// A client that is going away must stop carrying data before anything slow
// happens to it: the write of the parked downloads and the wait for the jobs
// that still run on it. These tests pin the order, not the end state, which
// TestLostInterfaceParksDownloadsInsteadOfFailingThem already covers.
func TestTeardownCutsTheClientOffBeforeItPersists(t *testing.T) {
	r := newNetRig(t, viaInterface)
	r.net.set(vpnIface("10.8.0.2"))
	r.reconcile(t)
	old := r.client()
	r.m.addTestDownload("dl")

	var parked, cutWhenParked bool
	original := emit
	emit = func(name string, data any) {
		if d, ok := data.(Download); ok && name == eventUpdated && d.ID == "dl" && d.Status == StatusQueued {
			parked, cutWhenParked = true, old.halted()
		}
	}
	t.Cleanup(func() { emit = original })

	r.net.set()
	r.reconcile(t)

	if !parked {
		t.Fatal("the download was never parked: the test saw nothing of the persist step")
	}
	if !cutWhenParked {
		t.Fatal("the client still carried data when the parked downloads were about to be written")
	}
}

func TestTeardownCutsTheClientOffBeforeItWaitsForJobs(t *testing.T) {
	r := newNetRig(t, viaInterface)
	r.net.set(vpnIface("10.8.0.2"))
	r.reconcile(t)
	old := r.client()
	r.m.addTestItem("dl", StatusVerifying)

	jobCtx, ok := r.m.beginJob(t.Context(), "dl")
	if !ok {
		t.Fatal("beginJob")
	}
	release := make(chan struct{})
	jobEnded := make(chan struct{})
	go func() {
		<-jobCtx.Done()
		// A recheck that is slow to notice it was cancelled.
		<-release
		r.m.endJob("dl")
		close(jobEnded)
	}()
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
		<-jobEnded
	})

	r.net.set()
	torn := make(chan struct{})
	go func() {
		r.reconcile(t)
		close(torn)
	}()

	// The job is cancelled after the persist and right before the wait, so
	// once it is, the teardown is sitting in the wait.
	<-jobCtx.Done()
	if !old.halted() {
		t.Fatal("the client still carried data while the teardown waited for a job to end")
	}
	select {
	case <-torn:
		t.Fatal("the teardown did not wait for the job")
	default:
	}
	close(release)
	<-torn
	if !clientClosed(old) {
		t.Fatal("the old client was not closed in the end")
	}
}

func TestHaltedClientTakesNoNewTorrents(t *testing.T) {
	cl := offlineClient(t)
	cl.halt()
	if !cl.halted() {
		t.Fatal("halt did not mark the client")
	}
	_, err := cl.addMagnet("magnet:?xt=urn:btih:a748597437835a2fd0d2e06f8edd86fee316a84d&dn=x", cl.metaDir, storageOpts{})
	if !errors.Is(err, errNetworkDown) {
		t.Fatalf("add on a halted client = %v, want errNetworkDown", err)
	}
	if n := len(cl.cl.Torrents()); n != 0 {
		t.Fatalf("%d torrents on a halted client", n)
	}
}

// TestHaltedClientCarriesNoDataEvenWhenAnEngineIsAllowedAgain is the real
// thing over loopback: after the cut, the scheduler or a job that lands late
// may still flip the gates of an engine back on, and nothing may cross. The
// control for a seeder that is not halted is TestSeedGateControlUploadsWhenAllowed.
func TestHaltedClientCarriesNoDataEvenWhenAnEngineIsAllowedAgain(t *testing.T) {
	mi, dataDir := makeSeedData(t, 1<<20)
	seedCl := offlineClient(t)
	seed, err := seedCl.addMetainfo(mi, dataDir, storageOpts{inPlace: true})
	if err != nil {
		t.Fatalf("add seeder torrent: %v", err)
	}
	t.Cleanup(seed.drop)
	<-seed.t.GotInfo()
	total := seed.t.Length()
	select {
	case <-seed.t.Complete().On():
	case <-time.After(30 * time.Second):
		t.Fatalf("seeder holds %d/%d bytes, cannot test the cut", seed.t.BytesCompleted(), total)
	}

	seedCl.halt()
	seed.allowDownload()
	seed.allowUpload()

	leechCl := offlineClient(t)
	leech, err := leechCl.addMetainfo(mi, t.TempDir(), storageOpts{})
	if err != nil {
		t.Fatalf("add leecher torrent: %v", err)
	}
	t.Cleanup(leech.drop)
	<-leech.t.GotInfo()
	leech.allowDownload()
	leech.setPriorities([]bool{true})
	if n := leech.t.AddClientPeer(seedCl.cl); n == 0 {
		t.Fatal("leecher could not be pointed at the seeder")
	}

	select {
	case <-leech.t.Complete().On():
	case <-time.After(3 * time.Second):
	}
	if got := leech.t.BytesCompleted(); got != 0 {
		t.Fatalf("leecher received %d/%d bytes from a halted client", got, total)
	}
	if sent := seedStats(seed).BytesWrittenData.Int64(); sent != 0 {
		t.Fatalf("a halted client sent %d bytes", sent)
	}
}

// A new route is asked for: the client of the old one must not keep working
// through the check of the new one, which can take as long as the proxy takes
// to time out.
func TestSettingsChangeTakesTheOldClientDownBeforeTheNewRouteIsProbed(t *testing.T) {
	r := newNetRig(t, viaProxy)
	r.reconcile(t)
	first := r.client()
	if first == nil {
		t.Fatalf("no client to start from: %+v", r.state())
	}

	entered, release := r.net.holdProbe()
	t.Cleanup(release)
	next := r.svc.GetSettings()
	next.ProxyPort = 1081
	if err := r.svc.SaveSettings(next); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		r.reconcile(t)
		close(done)
	}()
	<-entered

	if !clientClosed(first) || r.client() != nil {
		t.Fatal("the client of the old proxy is still up while the new one is probed")
	}
	r.m.mu.Lock()
	offline := r.m.offlineLocked()
	r.m.mu.Unlock()
	if !offline {
		t.Fatal("requests during the probe must be told the network is down, not that there is no client")
	}
	release()
	<-done

	st := r.state()
	if st.State != NetworkOK || st.Address != "127.0.0.1:1081" {
		t.Fatalf("state = %+v", st)
	}
	if p := r.builds.lastPlan(); p.pport != 1081 {
		t.Fatalf("plan = %+v", p)
	}
}

func TestSlowProbeOfAnUnchangedRouteKeepsTheClient(t *testing.T) {
	r := newNetRig(t, viaProxy)
	r.reconcile(t)
	first := r.client()

	entered, release := r.net.holdProbe()
	t.Cleanup(release)
	done := make(chan struct{})
	go func() {
		r.reconcile(t)
		close(done)
	}()
	<-entered
	if clientClosed(first) || r.client() != first {
		t.Fatal("a probe of the same proxy that is merely slow took the client down")
	}
	release()
	<-done
	if r.client() != first || r.builds.built() != 1 {
		t.Fatal("an unchanged route rebuilt the client")
	}
}

func TestPlanServesSettings(t *testing.T) {
	proxy := netPlan{mode: settings.NetworkProxy, ptype: settings.ProxySOCKS5, phost: "127.0.0.1", pport: 1080, puser: "u", ppass: "p"}
	iface := netPlan{mode: settings.NetworkInterface, iface: "vpn0", index: 7}
	cases := []struct {
		name   string
		plan   netPlan
		change func(*settings.Settings)
		want   bool
	}{
		{"direct stays direct", netPlan{mode: settings.NetworkDirect}, func(s *settings.Settings) {}, true},
		{"direct to proxy", netPlan{mode: settings.NetworkDirect}, func(s *settings.Settings) { s.NetworkMode = settings.NetworkProxy }, false},
		{"proxy unchanged", proxy, func(s *settings.Settings) {}, true},
		{"proxy host", proxy, func(s *settings.Settings) { s.ProxyHost = "10.0.0.1" }, false},
		{"proxy port", proxy, func(s *settings.Settings) { s.ProxyPort = 1 }, false},
		{"proxy type", proxy, func(s *settings.Settings) { s.ProxyType = settings.ProxyHTTP }, false},
		{"proxy user", proxy, func(s *settings.Settings) { s.ProxyUsername = "other" }, false},
		{"proxy to interface", proxy, func(s *settings.Settings) { s.NetworkMode = settings.NetworkInterface }, false},
		{"interface unchanged", iface, func(s *settings.Settings) {}, true},
		{"another interface", iface, func(s *settings.Settings) { s.NetworkInterface = "tun1" }, false},
		{"the same interface in another case", iface, func(s *settings.Settings) { s.NetworkInterface = "VPN0" }, true},
		{"interface to direct", iface, func(s *settings.Settings) { s.NetworkMode = settings.NetworkDirect }, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := settings.Defaults()
			cfg.NetworkMode = c.plan.mode
			cfg.NetworkInterface = c.plan.iface
			cfg.ProxyType, cfg.ProxyHost, cfg.ProxyPort, cfg.ProxyUsername = c.plan.ptype, c.plan.phost, c.plan.pport, c.plan.puser
			c.change(&cfg)
			if got := c.plan.servesSettings(cfg); got != c.want {
				t.Fatalf("servesSettings = %v, want %v", got, c.want)
			}
		})
	}
}

// A restore that is in flight when the client is cut off finds the client
// refusing work. That is the teardown talking, not a failure of the download:
// it must come back with the next client, not turn into a failed one.
func TestRestoreRefusedByAClientThatIsGoingAwayDoesNotFailTheDownload(t *testing.T) {
	const hash = "a748597437835a2fd0d2e06f8edd86fee316a84d"
	r := newNetRig(t, nil)
	r.reconcile(t)
	cl := r.client()
	d := r.m.addTestItem("dl", StatusQueued)
	r.m.mu.Lock()
	d.Source = "magnet:?xt=urn:btih:" + hash + "&dn=x"
	d.InfoHash = hash
	gen := r.m.gen
	r.m.mu.Unlock()

	var failed bool
	original := emit
	emit = func(name string, data any) {
		if name == eventFailed {
			failed = true
		}
		// setStatus announces the metadata step with the lock held, which is the
		// moment the teardown of the test's story replaces the client.
		if dd, ok := data.(Download); ok && name == eventUpdated && dd.ID == "dl" && dd.Status == StatusMetadata {
			r.m.gen++
		}
	}
	t.Cleanup(func() { emit = original })

	cl.halt()
	r.m.restoreOne(t.Context(), cl, restoreJob{id: "dl", infoHash: hash, source: d.Source, dest: t.TempDir(), gen: gen})

	if failed || r.m.statusOf(t, "dl") == StatusFailed {
		t.Fatalf("a restore refused by a halted client failed the download: %+v", mustGet(t, r.m, "dl"))
	}
}
