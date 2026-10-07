package download

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/netip"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/anacrolix/dht/v2"
	"github.com/anacrolix/torrent"
	"golang.org/x/sync/errgroup"
)

const (
	nameLookupLimit = 5 * time.Second
	bootstrapTTL    = 5 * time.Minute
	retryBase       = 5 * time.Second
	retryCap        = 5 * time.Minute

	// maxNameMisses is how many answers in a row that the name does not exist
	// a lost tracker survives. With the backoff above that is about eight
	// minutes, long enough for a name server that answers wrongly while a
	// tunnel comes up; a server that cannot be reached counts for nothing.
	maxNameMisses = 5
)

var (
	errNoAdapterDNS = errors.New("the adapter has no name server to ask")
	errNoUsableAddr = errors.New("the name has no address of a family the adapter has")
)

// dnsDial is what the resolver dials instead of the servers the system would
// pick: the name servers of the bound adapter, from the bound address. A name
// is then either resolved through the adapter or not at all.
func (b *bindDialer) dnsDial(servers []netip.Addr) func(ctx context.Context, network, address string) (net.Conn, error) {
	return func(ctx context.Context, network, _ string) (net.Conn, error) {
		proto := "udp"
		if strings.HasPrefix(network, "tcp") {
			proto = "tcp"
		}
		var errs []error
		for _, server := range servers {
			local, family := b.ip4, "4"
			if server.Is6() {
				local, family = b.ip6, "6"
			}
			if !local.IsValid() {
				continue
			}
			d := net.Dialer{Timeout: nameLookupLimit, Control: b.control}
			if proto == "tcp" {
				d.LocalAddr = &net.TCPAddr{IP: local.AsSlice()}
			} else {
				d.LocalAddr = &net.UDPAddr{IP: local.AsSlice()}
			}
			port := b.dnsPort
			if port == "" {
				port = "53"
			}
			conn, err := d.DialContext(ctx, proto+family, net.JoinHostPort(server.String(), port))
			if err == nil {
				return conn, nil
			}
			errs = append(errs, err)
			if ctx.Err() != nil {
				break
			}
		}
		if len(errs) == 0 {
			return nil, errNoAdapterDNS
		}
		return nil, errors.Join(errs...)
	}
}

// newResolver forces the Go resolver, which is the only one that honours Dial,
// on every system this builds for.
func (b *bindDialer) newResolver(servers []netip.Addr) *net.Resolver {
	return &net.Resolver{PreferGo: true, Dial: b.dnsDial(servers)}
}

// nameResolver serves the places where the library would resolve a name on its
// own with the system resolver: UDP tracker hosts and the DHT bootstrap nodes.
type nameResolver struct {
	r      *net.Resolver
	allow4 bool
	allow6 bool
	// log and seen make a failing name show up once; nil means the default
	// logger and a record for every failure.
	log  *slog.Logger
	seen *hostLog
	// later brings back the trackers whose name did not resolve; nil means
	// they are not tried again.
	later *retrier
}

type hostLog struct {
	mu   sync.Mutex
	done map[string]bool
}

func newHostLog() *hostLog { return &hostLog{done: map[string]bool{}} }

// first says whether this is the first time host is asked about.
func (h *hostLog) first(host string) bool {
	if h == nil {
		return true
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.done[host] {
		return false
	}
	h.done[host] = true
	return true
}

// lostTracker is a UDP tracker that stays out of its torrent until its name
// resolves. tier is the index it would have in the list the torrent got, which
// keeps a slot for a tier that has nothing else in it for now.
type lostTracker struct {
	u      *url.URL
	tier   int
	misses int
}

type retryJob struct {
	lost []lostTracker
}

// retrier owns the goroutines that try the lost trackers again. They end with
// the context, with their torrent, or when the client that owns the retrier
// is closed.
type retrier struct {
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	// after says when the next try is due; nil means the backoff below.
	after func(attempt int) <-chan time.Time

	mu     sync.Mutex
	closed bool
	jobs   map[*torrent.Torrent]*retryJob
}

func newRetrier(ctx context.Context) *retrier {
	ctx, cancel := context.WithCancel(ctx)
	return &retrier{ctx: ctx, cancel: cancel, jobs: map[*torrent.Torrent]*retryJob{}}
}

// join hands lost to the job of t, and says whether that job is new and needs a
// goroutine. A torrent has one job however often its trackers are lost.
func (r *retrier) join(t *torrent.Torrent, lost []lostTracker) (*retryJob, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil, false
	}
	job, ok := r.jobs[t]
	if !ok {
		job = &retryJob{}
		r.jobs[t] = job
	}
	for _, l := range lost {
		if !slices.ContainsFunc(job.lost, func(o lostTracker) bool { return o.u.String() == l.u.String() }) {
			job.lost = append(job.lost, l)
		}
	}
	return job, !ok
}

