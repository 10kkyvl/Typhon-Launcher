package selfupdate

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stubInstanceLock decides whether another launcher already holds the
// instance lock, and turns the guard on for platforms that leave it off.
func stubInstanceLock(t *testing.T, taken bool) *[]string {
	t.Helper()
	var asked []string
	restoreAcquire, restoreSupported := acquireInstance, startGuardSupported
	acquireInstance = func(name string) (bool, error) {
		asked = append(asked, name)
		return !taken, nil
	}
	startGuardSupported = true
	t.Cleanup(func() { acquireInstance, startGuardSupported = restoreAcquire, restoreSupported })
	return &asked
}

func markerOf(t *testing.T, configDir string) (startupMarker, bool) {
	t.Helper()
	cache, err := CacheDir(configDir)
	if err != nil {
		t.Fatal(err)
	}
	m, err := readStartupMarker(filepath.Join(cache, startupMarkerName))
	if err != nil {
		t.Fatalf("read marker: %v", err)
	}
	return m, m.Version != ""
}

func plantMarker(t *testing.T, configDir string, m startupMarker) {
	t.Helper()
	cache, err := CacheDir(configDir)
	if err != nil {
		t.Fatal(err)
	}
	g := &StartGuard{path: filepath.Join(cache, startupMarkerName)}
	g.write(m)
}

func TestBeginStartCountsStartsOfOneVersionInARow(t *testing.T) {
	stubInstanceLock(t, false)
	cfg := filepath.Join(t.TempDir(), "Typhon")

	var last *StartGuard
	for want := 1; want <= 4; want++ {
		last = BeginStart(cfg, "1.3.0")
		if got := last.Attempts(); got != want {
			t.Fatalf("start %d: Attempts = %d", want, got)
		}
		m, ok := markerOf(t, cfg)
		if !ok || m.Version != "1.3.0" || m.Attempts != want {
			t.Fatalf("start %d: marker = %+v (%v)", want, m, ok)
		}
	}
	last.Succeeded()
	if _, ok := markerOf(t, cfg); ok {
		t.Fatal("a successful start left the marker behind")
	}
	if got := BeginStart(cfg, "1.3.0").Attempts(); got != 1 {
		t.Fatalf("the start after a success counts %d, want 1", got)
	}
}

func TestBeginStartOfAnotherVersionStartsOver(t *testing.T) {
	stubInstanceLock(t, false)
	cfg := filepath.Join(t.TempDir(), "Typhon")
	plantMarker(t, cfg, startupMarker{Version: "1.3.0", Attempts: 5})

	g := BeginStart(cfg, "1.2.0")
	if g.Attempts() != 1 {
		t.Fatalf("Attempts = %d: failed starts of another version say nothing about this one", g.Attempts())
	}
	if m, _ := markerOf(t, cfg); m.Version != "1.2.0" || m.Attempts != 1 {
		t.Fatalf("marker = %+v", m)
	}
}

func TestBeginStartIgnoresASecondCopyOfTheLauncher(t *testing.T) {
	cases := []struct {
		name      string
		taken     bool
		wantCount int
	}{
		{name: "another launcher holds the lock", taken: true, wantCount: 0},
		{name: "the lock is free", taken: false, wantCount: 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			asked := stubInstanceLock(t, tc.taken)
			cfg := filepath.Join(t.TempDir(), "Typhon")
			plantMarker(t, cfg, startupMarker{Version: "1.3.0", Attempts: 1})

			g := BeginStart(cfg, "1.3.0")
			if g.Attempts() != tc.wantCount {
				t.Fatalf("Attempts = %d, want %d", g.Attempts(), tc.wantCount)
			}
			g.Succeeded()
			if tc.taken {
				g.Reset()
			}
			m, present := markerOf(t, cfg)
			if tc.taken && (!present || m.Attempts != 1) {
				t.Fatalf("a second copy of the launcher changed the marker of the one that is starting: %+v (%v)", m, present)
			}
			if !tc.taken && present {
				t.Fatalf("the counted start did not clear the marker on success: %+v", m)
			}
			if len(*asked) != 1 || !strings.HasPrefix((*asked)[0], "typhon-start-guard-") {
				t.Fatalf("lock names asked for = %v", *asked)
			}
		})
	}
}

