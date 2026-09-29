package download

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"time"

	"typhon/internal/account"
	"typhon/internal/settings"

	"github.com/anacrolix/torrent/storage"
)

const (
	proxyStoreName = "Typhon Launcher Proxy"

	netPollInterval     = 5 * time.Second
	proxyProbeLimit     = 6 * time.Second
	startupNetworkLimit = 3 * time.Second
	maxProxyPassLen     = 255
)

type netKey struct {
	mode  string
	iface string
	ptype string
	phost string
	pport int
	puser string
}

func netKeyOf(cfg settings.Settings) netKey {
	return netKey{
		mode:  cfg.NetworkMode,
		iface: cfg.NetworkInterface,
		ptype: cfg.ProxyType,
		phost: cfg.ProxyHost,
		pport: cfg.ProxyPort,
		puser: cfg.ProxyUsername,
	}
}

type netEnv struct {
	interfaces func() ([]ifaceInfo, error)
	probe      func(ctx context.Context, addr string) error
	// dns lists the name servers of one adapter and hostCheck says whether a
	// socket bound to its addresses is held to that adapter. Both are asked
	// for the chosen adapter only, on every check.
	dns       func(ifaceInfo) ([]netip.Addr, error)
	hostCheck func(ifc ifaceInfo, v4, v6 bool) error
}

func systemNetEnv() netEnv {
	return netEnv{interfaces: systemInterfaces, probe: probeTCP, dns: adapterDNS, hostCheck: systemHostCheck}
}

type clientBuilder func(ctx context.Context, cfg settings.Settings, metaDir string, completion storage.PieceCompletion, plan netPlan) (*client, error)

// resumeEntry says how one download comes back: trusted skips the full
// recheck, force starts it at once instead of leaving it to the queue.
type resumeEntry struct {
	trusted bool
	force   bool
}

// resumeSet remembers which downloads have to come back when the client does.
// all means the client never ran in this process, so every download is
// restored the way a start restores it and nothing is trusted; otherwise only
// the listed ones are. The entries of a set with all still carry force.
type resumeSet struct {
	all bool
	ids map[string]resumeEntry
}

// offlineLocked is true while there is no route for torrent traffic: the
// network is down, or the client is being replaced and the new one is not up.
func (m *Manager) offlineLocked() bool {
	return m.netState.State == NetworkDown || m.switching
}

func (m *Manager) noClientLocked() error {
	if m.offlineLocked() {
		return errNetworkDown
	}
	return errNoClient
}

func (m *Manager) setSwitching(on bool) {
	m.mu.Lock()
	m.switching = on
	m.mu.Unlock()
}

func (m *Manager) markResumeLocked(id string, trusted, force bool) {
	if m.resume == nil {
		m.resume = &resumeSet{ids: map[string]resumeEntry{}}
	}
	if m.resume.ids == nil {
		m.resume.ids = map[string]resumeEntry{}
	}
	e, ok := m.resume.ids[id]
	if !ok {
		e.trusted = trusted
	}
	e.force = e.force || force
	m.resume.ids[id] = e
}

func (m *Manager) markVerifiedLocked(id string) {
	if m.verified == nil {
		m.verified = map[string]bool{}
	}
	m.verified[id] = true
}

// jobsLocked turns downloads into restore jobs. pick says whether a download
// takes part and how it comes back.
func (m *Manager) jobsLocked(pick func(*Download) (bool, resumeEntry)) []restoreJob {
	jobs := make([]restoreJob, 0, len(m.items))
	for _, d := range m.items {
		var how resumeEntry
		if pick != nil {
			var include bool
			if include, how = pick(d); !include {
				continue
			}
		}
		jobs = append(jobs, restoreJob{
			id:       d.ID,
			infoHash: d.InfoHash,
			source:   d.Source,
			dest:     d.Destination,
			flat:     d.Flat,
			inPlace:  d.InPlace,
			paused:   d.Status == StatusPaused,
			complete: d.Status == StatusCompleted,
			seeding:  d.Seeding,
			gen:      m.gen,
			trusted:  how.trusted,
			force:    how.force,
		})
	}
	return jobs
}