func (r *retrier) take(job *retryJob) []lostTracker {
	r.mu.Lock()
	defer r.mu.Unlock()
	batch := job.lost
	job.lost = nil
	return batch
}

// settle puts back what is still lost and ends the job, in the same step, when
// nothing is: a tracker that joins right after finds no job and starts a new one.
func (r *retrier) settle(t *torrent.Torrent, job *retryJob, still []lostTracker) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	job.lost = append(still, job.lost...)
	if len(job.lost) > 0 {
		return false
	}
	delete(r.jobs, t)
	return true
}

func (r *retrier) forget(t *torrent.Torrent, job *retryJob) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.jobs[t] == job {
		delete(r.jobs, t)
	}
}

// start runs fn as one of the retrier's goroutines. Once the retrier is
// stopped it runs nothing, so that wait cannot overlap a late start.
func (r *retrier) start(fn func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		fn()
	}()
}

func (r *retrier) stop() {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.closed = true
	r.mu.Unlock()
	r.cancel()
}

func (r *retrier) wait() {
	if r != nil {
		r.wg.Wait()
	}
}

func (r *retrier) due(attempt int) <-chan time.Time {
	if r.after != nil {
		return r.after(attempt)
	}
	return time.After(retryDelay(attempt))
}

// retryDelay grows threefold with every try, from retryBase up to retryCap: a
// name server that is not up yet is asked soon, one that stays down is not
// hammered.
func retryDelay(attempt int) time.Duration {
	d := retryBase
	for i := 0; i < attempt && d < retryCap; i++ {
		d *= 3
	}
	return min(d, retryCap)
}

// query is the one place a name is looked up. An answer with no address of a
// family the adapter has is a failure too: the tracker is as unreachable as
// with no answer at all.
func (n nameResolver) query(ctx context.Context, host string) ([]netip.Addr, error) {
	ctx, cancel := context.WithTimeout(ctx, nameLookupLimit)
	defer cancel()
	addrs, err := n.r.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, err
	}
	var out []netip.Addr
	for _, a := range addrs {
		a = a.Unmap()
		if (a.Is4() && n.allow4) || (a.Is6() && n.allow6) {
			out = append(out, a)
		}
	}
	if len(out) == 0 {
		return nil, errNoUsableAddr
	}
	return out, nil
}

func (n nameResolver) logger() *slog.Logger {
	if n.log != nil {
		return n.log
	}
	return slog.Default()
}

// lookup resolves host, and says so once when it cannot. A failure that comes
// from ctx ending is the shutdown talking and is not reported.
func (n nameResolver) lookup(ctx context.Context, host string) []netip.Addr {
	out, err := n.lookupErr(ctx, host)
	if err != nil {
		return nil
	}
	return out
}

func (n nameResolver) lookupErr(ctx context.Context, host string) ([]netip.Addr, error) {
	out, err := n.query(ctx, host)
	if err != nil {
		if ctx.Err() == nil && n.seen.first(host) {
			n.logger().Warn("resolve host through the adapter", "host", host, "error", err)
		}
		return nil, err
	}
	return out, nil
}