func TestInstanceLockNameIsPerDataDirectory(t *testing.T) {
	a := instanceLockName(filepath.Join(t.TempDir(), "Typhon"))
	b := instanceLockName(filepath.Join(t.TempDir(), "Typhon"))
	if a == b {
		t.Fatal("two data directories share one lock: a portable copy would block the installed launcher")
	}
	cfg := filepath.Join(t.TempDir(), "Typhon")
	if instanceLockName(cfg) != instanceLockName(filepath.Join(cfg, "..", "Typhon")) {
		t.Fatal("the same directory written two ways gets two locks")
	}
	if strings.ContainsAny(a, `\/`) {
		t.Fatalf("lock name %q contains a separator: the system treats it as a namespace", a)
	}
}

func TestBeginStartSurvivesAMarkerItCannotUse(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"garbage", "{{{ not json"},
		{"empty file", ""},
		{"written by a newer launcher", `{"version":99,"data":{"version":"1.3.0","attempts":7}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stubInstanceLock(t, false)
			cfg := filepath.Join(t.TempDir(), "Typhon")
			cache := mustCacheDir(t, cfg)
			writeTestFile(t, filepath.Join(cache, startupMarkerName), []byte(tc.body))

			g := BeginStart(cfg, "1.3.0")
			if g.Attempts() != 1 {
				t.Fatalf("Attempts = %d, want a fresh count of 1", g.Attempts())
			}
			if m, ok := markerOf(t, cfg); !ok || m.Attempts != 1 {
				t.Fatalf("marker = %+v (%v), want it rewritten", m, ok)
			}
		})
	}
}

func TestBeginStartNeverStopsTheStart(t *testing.T) {
	stubInstanceLock(t, false)
	cfg := filepath.Join(t.TempDir(), "Typhon")
	cache := mustCacheDir(t, cfg)
	// A file where the cache directory should be: nothing can be written.
	writeTestFile(t, cache, []byte("in the way"))

	g := BeginStart(cfg, "1.3.0")
	if g.Attempts() != 1 {
		t.Fatalf("Attempts = %d", g.Attempts())
	}
	g.Succeeded()
	g.Reset()
	if _, err := os.Stat(cache); err != nil {
		t.Fatalf("the guard disturbed what was in its way: %v", err)
	}

	if got := BeginStart("", "1.3.0"); got.Attempts() != 0 {
		t.Fatalf("a guard without a config directory counts %d", got.Attempts())
	}
}

func TestBeginStartOnAPlatformWithoutRollbackDoesNothing(t *testing.T) {
	stubInstanceLock(t, false)
	startGuardSupported = false
	cfg := filepath.Join(t.TempDir(), "Typhon")

	g := BeginStart(cfg, "1.3.0")
	g.Succeeded()
	g.Reset()
	if g.Attempts() != 0 {
		t.Fatalf("Attempts = %d", g.Attempts())
	}
	if _, err := os.Stat(cfg); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("an inert guard created %s: %v", cfg, err)
	}
}

func TestResetForgetsTheFailedStarts(t *testing.T) {
	stubInstanceLock(t, false)
	cfg := filepath.Join(t.TempDir(), "Typhon")
	plantMarker(t, cfg, startupMarker{Version: "1.3.0", Attempts: 2})

	g := BeginStart(cfg, "1.3.0")
	if g.Attempts() != 3 {
		t.Fatalf("Attempts = %d", g.Attempts())
	}
	g.Reset()
	if g.Attempts() != 0 {
		t.Fatalf("Attempts after Reset = %d", g.Attempts())
	}
	if m, _ := markerOf(t, cfg); m.Attempts != 0 || m.Version != "1.3.0" {
		t.Fatalf("marker = %+v", m)
	}
	if got := BeginStart(cfg, "1.3.0"); got.Attempts() != 1 {
		t.Fatalf("the start after a refusal counts %d, want 1", got.Attempts())
	}
}

type rollbackOffer struct {
	asked       []string
	askTitle    string
	answer      bool
	askErr      error
	errorsShown []string
	workers     int
	specPath    string
}

// stubOffer wires everything OfferRollback touches outside the files: the
// question, the error box, the worker start, the executable and the language.
func stubOffer(t *testing.T, exe string) *rollbackOffer {
	t.Helper()
	o := &rollbackOffer{}
	stubRollbackSupported(t, nil)
	restoreAsk, restoreShow, restoreWorker, restoreExe, restoreLang := askYesNo, showError, startWorker, currentExecutable, systemLanguage
	askYesNo = func(title, text string) (bool, error) {
		o.askTitle = title
		o.asked = append(o.asked, text)
		return o.answer, o.askErr
	}
	showError = func(_, text string) { o.errorsShown = append(o.errorsShown, text) }
	startWorker = func(_, spec string) error {
		o.workers++
		o.specPath = spec
		return nil
	}
	currentExecutable = func() (string, error) { return exe, nil }
	systemLanguage = func() string { return "en" }
	t.Cleanup(func() {
		askYesNo, showError, startWorker, currentExecutable, systemLanguage = restoreAsk, restoreShow, restoreWorker, restoreExe, restoreLang
	})
	return o
}

func failedStarts(t *testing.T, cfg string, n int) *StartGuard {
	t.Helper()
	plantMarker(t, cfg, startupMarker{Version: "1.3.0", Attempts: n - 1})
	g := BeginStart(cfg, "1.3.0")
	if g.Attempts() != n {
		t.Fatalf("setup: Attempts = %d, want %d", g.Attempts(), n)
	}
	return g
}

func TestOfferRollbackAfterTwoFailedStarts(t *testing.T) {
	stubInstanceLock(t, false)
	f := newRollbackFixture(t)
	o := stubOffer(t, f.target)
	o.answer = true

	g := failedStarts(t, f.configDir, 3)
	started, err := g.OfferRollback(false)
	if err != nil || !started {
		t.Fatalf("OfferRollback = %v, %v", started, err)
	}
	if len(o.asked) != 1 || !strings.Contains(o.asked[0], "1.2.0") || !strings.Contains(o.asked[0], "failed to start twice") {
		t.Fatalf("question = %q", o.asked)
	}
	if o.workers != 1 {
		t.Fatalf("workers started = %d", o.workers)
	}
	spec, err := readUpdateSpec(o.specPath)
	if err != nil {
		t.Fatal(err)
	}
	if spec.Kind != specKindRollback || spec.Version != "1.2.0" || spec.FromVersion != "1.3.0" || spec.RelaunchPath != f.target || spec.ParentPID != os.Getpid() || spec.Language != "en" {
		t.Fatalf("spec = %+v", spec)
	}
}

func TestOfferRollbackAsksInTheLanguageOfTheSettings(t *testing.T) {
	cases := []struct {
		name     string
		settings string
		system   string
		want     string
	}{
		{"russian in settings", `{"language":"ru"}`, "en", "не смог запуститься"},
		{"english in settings", `{"language":"en"}`, "ru", "failed to start twice"},
		{"system follows the OS, russian", `{"language":"system"}`, "ru", "не смог запуститься"},
		{"system follows the OS, english", `{"language":"system"}`, "en", "failed to start twice"},
		{"no settings file follows the OS", ``, "ru", "не смог запуститься"},
		{"unreadable settings follow the OS", `{broken`, "en", "failed to start twice"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stubInstanceLock(t, false)
			f := newRollbackFixture(t)
			o := stubOffer(t, f.target)
			systemLanguage = func() string { return tc.system }
			if tc.settings != "" {
				writeTestFile(t, filepath.Join(f.configDir, "settings.json"), []byte(tc.settings))
			}

			g := failedStarts(t, f.configDir, 3)
			if _, err := g.OfferRollback(false); err != nil {
				t.Fatalf("OfferRollback: %v", err)
			}
			if len(o.asked) != 1 || !strings.Contains(o.asked[0], tc.want) {
				t.Fatalf("question = %q, want it to contain %q", o.asked, tc.want)
			}
		})
	}
}

func TestOfferRollbackDeclinedKeepsThisVersion(t *testing.T) {
	stubInstanceLock(t, false)
	f := newRollbackFixture(t)
	o := stubOffer(t, f.target)
	o.answer = false

	g := failedStarts(t, f.configDir, 3)
	started, err := g.OfferRollback(false)
	if err != nil || started {
		t.Fatalf("OfferRollback = %v, %v", started, err)
	}
	if o.workers != 0 {
		t.Fatalf("a refusal started %d workers", o.workers)
	}
	if g.Attempts() != 0 {
		t.Fatalf("Attempts = %d, a refusal resets the count", g.Attempts())
	}
	if m, _ := markerOf(t, f.configDir); m.Attempts != 0 {
		t.Fatalf("marker = %+v", m)
	}
	if _, err := loadPrevious(f.configDir); err != nil {
		t.Fatalf("a refusal dropped the snapshot: %v", err)
	}
}

func TestOfferRollbackStaysQuietWhenThereIsNothingToOffer(t *testing.T) {
	cases := []struct {
		name     string
		attempts int
		prepare  func(t *testing.T, f rollbackFixture)
	}{
		{name: "two starts are not enough", attempts: 2},
		{name: "first start", attempts: 1},
		{name: "no snapshot", attempts: 3, prepare: func(t *testing.T, f rollbackFixture) {
			if err := dropPrevious(f.configDir); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "damaged snapshot", attempts: 4, prepare: func(t *testing.T, f rollbackFixture) {
			dir := mustPreviousDir(t, f.configDir)
			writeTestFile(t, filepath.Join(dir, previousExeName), []byte("x"))
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stubInstanceLock(t, false)
			f := newRollbackFixture(t)
			o := stubOffer(t, f.target)
			o.answer = true
			if tc.prepare != nil {
				tc.prepare(t, f)
			}

			g := failedStarts(t, f.configDir, tc.attempts)
			started, err := g.OfferRollback(false)
			if err != nil || started {
				t.Fatalf("OfferRollback = %v, %v", started, err)
			}
			if len(o.asked) != 0 || o.workers != 0 {
				t.Fatalf("asked %d times, started %d workers", len(o.asked), o.workers)
			}
		})
	}
}

func TestOfferRollbackReportsWhatWentWrong(t *testing.T) {
	t.Run("the question cannot be shown", func(t *testing.T) {
		stubInstanceLock(t, false)
		f := newRollbackFixture(t)
		o := stubOffer(t, f.target)
		o.askErr = errors.New("no desktop")

		started, err := failedStarts(t, f.configDir, 3).OfferRollback(false)
		if started || !errors.Is(err, o.askErr) {
			t.Fatalf("OfferRollback = %v, %v", started, err)
		}
		if o.workers != 0 {
			t.Fatal("a rollback started without an answer")
		}
	})

	t.Run("the worker cannot be started", func(t *testing.T) {
		stubInstanceLock(t, false)
		f := newRollbackFixture(t)
		o := stubOffer(t, f.target)
		o.answer = true
		errStart := errors.New("process creation refused")
		startWorker = func(string, string) error { return errStart }

		started, err := failedStarts(t, f.configDir, 3).OfferRollback(false)
		if started || !errors.Is(err, errStart) {
			t.Fatalf("OfferRollback = %v, %v", started, err)
		}
		if len(o.errorsShown) != 1 || !strings.Contains(o.errorsShown[0], "process creation refused") {
			t.Fatalf("the user was told %q", o.errorsShown)
		}
	})
}