func (m *Manager) resumeJobsLocked() []restoreJob {
	r := m.resume
	m.resume = nil
	if r == nil {
		return nil
	}
	return m.jobsLocked(func(d *Download) (bool, resumeEntry) {
		e, listed := r.ids[d.ID]
		if r.all {
			return true, resumeEntry{force: e.force}
		}
		if !listed || d.Status == StatusFailed {
			return false, resumeEntry{}
		}
		return true, e
	})
}

// NetworkStatus tells whether torrent traffic currently has the route the
// user asked for.
func (m *Manager) NetworkStatus() NetworkState {
	mode := m.config().NetworkMode
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.netState.State == "" {
		return NetworkState{Mode: mode, State: NetworkOK}
	}
	return m.netState
}

func (m *Manager) ListNetworkInterfaces() ([]NetInterface, error) {
	list, err := m.netEnv.interfaces()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errNetIfaceList, err)
	}
	return netInterfaces(list), nil
}

type proxySecret struct {
	user string
	pass string
}

func (m *Manager) clearPasswordCache() {
	m.passMu.Lock()
	m.passCache = nil
	m.passMu.Unlock()
}

func (m *Manager) SetProxyPassword(password string) error {
	if len(password) > maxProxyPassLen {
		return errProxyPasswordSize
	}
	if m.proxyStore == nil {
		return fmt.Errorf("%w: no credential store", errProxyCredentials)
	}
	if password == "" {
		if err := m.proxyStore.Delete(); err != nil {
			return fmt.Errorf("%w: %w", errProxyCredentials, err)
		}
	} else if err := m.proxyStore.Save(account.Credential{Token: password, Username: m.config().ProxyUsername}); err != nil {
		return fmt.Errorf("%w: %w", errProxyCredentials, err)
	}
	m.clearPasswordCache()
	m.kickNetwork()
	return nil
}

// HasProxyPassword is true when a password is stored for the user name that
// is saved now; one stored for another name is as good as none.
func (m *Manager) HasProxyPassword() (bool, error) {
	if m.proxyStore == nil {
		return false, fmt.Errorf("%w: no credential store", errProxyCredentials)
	}
	cred, err := m.proxyStore.Load()
	if errors.Is(err, account.ErrNoCredential) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("%w: %w", errProxyCredentials, err)
	}
	return cred.Username == m.config().ProxyUsername, nil
}

// TestProxy checks the saved proxy settings, whatever the current mode: the
// proxy is reached and the login is accepted, nothing is connected through it.
func (m *Manager) TestProxy(ctx context.Context) error {
	cfg := m.config()
	if cfg.ProxyHost == "" || cfg.ProxyPort == 0 {
		return errProxyNotSet
	}
	pass, err := m.proxyPassword(cfg.ProxyUsername)
	if err != nil {
		return err
	}
	return testProxy(ctx, netPlan{
		mode:  settings.NetworkProxy,
		ptype: cfg.ProxyType,
		phost: cfg.ProxyHost,
		pport: cfg.ProxyPort,
		puser: cfg.ProxyUsername,
		ppass: pass,
	})
}

// proxyPassword reads the store once per user name: the monitor asks every few
// seconds, and a keychain is not meant to be polled. A password saved for
// another user name is refused, not sent.
func (m *Manager) proxyPassword(user string) (string, error) {
	if user == "" {
		return "", nil
	}
	m.passMu.Lock()
	if c := m.passCache; c != nil && c.user == user {
		pass := c.pass
		m.passMu.Unlock()
		return pass, nil
	}
	m.passMu.Unlock()
	if m.proxyStore == nil {
		return "", fmt.Errorf("%w: no credential store", errProxyCredentials)
	}
	cred, err := m.proxyStore.Load()
	if errors.Is(err, account.ErrNoCredential) {
		return m.cachePassword(user, ""), nil
	}
	if err != nil {
		return "", fmt.Errorf("%w: %w", errProxyCredentials, err)
	}
	if cred.Username != user {
		return "", errProxyMismatch
	}
	return m.cachePassword(user, cred.Token), nil
}

