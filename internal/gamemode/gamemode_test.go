package gamemode

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"typhon/internal/library"
	"typhon/internal/settings"
)

func bp(v bool) *bool { return &v }

type fakePower struct {
	mu       sync.Mutex
	active   string
	missing  bool
	gone     map[string]bool
	setErr   error
	sets     []string
	onSet    func()
	activeEr error
}

func (p *fakePower) Active() (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.active, p.activeEr
}

func (p *fakePower) Has(scheme string) (bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return !p.missing && !p.gone[scheme], nil
}

func (p *fakePower) Set(scheme string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.onSet != nil {
		p.onSet()
	}
	if p.setErr != nil {
		return p.setErr
	}
	p.sets = append(p.sets, scheme)
	p.active = scheme
	return nil
}

func (p *fakePower) setCalls() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.sets...)
}

func (p *fakePower) current() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.active
}

type fakePriority struct {
	mu      sync.Mutex
	classes map[uint32]uint32
	denied  map[uint32]bool
	created map[uint32]time.Time
}

var epoch = time.Unix(1_000_000, 0)

func (p *fakePriority) Created(pid uint32) (time.Time, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.classes[pid]; !ok {
		return time.Time{}, errors.New("no such process")
	}
	return p.created[pid], nil
}

func (p *fakePriority) spawn(pid uint32, class uint32, created time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.classes[pid] = class
	p.created[pid] = created
}

func (p *fakePriority) Get(pid uint32) (uint32, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	c, ok := p.classes[pid]
	if !ok {
		return 0, errors.New("no such process")
	}
	return c, nil
}

func (p *fakePriority) Set(pid, class uint32) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.denied[pid] {
		return errors.New("access denied")
	}
	p.classes[pid] = class
	return nil
}

func (p *fakePriority) class(pid uint32) uint32 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.classes[pid]
}

type fakeWindow struct {
	mu      sync.Mutex
	visible bool
	noHide  bool
	entered chan struct{}
	release chan struct{}
	hides   int
	shows   int
}

func (w *fakeWindow) CanHide() bool { return !w.noHide }
func (w *fakeWindow) Visible() bool { w.mu.Lock(); defer w.mu.Unlock(); return w.visible }
func (w *fakeWindow) Hide() {
	if w.entered != nil {
		close(w.entered)
		<-w.release
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.visible = false
	w.hides++
}
func (w *fakeWindow) setVisible(v bool) { w.mu.Lock(); defer w.mu.Unlock(); w.visible = v }
func (w *fakeWindow) Show()             { w.mu.Lock(); defer w.mu.Unlock(); w.visible = true; w.shows++ }
func (w *fakeWindow) counts() (hides, shows int, visible bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.hides, w.shows, w.visible
}

type rig struct {
	svc      *Service
	dir      string
	power    *fakePower
	prio     *fakePriority
	win      *fakeWindow
	cfg      settings.GameMode
	pids     map[string]uint32
	children map[uint32][]uint32
	after    chan time.Time

	mu     sync.Mutex
	events []ErrorInfo
}

func (r *rig) errorCodes() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for _, e := range r.events {
		out = append(out, e.Code)
	}
	return out
}

func newRig(t *testing.T) *rig {
	t.Helper()
	r := &rig{
		dir:      t.TempDir(),
		power:    &fakePower{active: "balanced"},
		prio:     &fakePriority{classes: map[uint32]uint32{}, denied: map[uint32]bool{}, created: map[uint32]time.Time{}},
		win:      &fakeWindow{visible: true},
		pids:     map[string]uint32{},
		children: map[uint32][]uint32{},
		after:    make(chan time.Time),
	}
	deps := Deps{
		Settings: func() settings.Settings {
			s := settings.Defaults()
			s.GameMode = r.cfg
			return s
		},
		PID: func(id string) (uint32, bool) {
			r.mu.Lock()
			defer r.mu.Unlock()
			pid, ok := r.pids[id]
			return pid, ok
		},
		Window: r.win,
		Emit: func(name string, data any) {
			if name != ErrorEvent {
				t.Errorf("unexpected event %q", name)
				return
			}
			info, ok := data.(ErrorInfo)
			if !ok {
				t.Errorf("event payload is %T", data)
				return
			}
			r.mu.Lock()
			r.events = append(r.events, info)
			r.mu.Unlock()
		},
		Power:       r.power,
		Priority:    r.prio,
		Descendants: func(root uint32) ([]uint32, error) { return r.children[root], nil },
		After:       func(time.Duration) <-chan time.Time { return r.after },
	}
	svc, err := New(r.dir, deps)
	if err != nil {
		t.Fatal(err)
	}
	r.svc = svc
	return r
}