// missingName is true when the server answered that the name has no address
// to use, as opposed to not answering: only the first says anything about the
// name.
func missingName(err error) bool {
	if errors.Is(err, errNoUsableAddr) {
		return true
	}
	var dnsErr *net.DNSError
	return errors.As(err, &dnsErr) && dnsErr.IsNotFound
}

// lookupHosts resolves the hosts side by side. A host that did not resolve is
// left out of the first answer and has its error in the second.
func (n nameResolver) lookupHosts(ctx context.Context, hosts map[string]bool) (map[string]netip.Addr, map[string]error) {
	resolved := map[string]netip.Addr{}
	failed := map[string]error{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for host := range hosts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			addrs, err := n.lookupErr(ctx, host)
			mu.Lock()
			defer mu.Unlock()
			if a, ok := first(addrs); ok {
				resolved[host] = a
			} else if err != nil {
				failed[host] = err
			}
		}()
	}
	wg.Wait()
	return resolved, failed
}

func (n nameResolver) resolveHosts(ctx context.Context, hosts map[string]bool) map[string]netip.Addr {
	resolved, _ := n.lookupHosts(ctx, hosts)
	return resolved
}

// first prefers IPv4: it is what the tracker protocol was written for.
func first(addrs []netip.Addr) (netip.Addr, bool) {
	for _, a := range addrs {
		if a.Is4() {
			return a, true
		}
	}
	if len(addrs) > 0 {
		return addrs[0], true
	}
	return netip.Addr{}, false
}

func isUDPScheme(s string) bool { return s == "udp" || s == "udp4" || s == "udp6" }

// resolveUDPTrackers swaps the host name of every UDP tracker for an address
// resolved through the adapter. The UDP tracker client resolves the name on
// its own, with the system resolver, on every announce; the protocol carries
// no host name, so the address is all the tracker ever needed. A tracker whose
// name does not resolve is left out of the list for now and handed back as
// lost, to be tried again by retryLost.
func (n nameResolver) resolveUDPTrackers(ctx context.Context, tiers [][]string) ([][]string, []lostTracker) {
	hosts := map[string]bool{}
	parsed := make([][]*url.URL, len(tiers))
	for i, tier := range tiers {
		for _, raw := range tier {
			u, err := url.Parse(strings.TrimSpace(raw))
			if err != nil {
				parsed[i] = append(parsed[i], nil)
				continue
			}
			parsed[i] = append(parsed[i], u)
			if isUDPScheme(u.Scheme) {
				if _, err := netip.ParseAddr(u.Hostname()); err != nil {
					hosts[u.Hostname()] = true
				}
			}
		}
	}

	resolved := n.resolveHosts(ctx, hosts)

	var out [][]string
	var lost []lostTracker
	for i, tier := range tiers {
		var kept []string
		lostHere := false
		for j, raw := range tier {
			u := parsed[i][j]
			switch {
			case u == nil:
			case !isUDPScheme(u.Scheme):
				kept = append(kept, raw)
			default:
				if _, err := netip.ParseAddr(u.Hostname()); err == nil {
					kept = append(kept, raw)
					continue
				}
				if a, ok := resolved[u.Hostname()]; ok {
					kept = append(kept, withHost(u, a))
				} else {
					lost = append(lost, lostTracker{u: u, tier: len(out)})
					lostHere = true
				}
			}
		}
		if len(kept) > 0 {
			out = append(out, kept)
		} else if lostHere {
			// An empty tier holds the place: the tracker that comes back goes
			// where it was, not into the tier that followed it.
			out = append(out, []string{})
		}
	}
	for len(out) > 0 && len(out[len(out)-1]) == 0 {
		out = out[:len(out)-1]
	}
	return out, lost
}