func (m *Manager) cachePassword(user, pass string) string {
	m.passMu.Lock()
	m.passCache = &proxySecret{user: user, pass: pass}
	m.passMu.Unlock()
	return pass
}

func (m *Manager) kickNetwork() {
	select {
	case m.netKick <- struct{}{}:
	default:
	}
}

func (m *Manager) netMonitor(ctx context.Context) {
	defer m.wg.Done()
	interval := m.netInterval
	if interval <= 0 {
		interval = netPollInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-m.netKick:
		}
		m.reconcileNetwork(ctx)
	}
}

// resolveNetwork works out what the client should be bound to right now. An
// error means the route the user asked for is not there, and traffic must not
// flow until it is.
func (m *Manager) resolveNetwork(ctx context.Context, cfg settings.Settings, active *netPlan) (netPlan, error) {
	switch cfg.NetworkMode {
	case settings.NetworkDirect:
		return netPlan{mode: settings.NetworkDirect}, nil
	case settings.NetworkInterface:
		return m.resolveInterface(cfg, active)
	case settings.NetworkProxy:
		return m.resolveProxy(ctx, cfg)
	}
	return netPlan{}, fmt.Errorf("%w: %q", errNoClient, cfg.NetworkMode)
}

func (m *Manager) resolveInterface(cfg settings.Settings, active *netPlan) (netPlan, error) {
	list, err := m.netEnv.interfaces()
	if err != nil {
		return netPlan{}, fmt.Errorf("%w: %w", errNetIfaceList, err)
	}
	ifc, ok := findInterface(list, cfg.NetworkInterface)
	if !ok {
		return netPlan{}, fmt.Errorf("%w: %s", errNetIfaceMissing, cfg.NetworkInterface)
	}
	if !ifc.Up {
		return netPlan{}, fmt.Errorf("%w: %s", errNetIfaceDown, ifc.Name)
	}
	v4, v6 := usableAddrs(ifc.Addrs)
	if len(v4) == 0 && len(v6) == 0 {
		return netPlan{}, fmt.Errorf("%w: %s", errNetIfaceNoAddr, ifc.Name)
	}
	var plan netPlan
	if active != nil && active.mode == settings.NetworkInterface && active.iface == ifc.Name && active.index == ifc.Index &&
		stillBound(active.ip4, v4) && stillBound(active.ip6, v6) {
		plan = *active
	} else {
		plan = netPlan{mode: settings.NetworkInterface, iface: ifc.Name, index: ifc.Index}
		if len(v4) > 0 {
			plan.ip4 = v4[0]
		}
		if len(v6) > 0 {
			plan.ip6 = v6[0]
		}
	}
	if err := m.netEnv.hostCheck(ifc, plan.ip4.IsValid(), plan.ip6.IsValid()); err != nil {
		return netPlan{}, err
	}
	servers, err := m.netEnv.dns(ifc)
	if err != nil {
		return netPlan{}, fmt.Errorf("%w: %s: %w", errNetIfaceHostCheck, ifc.Name, err)
	}
	plan.dns = joinAddrs(servers)
	return plan, nil
}

// stillBound is true when the address the client is bound to is still on the
// adapter, or when the client is not bound to that family at all: an adapter
// that gains an address must not cost a restart, one that loses the address
// in use must.
func stillBound(bound netip.Addr, present []netip.Addr) bool {
	if !bound.IsValid() {
		return true
	}
	for _, a := range present {
		if a == bound {
			return true
		}
	}
	return false
}

func (m *Manager) resolveProxy(ctx context.Context, cfg settings.Settings) (netPlan, error) {
	pass, err := m.proxyPassword(cfg.ProxyUsername)
	if err != nil {
		return netPlan{}, err
	}
	plan := netPlan{
		mode:  settings.NetworkProxy,
		ptype: cfg.ProxyType,
		phost: cfg.ProxyHost,
		pport: cfg.ProxyPort,
		puser: cfg.ProxyUsername,
		ppass: pass,
	}
	probeCtx, cancel := context.WithTimeout(ctx, proxyProbeLimit)
	defer cancel()
	if err := m.netEnv.probe(probeCtx, plan.proxyAddr()); err != nil {
		if ctx.Err() != nil {
			return netPlan{}, ctx.Err()
		}
		return netPlan{}, fmt.Errorf("%w: %w", errProxyUnreachable, err)
	}
	return plan, nil
}