// begin sets the service up the way start does, minus the worker goroutine,
// so a test drives the queue itself with drain.
func (r *rig) begin() {
	r.svc.ctx, r.svc.cancel = context.WithCancel(context.Background())
	r.svc.procMu.Lock()
	r.svc.recoverLocked()
	r.svc.procMu.Unlock()
}

func (r *rig) addGame(id string, pid uint32) library.Game {
	r.mu.Lock()
	r.pids[id] = pid
	r.mu.Unlock()
	r.prio.mu.Lock()
	r.prio.classes[pid] = NormalPriorityClass
	r.prio.created[pid] = epoch
	r.prio.mu.Unlock()
	return library.Game{ID: id}
}

func (r *rig) markerExists() bool {
	_, err := os.Stat(filepath.Join(r.dir, markerName))
	return err == nil
}

func TestResolve(t *testing.T) {
	cases := []struct {
		name   string
		global settings.GameMode
		over   *library.GameModeOverride
		want   Options
	}{
		{name: "no override inherits", global: settings.GameMode{HideLauncher: true}, want: Options{HideLauncher: true}},
		{name: "nil field inherits", global: settings.GameMode{RaisePriority: true}, over: &library.GameModeOverride{HideLauncher: bp(true)}, want: Options{RaisePriority: true, HideLauncher: true}},
		{name: "false disables", global: settings.GameMode{HideLauncher: true, HighPerformance: true}, over: &library.GameModeOverride{HideLauncher: bp(false)}, want: Options{HighPerformance: true}},
		{name: "true enables", over: &library.GameModeOverride{RaisePriority: bp(true)}, want: Options{RaisePriority: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Resolve(tc.global, tc.over); got != tc.want {
				t.Fatalf("Resolve = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestNewRejectsEmptyDirAndMissingDeps(t *testing.T) {
	r := newRig(t)
	if _, err := New("", r.svc.deps); err == nil {
		t.Fatal("empty dir accepted")
	}
	if _, err := New(r.dir, Deps{}); err == nil {
		t.Fatal("empty deps accepted")
	}
}

func TestRefcountWithTwoGames(t *testing.T) {
	r := newRig(t)
	r.cfg = settings.GameMode{HighPerformance: true, HideLauncher: true, RaisePriority: true}
	r.begin()
	a, b := r.addGame("a", 10), r.addGame("b", 20)

	r.svc.SessionStarted(a)
	r.svc.SessionStarted(b)
	r.svc.drain()
	if got := r.power.setCalls(); len(got) != 1 || got[0] != HighPerformanceScheme {
		t.Fatalf("power sets after two starts = %v, want one switch", got)
	}
	if hides, _, _ := r.win.counts(); hides != 1 {
		t.Fatalf("hides = %d, want 1", hides)
	}
	if r.prio.class(10) != AboveNormalPriorityClass || r.prio.class(20) != AboveNormalPriorityClass {
		t.Fatal("both games should be raised")
	}

	r.svc.SessionStopped("a")
	r.svc.drain()
	if _, shows, _ := r.win.counts(); len(r.power.setCalls()) != 1 || shows != 0 || !r.markerExists() {
		t.Fatal("first stop must not restore while another game runs")
	}

	r.svc.SessionStopped("b")
	r.svc.drain()
	if got := r.power.setCalls(); len(got) != 2 || got[1] != "balanced" {
		t.Fatalf("power sets = %v, want switch and restore", got)
	}
	if _, shows, visible := r.win.counts(); shows != 1 || !visible {
		t.Fatalf("shows = %d, visible = %v", shows, visible)
	}
	if r.markerExists() {
		t.Fatal("marker left after the last stop")
	}
}

func TestMarkerExistsBeforeSystemChange(t *testing.T) {
	r := newRig(t)
	r.cfg = settings.GameMode{HighPerformance: true}
	r.begin()
	var atSet []bool
	r.power.onSet = func() { atSet = append(atSet, r.markerExists()) }
	r.svc.SessionStarted(r.addGame("a", 10))
	r.svc.drain()
	if len(atSet) != 1 || !atSet[0] {
		t.Fatalf("marker present at the power change = %v, want [true]", atSet)
	}
	r.svc.SessionStopped("a")
	r.svc.drain()
	if r.markerExists() {
		t.Fatal("marker not removed after restore")
	}
}

func TestMarkerWriteFailureBlocksChange(t *testing.T) {
	r := newRig(t)
	r.cfg = settings.GameMode{HighPerformance: true, HideLauncher: true}
	r.begin()
	blocked := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocked, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	r.svc.store.path = filepath.Join(blocked, markerName)
	r.svc.SessionStarted(r.addGame("a", 10))
	r.svc.drain()
	if hides, _, _ := r.win.counts(); len(r.power.setCalls()) != 0 || hides != 0 {
		t.Fatal("system changed without a recovery marker")
	}
	codes := r.errorCodes()
	if len(codes) != 2 || codes[0] != CodeMarkerWriteFailed {
		t.Fatalf("error codes = %v", codes)
	}
}

func TestStartupRestoresFromMarker(t *testing.T) {
	cases := []struct {
		name     string
		active   string
		wantSets []string
	}{
		{name: "still on high performance", active: HighPerformanceScheme, wantSets: []string{"balanced"}},
		{name: "user changed the plan since", active: "custom"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t)
			r.power.active = tc.active
			if err := r.svc.store.save(marker{Power: &powerMark{Previous: "balanced", Applied: HighPerformanceScheme}, Hide: true}); err != nil {
				t.Fatal(err)
			}
			if err := r.svc.start(context.Background()); err != nil {
				t.Fatal(err)
			}
			got := r.power.setCalls()
			if len(got) != len(tc.wantSets) || (len(got) > 0 && got[0] != tc.wantSets[0]) {
				t.Fatalf("power sets = %v, want %v", got, tc.wantSets)
			}
			if r.markerExists() {
				t.Fatal("marker left after recovery")
			}
			if _, shows, _ := r.win.counts(); shows != 0 {
				t.Fatal("recovery must not show a window the new process never hid")
			}
			if err := r.svc.ServiceShutdown(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRestoreFailureKeepsMarker(t *testing.T) {
	r := newRig(t)
	r.power.active = HighPerformanceScheme
	r.power.setErr = errors.New("denied")
	if err := r.svc.store.save(marker{Power: &powerMark{Previous: "balanced", Applied: HighPerformanceScheme}}); err != nil {
		t.Fatal(err)
	}
	r.begin()
	if !r.markerExists() {
		t.Fatal("marker deleted although the restore failed")
	}
	if codes := r.errorCodes(); len(codes) != 1 || codes[0] != CodeRestoreFailed {
		t.Fatalf("codes = %v", codes)
	}
}

func TestCorruptMarkerStopsService(t *testing.T) {
	r := newRig(t)
	r.cfg = settings.GameMode{HighPerformance: true, HideLauncher: true}
	path := filepath.Join(r.dir, markerName)
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	r.begin()
	r.svc.SessionStarted(r.addGame("a", 10))
	r.svc.drain()
	if hides, _, _ := r.win.counts(); len(r.power.setCalls()) != 0 || hides != 0 {
		t.Fatal("service changed the system on top of an unreadable marker")
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "{not json" {
		t.Fatalf("corrupt marker was touched: %q, %v", data, err)
	}
	codes := r.errorCodes()
	if len(codes) != 2 || codes[0] != CodeMarkerCorrupt || codes[1] != CodeMarkerCorrupt {
		t.Fatalf("codes = %v, want the startup report and one per session", codes)
	}
	r.svc.SessionStopped("a")
	r.svc.drain()
	if err := r.svc.ServiceShutdown(); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(path)
	if err != nil || string(data) != "{not json" {
		t.Fatalf("corrupt marker did not survive the session and shutdown: %q, %v", data, err)
	}
	st := r.svc.Status()
	if !st.Blocked || st.Code != CodeMarkerCorrupt || st.Detail == "" {
		t.Fatalf("status = %+v", st)
	}
}

func TestCorruptMarkerStillAllowsPriority(t *testing.T) {
	r := newRig(t)
	r.cfg = settings.GameMode{HighPerformance: true, RaisePriority: true}
	if err := os.WriteFile(filepath.Join(r.dir, markerName), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	r.begin()
	r.svc.SessionStarted(r.addGame("a", 10))
	r.svc.drain()
	if r.prio.class(10) != AboveNormalPriorityClass {
		t.Fatal("priority needs no marker and must keep working")
	}
	if len(r.power.setCalls()) != 0 {
		t.Fatal("power plan changed without a usable marker")
	}
}

func TestShutdownRestoresRunningSession(t *testing.T) {
	r := newRig(t)
	r.cfg = settings.GameMode{HighPerformance: true, HideLauncher: true, RaisePriority: true}
	if err := r.svc.start(context.Background()); err != nil {
		t.Fatal(err)
	}
	r.svc.SessionStarted(r.addGame("a", 10))
	r.svc.drain()
	if r.prio.class(10) != AboveNormalPriorityClass {
		t.Fatal("priority not raised")
	}
	if err := r.svc.ServiceShutdown(); err != nil {
		t.Fatal(err)
	}
	_, shows, _ := r.win.counts()
	if r.power.current() != "balanced" || shows != 0 || r.prio.class(10) != NormalPriorityClass || r.markerExists() {
		t.Fatalf("not restored: power=%s shows=%d class=%#x marker=%v", r.power.current(), shows, r.prio.class(10), r.markerExists())
	}
}

func TestShutdownDoesNotWaitForWindowThread(t *testing.T) {
	r := newRig(t)
	r.cfg = settings.GameMode{HideLauncher: true}
	r.win.entered = make(chan struct{})
	r.win.release = make(chan struct{})
	if err := r.svc.start(context.Background()); err != nil {
		t.Fatal(err)
	}
	r.svc.SessionStarted(r.addGame("a", 10))
	<-r.win.entered
	done := make(chan error, 1)
	go func() { done <- r.svc.ServiceShutdown() }()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	close(r.win.release)
}

func TestOneToggleFailingDoesNotStopOthers(t *testing.T) {
	r := newRig(t)
	r.cfg = settings.GameMode{HighPerformance: true, HideLauncher: true, RaisePriority: true}
	r.power.setErr = errors.New("denied")
	r.begin()
	r.svc.SessionStarted(r.addGame("a", 10))
	r.svc.drain()
	if hides, _, _ := r.win.counts(); hides != 1 || r.prio.class(10) != AboveNormalPriorityClass {
		t.Fatal("hide or priority skipped after the power plan failed")
	}
	codes := r.errorCodes()
	if len(codes) != 1 || codes[0] != CodePowerFailed {
		t.Fatalf("codes = %v, want one power_failed", codes)
	}
	if r.markerExists() && r.svc.mark.Power != nil {
		t.Fatal("marker still records a power change that never happened")
	}
	r.svc.SessionStopped("a")
	r.svc.drain()
	if _, _, visible := r.win.counts(); !visible {
		t.Fatal("failed power plan must not block the restore of the others")
	}
}

func TestHideNeedsTrayIsReportedOncePerSession(t *testing.T) {
	r := newRig(t)
	r.cfg = settings.GameMode{HideLauncher: true, RaisePriority: true}
	r.win.noHide = true
	r.begin()
	r.svc.SessionStarted(r.addGame("a", 10))
	r.svc.drain()
	if hides, _, _ := r.win.counts(); hides != 0 {
		t.Fatal("window hidden without a tray icon")
	}
	if r.prio.class(10) != AboveNormalPriorityClass {
		t.Fatal("priority skipped because hide was refused")
	}
	r.mu.Lock()
	events := append([]ErrorInfo(nil), r.events...)
	r.mu.Unlock()
	if len(events) != 1 || events[0].Code != CodeHideNeedsTray || events[0].Toggle != "hide" || events[0].GameID != "a" {
		t.Fatalf("events = %+v, want one hide_needs_tray for game a", events)
	}
	r.svc.SessionStopped("a")
	r.svc.drain()
	if _, shows, _ := r.win.counts(); shows != 0 {
		t.Fatal("stop showed a window that was never hidden")
	}
}

func TestMarkerWithLegacyDNDFieldStillLoads(t *testing.T) {
	r := newRig(t)
	raw := `{"version":1,"data":{"power":{"previous":"balanced","applied":"` + HighPerformanceScheme + `"},"dnd":{"previous":0,"applied":1}}}`
	if err := os.WriteFile(filepath.Join(r.dir, markerName), []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	r.power.active = HighPerformanceScheme
	r.begin()
	if codes := r.errorCodes(); len(codes) != 0 {
		t.Fatalf("legacy marker rejected: %v", codes)
	}
	if got := r.power.setCalls(); len(got) != 1 || got[0] != "balanced" {
		t.Fatalf("power sets = %v, want restore to balanced", got)
	}
	if r.markerExists() {
		t.Fatal("marker left after recovery")
	}
}

func TestMissingPowerSchemeIsReported(t *testing.T) {
	r := newRig(t)
	r.cfg = settings.GameMode{HighPerformance: true}
	r.power.missing = true
	r.begin()
	r.svc.SessionStarted(r.addGame("a", 10))
	r.svc.drain()
	if codes := r.errorCodes(); len(codes) != 1 || codes[0] != CodePowerSchemeMissing {
		t.Fatalf("codes = %v", codes)
	}
	if r.markerExists() || len(r.power.setCalls()) != 0 {
		t.Fatal("nothing should change when the plan is missing")
	}
}

func TestPriorityDeniedIsReportedNotFatal(t *testing.T) {
	r := newRig(t)
	r.cfg = settings.GameMode{RaisePriority: true, HideLauncher: true}
	r.begin()
	g := r.addGame("a", 10)
	r.prio.denied[10] = true
	r.svc.SessionStarted(g)
	r.svc.drain()
	if codes := r.errorCodes(); len(codes) != 1 || codes[0] != CodePriorityFailed {
		t.Fatalf("codes = %v", codes)
	}
	if hides, _, _ := r.win.counts(); hides != 1 {
		t.Fatal("hide skipped after a priority failure")
	}
}

func TestPriorityRetryRaisesWrapperChildren(t *testing.T) {
	r := newRig(t)
	r.cfg = settings.GameMode{RaisePriority: true}
	r.begin()
	r.svc.SessionStarted(r.addGame("a", 10))
	r.svc.drain()
	r.prio.mu.Lock()
	r.prio.classes[11] = NormalPriorityClass
	r.prio.created[11] = epoch.Add(time.Second)
	r.prio.classes[12] = 0x80
	r.prio.created[12] = epoch.Add(time.Second)
	r.prio.mu.Unlock()
	r.children[10] = []uint32{11, 12}
	r.after <- time.Time{}
	r.svc.wg.Wait()
	r.svc.drain()
	if r.prio.class(11) != AboveNormalPriorityClass {
		t.Fatal("child spawned after the start was not raised")
	}
	if r.prio.class(12) != 0x80 {
		t.Fatal("a priority the user already chose must be left alone")
	}
}

func TestOverrideDisablesGlobalSwitch(t *testing.T) {
	r := newRig(t)
	r.cfg = settings.GameMode{HideLauncher: true, RaisePriority: true}
	r.begin()
	g := r.addGame("a", 10)
	g.GameMode = &library.GameModeOverride{HideLauncher: bp(false)}
	r.svc.SessionStarted(g)
	r.svc.drain()
	if hides, _, _ := r.win.counts(); hides != 0 || r.prio.class(10) != AboveNormalPriorityClass {
		t.Fatalf("hides=%d class=%#x", hides, r.prio.class(10))
	}
}

func TestInvisibleWindowIsNotShownAfterwards(t *testing.T) {
	r := newRig(t)
	r.cfg = settings.GameMode{HideLauncher: true}
	r.win.visible = false
	r.begin()
	r.svc.SessionStarted(r.addGame("a", 10))
	r.svc.drain()
	r.svc.SessionStopped("a")
	r.svc.drain()
	if hides, shows, _ := r.win.counts(); hides != 0 || shows != 0 {
		t.Fatalf("hides=%d shows=%d for a window that was already hidden", hides, shows)
	}
}

func TestConcurrentStartedAndStopped(t *testing.T) {
	r := newRig(t)
	r.cfg = settings.GameMode{HighPerformance: true, HideLauncher: true, RaisePriority: true}
	games := make([]library.Game, 8)
	for i := range games {
		games[i] = r.addGame(string(rune('a'+i)), uint32(100+i))
	}
	if err := r.svc.start(context.Background()); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for _, g := range games {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.svc.SessionStarted(g)
			r.svc.SessionStopped(g.ID)
		}()
	}
	wg.Wait()
	r.svc.drain()
	if err := r.svc.ServiceShutdown(); err != nil {
		t.Fatal(err)
	}
	_, _, visible := r.win.counts()
	if r.power.current() != "balanced" || !visible || r.markerExists() {
		t.Fatalf("state left changed: power=%s visible=%v marker=%v", r.power.current(), visible, r.markerExists())
	}
}

func TestShowSkippedWhenUserReopenedWindow(t *testing.T) {
	r := newRig(t)
	r.cfg = settings.GameMode{HideLauncher: true}
	r.begin()
	r.svc.SessionStarted(r.addGame("a", 10))
	r.svc.drain()
	r.win.setVisible(true)
	r.svc.SessionStopped("a")
	r.svc.drain()
	if hides, shows, _ := r.win.counts(); hides != 1 || shows != 0 {
		t.Fatalf("hides=%d shows=%d, want the window left alone after the user opened it", hides, shows)
	}
}

func TestMissingPreviousPlanIsDroppedWithOneNotice(t *testing.T) {
	r := newRig(t)
	r.power.active = HighPerformanceScheme
	r.power.setErr = errors.New("not found")
	r.power.gone = map[string]bool{"balanced": true}
	if err := r.svc.store.save(marker{Power: &powerMark{Previous: "balanced", Applied: HighPerformanceScheme}}); err != nil {
		t.Fatal(err)
	}
	r.begin()
	if r.markerExists() {
		t.Fatal("marker kept for a plan that no longer exists")
	}
	if codes := r.errorCodes(); len(codes) != 1 || codes[0] != CodeRestoreFailed {
		t.Fatalf("codes = %v, want one restore_failed", codes)
	}
}

func TestPriorityFailuresAreOneEventPerSession(t *testing.T) {
	r := newRig(t)
	r.cfg = settings.GameMode{RaisePriority: true}
	r.begin()
	r.svc.SessionStarted(r.addGame("a", 10))
	r.svc.drain()
	r.prio.spawn(11, NormalPriorityClass, epoch.Add(time.Second))
	r.prio.spawn(12, NormalPriorityClass, epoch.Add(time.Second))
	r.prio.denied[11], r.prio.denied[12] = true, true
	r.children[10] = []uint32{11, 12}
	r.after <- time.Time{}
	r.svc.wg.Wait()
	r.svc.drain()
	if codes := r.errorCodes(); len(codes) != 1 || codes[0] != CodePriorityFailed {
		t.Fatalf("codes = %v, want a single priority_failed for the session", codes)
	}
}

func TestPriorityIgnoresReusedAndOlderPIDs(t *testing.T) {
	r := newRig(t)
	r.cfg = settings.GameMode{RaisePriority: true}
	r.begin()
	r.svc.SessionStarted(r.addGame("a", 10))
	r.svc.drain()
	r.prio.spawn(11, NormalPriorityClass, epoch.Add(-time.Hour))
	r.children[10] = []uint32{11}
	r.after <- time.Time{}
	r.svc.wg.Wait()
	r.svc.drain()
	if r.prio.class(11) != NormalPriorityClass {
		t.Fatal("a process older than the game must not be raised")
	}

	r.prio.spawn(10, NormalPriorityClass, epoch.Add(time.Hour))
	r.svc.enqueue(event{retry: true, gameID: "a"})
	r.svc.drain()
	if r.prio.class(10) != NormalPriorityClass {
		t.Fatal("a reused root pid must not be touched")
	}
}

func TestStopRestoresSurvivingDescendantsOnly(t *testing.T) {
	r := newRig(t)
	r.cfg = settings.GameMode{RaisePriority: true}
	r.begin()
	r.svc.SessionStarted(r.addGame("a", 10))
	r.prio.spawn(11, NormalPriorityClass, epoch.Add(time.Second))
	r.prio.spawn(12, NormalPriorityClass, epoch.Add(time.Second))
	r.children[10] = []uint32{11, 12}
	r.svc.drain()
	if r.prio.class(11) != AboveNormalPriorityClass || r.prio.class(12) != AboveNormalPriorityClass {
		t.Fatal("children not raised at start")
	}
	r.prio.spawn(12, NormalPriorityClass, epoch.Add(time.Hour))
	r.svc.SessionStopped("a")
	r.svc.drain()
	if r.prio.class(11) != NormalPriorityClass {
		t.Fatal("surviving descendant not restored")
	}
	if r.prio.class(12) != NormalPriorityClass {
		t.Fatal("test setup: reused pid should stay as the new process had it")
	}
	if errs := r.errorCodes(); len(errs) != 0 {
		t.Fatalf("unexpected events %v", errs)
	}
}