// retryLost tries the lost trackers of t again, with growing pauses, until
// every one has come back, t is dropped, or the client is closed. A name server
// that is not up yet, as right after a tunnel is, would otherwise cost the
// torrent its UDP trackers for as long as the client lives.
func (n nameResolver) retryLost(t *torrent.Torrent, lost []lostTracker) {
	l := n.later
	if l == nil || len(lost) == 0 {
		return
	}
	job, fresh := l.join(t, lost)
	if !fresh {
		return
	}
	l.start(func() {
		defer l.forget(t, job)
		for attempt := 0; ; attempt++ {
			select {
			case <-l.ctx.Done():
				return
			case <-t.Closed():
				return
			case <-l.due(attempt):
			}
			if l.settle(t, job, n.recover(l.ctx, t, l.take(job))) {
				return
			}
		}
	})
}

// recover resolves the names of lost again, gives t the trackers whose name
// resolved now, and returns the rest. A tracker whose name the server keeps
// saying does not exist is dropped.
func (n nameResolver) recover(ctx context.Context, t *torrent.Torrent, lost []lostTracker) []lostTracker {
	hosts := map[string]bool{}
	for _, l := range lost {
		hosts[l.u.Hostname()] = true
	}
	resolved, failed := n.lookupHosts(ctx, hosts)

	var back [][]string
	var still []lostTracker
	for _, l := range lost {
		a, ok := resolved[l.u.Hostname()]
		if !ok {
			if missingName(failed[l.u.Hostname()]) {
				l.misses++
			} else {
				l.misses = 0
			}
			if l.misses >= maxNameMisses {
				n.logger().Warn("tracker name does not exist, not trying again", "host", l.u.Hostname())
				continue
			}
			still = append(still, l)
			continue
		}
		for len(back) <= l.tier {
			back = append(back, nil)
		}
		back[l.tier] = append(back[l.tier], withHost(l.u, a))
		n.logger().Info("tracker name resolved after a retry", "host", l.u.Hostname())
	}
	if len(back) > 0 {
		t.AddTrackers(back)
	}
	return still
}

func withHost(u *url.URL, a netip.Addr) string {
	c := *u
	if port := u.Port(); port != "" {
		c.Host = net.JoinHostPort(a.String(), port)
	} else if a.Is6() {
		c.Host = "[" + a.String() + "]"
	} else {
		c.Host = a.String()
	}
	return c.String()
}

// dhtStartingNodes replaces the library default, which resolves the bootstrap
// hosts with the system resolver through a global cache of its own. Nothing
// here calls into that cache, so its refresher never starts.
func (n nameResolver) dhtStartingNodes(ctx context.Context) func(string) dht.StartingNodesGetter {
	return func(string) dht.StartingNodesGetter { return n.bootstrapGetter(ctx) }
}

func (n nameResolver) bootstrapGetter(ctx context.Context) dht.StartingNodesGetter {
	var mu sync.Mutex
	var cached []dht.Addr
	var fetched time.Time
	return func() ([]dht.Addr, error) {
		mu.Lock()
		defer mu.Unlock()
		if len(cached) > 0 && time.Since(fetched) < bootstrapTTL {
			return cached, nil
		}
		nodes := n.resolveHostPorts(ctx, dht.DefaultGlobalBootstrapHostPorts)
		if len(nodes) == 0 {
			return nil, errors.New("no bootstrap node resolved through the adapter")
		}
		cached, fetched = nodes, time.Now()
		return cached, nil
	}
}

func (n nameResolver) resolveHostPorts(ctx context.Context, hostPorts []string) []dht.Addr {
	type target struct {
		host string
		port int
	}
	var targets []target
	for _, hp := range hostPorts {
		host, portText, err := net.SplitHostPort(hp)
		if err != nil {
			continue
		}
		port, err := strconv.Atoi(portText)
		if err != nil {
			continue
		}
		targets = append(targets, target{host, port})
	}
	results := make([][]dht.Addr, len(targets))
	var group errgroup.Group
	for i, tg := range targets {
		group.Go(func() error {
			for _, a := range n.lookup(ctx, tg.host) {
				results[i] = append(results[i], dht.NewAddr(&net.UDPAddr{IP: a.AsSlice(), Port: tg.port}))
			}
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil
	}
	var out []dht.Addr
	for _, r := range results {
		out = append(out, r...)
	}
	return out
}