// reconcileNetwork makes the client match the settings and the state of the
// route. Only one reconcile runs at a time, so a settings change and a
// monitor tick never rebuild the client against each other.
func (m *Manager) reconcileNetwork(ctx context.Context) {
	m.netMu.Lock()
	defer m.netMu.Unlock()
	if ctx.Err() != nil {
		return
	}
	cfg := m.config()

	m.mu.Lock()
	if m.closing {
		m.mu.Unlock()
		return
	}
	var active *netPlan
	if m.netActive != nil {
		a := *m.netActive
		active = &a
	}
	hasClient := m.client != nil
	m.mu.Unlock()

	plan, err := m.resolveNetwork(ctx, cfg, active)
	if ctx.Err() != nil {
		return
	}
	if err == nil && hasClient && active != nil && *active == plan {
		return
	}
	m.setSwitching(true)
	defer m.setSwitching(false)
	if err != nil {
		m.enterDown(cfg.NetworkMode, err)
		return
	}
	m.teardownClient()
	m.bringUp(ctx, cfg, plan)
}

func (m *Manager) enterDown(mode string, cause error) {
	m.teardownClient()
	code, reason := reasonOf(cause)
	m.mu.Lock()
	m.netActive = nil
	// The state says "down" from here on, which is what keeps requests out;
	// leaving switching set past this point would only be a second answer to
	// the same question.
	m.switching = false
	changed := m.setNetStateLocked(NetworkState{Mode: mode, State: NetworkDown, Code: code, Reason: reason})
	m.mu.Unlock()
	if changed {
		slog.Warn("torrent network is down", "mode", mode, "code", code, "reason", reason)
	}
}

// setNetStateLocked stores the state and tells the window when what it shows
// changes; a reason that only differs in wording is stored without an event.
func (m *Manager) setNetStateLocked(next NetworkState) bool {
	prev := m.netState
	m.netState = next
	if prev.Mode == next.Mode && prev.State == next.State && prev.Code == next.Code && prev.Address == next.Address && prev.Warning == next.Warning {
		return false
	}
	emit(eventNetwork, next)
	return true
}

func (m *Manager) bringUp(ctx context.Context, cfg settings.Settings, plan netPlan) {
	m.mu.Lock()
	completion := m.pieceCompletion
	build := m.buildClient
	m.mu.Unlock()
	if completion == nil {
		return
	}
	// The client outlives the check that built it, so it must not stop with
	// ctx; the manager cancels it when the client is replaced or closed.
	clientCtx, cancelClient := context.WithCancel(context.WithoutCancel(ctx))
	cl, err := build(clientCtx, cfg, m.metaDir, completion, plan)
	if err != nil {
		cancelClient()
		m.enterDown(cfg.NetworkMode, fmt.Errorf("%w: %w", errNoClient, err))
		return
	}

	m.mu.Lock()
	if m.closing || ctx.Err() != nil {
		m.mu.Unlock()
		cancelClient()
		cl.close()
		return
	}
	m.gen++
	cl.gen = m.gen
	m.client = cl
	m.clientCtx, m.clientCancel = clientCtx, cancelClient
	// Cleared in the step that installs the client: a request that comes in
	// afterwards finds a client, and one that came in before was told to wait.
	m.switching = false
	active := plan
	m.netActive = &active
	if m.setNetStateLocked(NetworkState{Mode: plan.mode, State: NetworkOK, Address: plan.address(), Warning: plan.warning()}) {
		slog.Info("torrent network is up", "mode", plan.mode, "address", plan.address())
	}
	jobs := m.resumeJobsLocked()
	if len(jobs) > 0 {
		seed := cfg.SeedAfterDownload
		m.spawnTrackedLocked(func() { m.restorePass(clientCtx, cl, jobs, seed) })
	}
	m.mu.Unlock()
}

// teardownClient stops everything that runs on the current client, closes it
// and parks the downloads that were active as queued. It is the same for a
// lost route and for a change of settings: in both the client the data was
// served by is gone, and the downloads wait for the next one.
func (m *Manager) teardownClient() {
	m.mu.Lock()
	cl := m.client
	if cl == nil {
		m.mu.Unlock()
		return
	}
	m.client = nil
	m.gen++
	cancelClient := m.clientCancel
	m.clientCtx, m.clientCancel = nil, nil

	// A download is trusted, and comes back without a recheck, only when it
	// was settled on this process's client and was not in the middle of
	// checking or looking for metadata: whatever this process has not seen
	// through still needs the check a start would give it.
	resume := &resumeSet{ids: map[string]resumeEntry{}}
	for _, d := range m.items {
		if d.Status == StatusFailed || (d.Status == StatusCompleted && !d.Seeding) {
			continue
		}
		settled := m.verified[d.ID] && d.Status != StatusVerifying && d.Status != StatusMetadata
		resume.ids[d.ID] = resumeEntry{trusted: settled}
	}
	m.resume = resume
	m.verified = nil

	engines := make([]engineTorrent, 0, len(m.engines))
	for id, eng := range m.engines {
		engines = append(engines, eng)
		delete(m.engines, id)
	}
	pendings := make([]*pending, 0, len(m.pending))
	for hash, p := range m.pending {
		pendings = append(pendings, p)
		delete(m.pending, hash)
	}
	fetches := make([]context.CancelFunc, 0, len(m.fetching))
	for _, e := range m.fetching {
		fetches = append(fetches, e.cancel)
	}
	jobs := make([]*jobState, 0, len(m.jobs))
	for _, j := range m.jobs {
		jobs = append(jobs, j)
	}

	persist := false
	for _, d := range m.items {
		delete(m.rates, d.ID)
		switch {
		case occupiesSlot(d.Status):
			m.idleLocked(d, StatusQueued)
		case seedingCompleted(d):
			d.UploadSpeed = 0
		default:
			continue
		}
		d.Seeders, d.Peers = 0, 0
		persist = true
		emit(eventUpdated, snapshot(d))
	}
	if persist {
		if err := m.persistLocked(); err != nil {
			// Queued is what loadLocked turns any active status into on the next
			// start, so the file that failed to update still reads back the same.
			// persistLocked has raised the degraded state already.
			slog.Error("persist parked downloads", "error", err)
		}
	}
	m.mu.Unlock()

	if cancelClient != nil {
		cancelClient()
	}
	for _, cancel := range fetches {
		cancel()
	}
	for _, j := range jobs {
		j.cancel()
		<-j.done
	}
	for _, p := range pendings {
		p.torrent.drop()
	}
	for _, eng := range engines {
		eng.drop()
	}
	cl.close()
}

// restorePass brings the given downloads back on cl, one after another.
func (m *Manager) restorePass(ctx context.Context, cl *client, jobs []restoreJob, seed bool) {
	for _, j := range jobs {
		if ctx.Err() != nil {
			return
		}
		if j.complete && !keepsSeeding(j, seed) {
			m.setSeeding(j.id, false)
			continue
		}
		m.restoreOne(ctx, cl, j)
	}

	m.mu.Lock()
	if err := m.persistLocked(); err != nil {
		// Same reasoning as applySettings: this is the end-of-pass flush for
		// Seeding flips made by setSeeding during the loop above, with no
		// caller waiting on this background pass.
		slog.Error("persist restore pass", "error", err)
	}
	m.schedule()
	m.mu.Unlock()
}

// queueWhileDownLocked is Resume and ForceStart while there is no client:
// nothing can start, so the download waits in the queue and comes back with
// the client.
func (m *Manager) queueWhileDownLocked(d *Download, what string, force bool) error {
	before := *d
	d.Status = StatusQueued
	d.Error = ""
	if err := m.persistLocked(); err != nil {
		*d = before
		emit(eventUpdated, snapshot(d))
		return fmt.Errorf("%s: %w", what, err)
	}
	m.markResumeLocked(d.ID, false, force)
	emit(eventUpdated, snapshot(d))
	return nil
}
