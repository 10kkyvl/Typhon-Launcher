package install

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func useSharedRoots(t *testing.T, roots ...string) {
	t.Helper()
	previous := sharedRootsFn
	sharedRootsFn = func() ([]string, error) { return roots, nil }
	t.Cleanup(func() { sharedRootsFn = previous })
}

func backdate(t *testing.T, paths ...string) {
	t.Helper()
	old := time.Now().Add(-time.Hour)
	for _, path := range paths {
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatalf("chtimes %s: %v", path, err)
		}
	}
}

func jobFor(t *testing.T, root string, game bool) shellJob {
	t.Helper()
	before, err := takeShellSnapshot(context.Background(), []string{root})
	if err != nil {
		t.Fatalf("takeShellSnapshot error = %v", err)
	}
	handoff, err := newShellHandoff(before, game)
	if err != nil || handoff == nil {
		t.Fatalf("newShellHandoff = (%v, %v), want a handoff", handoff, err)
	}
	return handoff.Job
}

func TestShellKeyInside(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Shared")
	sep := string(filepath.Separator)
	lower := strings.ToLower(root)
	cases := []struct {
		name  string
		roots []string
		key   string
		want  bool
	}{
		{name: "файл в корне", roots: []string{root}, key: lower + sep + "a.lnk", want: true},
		{name: "вложенный файл", roots: []string{root}, key: lower + sep + "dir" + sep + "a.lnk", want: true},
		{name: "сам корень", roots: []string{root}, key: lower, want: false},
		{name: "сосед с общим префиксом", roots: []string{root}, key: lower + "x" + sep + "a.lnk", want: false},
		{name: "точки в пути", roots: []string{root}, key: lower + sep + ".." + sep + "other" + sep + "a.lnk", want: false},
		{name: "относительный путь", roots: []string{root}, key: "a.lnk", want: false},
		{name: "пустой ключ", roots: []string{root}, key: "", want: false},
		{name: "нет корней", roots: nil, key: lower + sep + "a.lnk", want: false},
		{name: "пустой корень", roots: []string{""}, key: lower + sep + "a.lnk", want: false},
		{name: "второй корень", roots: []string{filepath.Join(filepath.Dir(root), "Other"), root}, key: lower + sep + "a.lnk", want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shellKeyInside(tc.roots, tc.key); got != tc.want {
				t.Fatalf("shellKeyInside(%q) = %v, want %v", tc.key, got, tc.want)
			}
		})
	}
}

func TestShellJobSnapshot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "shared")
	inRoot := strings.ToLower(filepath.Join(root, "a.lnk"))
	outside := strings.ToLower(filepath.Join(filepath.Dir(root), "elsewhere", "a.lnk"))

	t.Run("записи внутри корней принимаются", func(t *testing.T) {
		job := shellJob{Before: map[string]shellEntryWire{inRoot: {Size: 7, Mod: 42}}}
		snap, err := job.snapshot([]string{root})
		if err != nil {
			t.Fatalf("snapshot error = %v", err)
		}
		entry, ok := snap.entries[inRoot]
		if !snap.taken || !ok || entry.size != 7 || !entry.mod.Equal(time.Unix(0, 42)) {
			t.Fatalf("snapshot = %+v", snap)
		}
	})
	t.Run("запись вне корней отвергает весь снимок", func(t *testing.T) {
		job := shellJob{Before: map[string]shellEntryWire{inRoot: {}, outside: {}}}
		if _, err := job.snapshot([]string{root}); !errors.Is(err, errShellOutsideRoots) {
			t.Fatalf("snapshot error = %v, want errShellOutsideRoots", err)
		}
	})
	t.Run("слишком большой снимок", func(t *testing.T) {
		job := shellJob{Before: make(map[string]shellEntryWire, shortcutMaxEntries+1)}
		for i := 0; i <= shortcutMaxEntries; i++ {
			job.Before[strings.ToLower(filepath.Join(root, fmt.Sprintf("%d.lnk", i)))] = shellEntryWire{}
		}
		if _, err := job.snapshot([]string{root}); !errors.Is(err, errShellTooLarge) {
			t.Fatalf("snapshot error = %v, want errShellTooLarge", err)
		}
	})
	t.Run("пустой снимок допустим", func(t *testing.T) {
		snap, err := shellJob{}.snapshot([]string{root})
		if err != nil || !snap.taken || len(snap.entries) != 0 {
			t.Fatalf("snapshot = (%+v, %v), want taken and empty", snap, err)
		}
	})
}

func TestNewShellHandoff(t *testing.T) {
	user := t.TempDir()
	shared := t.TempDir()
	writeFile(t, filepath.Join(user, "mine.url"), siteURL)
	writeFile(t, filepath.Join(shared, "theirs.url"), siteURL)
	before, err := takeShellSnapshot(context.Background(), []string{user, shared})
	if err != nil {
		t.Fatalf("takeShellSnapshot error = %v", err)
	}

	t.Run("в задание попадает только общий каталог", func(t *testing.T) {
		useSharedRoots(t, shared)
		handoff, err := newShellHandoff(before, true)
		if err != nil || handoff == nil {
			t.Fatalf("newShellHandoff = (%v, %v)", handoff, err)
		}
		if !handoff.Job.Game || len(handoff.Job.Before) != 1 {
			t.Fatalf("job = %+v, want game and one shared entry", handoff.Job)
		}
		key := strings.ToLower(filepath.Join(shared, "theirs.url"))
		wire, ok := handoff.Job.Before[key]
		want := before.entries[key]
		if !ok || wire.Size != want.size || wire.Mod != want.mod.UnixNano() {
			t.Fatalf("job.Before[%q] = %+v, want size %d mod %d", key, wire, want.size, want.mod.UnixNano())
		}
	})
	t.Run("снимок без записей даёт пустое, но живое задание", func(t *testing.T) {
		useSharedRoots(t, shared)
		empty, err := takeShellSnapshot(context.Background(), []string{t.TempDir()})
		if err != nil {
			t.Fatalf("takeShellSnapshot error = %v", err)
		}
		handoff, err := newShellHandoff(empty, false)
		if err != nil || handoff == nil || handoff.Job.Before == nil || len(handoff.Job.Before) != 0 {
			t.Fatalf("newShellHandoff = (%+v, %v), want an empty job", handoff, err)
		}
	})
	t.Run("снимок не взят", func(t *testing.T) {
		useSharedRoots(t, shared)
		handoff, err := newShellHandoff(shellSnapshot{}, false)
		if err != nil || handoff != nil {
			t.Fatalf("newShellHandoff = (%v, %v), want nothing", handoff, err)
		}
	})
	t.Run("общих каталогов нет", func(t *testing.T) {
		useSharedRoots(t)
		handoff, err := newShellHandoff(before, false)
		if err != nil || handoff != nil {
			t.Fatalf("newShellHandoff = (%v, %v), want nothing", handoff, err)
		}
	})
	t.Run("ошибка определения каталогов не молчит", func(t *testing.T) {
		failure := errors.New("known folder недоступна")
		previous := sharedRootsFn
		sharedRootsFn = func() ([]string, error) { return nil, failure }
		t.Cleanup(func() { sharedRootsFn = previous })
		if _, err := newShellHandoff(before, false); !errors.Is(err, failure) {
			t.Fatalf("newShellHandoff error = %v, want %v", err, failure)
		}
	})
}

func TestShellSnapshotWithout(t *testing.T) {
	user := t.TempDir()
	shared := t.TempDir()
	writeFile(t, filepath.Join(user, "mine.url"), siteURL)
	writeFile(t, filepath.Join(shared, "theirs.url"), siteURL)
	before, err := takeShellSnapshot(context.Background(), []string{user, shared})
	if err != nil {
		t.Fatalf("takeShellSnapshot error = %v", err)
	}

	got := before.without([]string{shared})
	if !got.taken || len(got.roots) != 1 || !samePath(got.roots[0], user) {
		t.Fatalf("without: roots = %v taken = %v, want only %s", got.roots, got.taken, user)
	}
	if _, ok := got.entries[strings.ToLower(filepath.Join(shared, "theirs.url"))]; ok {
		t.Fatal("запись общего каталога осталась в снимке лаунчера")
	}
	if _, ok := got.entries[strings.ToLower(filepath.Join(user, "mine.url"))]; !ok {
		t.Fatal("запись своего каталога потеряна")
	}
	if len(before.roots) != 2 || len(before.entries) != 2 {
		t.Fatalf("without изменил исходный снимок: %+v", before)
	}
}

func TestCleanSharedShortcuts(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "gta")

	t.Run("убирает новый ярлык сайта и не трогает старые и чужие", func(t *testing.T) {
		shared := t.TempDir()
		useSharedRoots(t, shared)
		mine := filepath.Join(shared, "mine.url")
		writeFile(t, mine, siteURL)
		backdate(t, mine)
		job := jobFor(t, shared, false)

		site := filepath.Join(shared, "ti.lnk")
		writeShortcut(t, site, `C:\Program Files (x86)\TI\TI.URL`)
		foreign := filepath.Join(shared, "browser.lnk")
		writeShortcut(t, foreign, `C:\Program Files\Browser\browser.exe`)

		report := cleanSharedShortcuts(context.Background(), job, dest)
		if report.Error != "" || len(report.Removed) != 1 {
			t.Fatalf("report = %+v, want one removal and no error", report)
		}
		if exists(site) {
			t.Fatal("ярлык сайта репака остался")
		}
		for _, path := range []string{mine, foreign} {
			if !exists(path) {
				t.Fatalf("%s удалён", path)
			}
		}
	})

	t.Run("ярлык игры убирается только при game", func(t *testing.T) {
		for _, game := range []bool{false, true} {
			shared := t.TempDir()
			useSharedRoots(t, shared)
			job := jobFor(t, shared, game)
			link := filepath.Join(shared, "gta.lnk")
			writeShortcut(t, link, filepath.Join(dest, "gta.exe"))

			report := cleanSharedShortcuts(context.Background(), job, dest)
			if report.Error != "" {
				t.Fatalf("game=%v: report = %+v", game, report)
			}
			if exists(link) == game {
				t.Fatalf("game=%v: ярлык игры существует = %v", game, exists(link))
			}
		}
	})

	t.Run("запись снимка вне общих каталогов: ничего не удаляется", func(t *testing.T) {
		shared := t.TempDir()
		useSharedRoots(t, shared)
		job := jobFor(t, shared, true)
		job.Before[strings.ToLower(filepath.Join(t.TempDir(), "evil.url"))] = shellEntryWire{}
		site := filepath.Join(shared, "ti.lnk")
		writeShortcut(t, site, `C:\Program Files (x86)\TI\TI.URL`)

		report := cleanSharedShortcuts(context.Background(), job, dest)
		if !strings.Contains(report.Error, errShellOutsideRoots.Error()) || len(report.Removed) != 0 {
			t.Fatalf("report = %+v, want errShellOutsideRoots and no removals", report)
		}
		if !exists(site) {
			t.Fatal("ярлык удалён по снимку, не прошедшему проверку")
		}
	})

	t.Run("ярлыки вне общих каталогов воркер не видит", func(t *testing.T) {
		shared := t.TempDir()
		elsewhere := t.TempDir()
		useSharedRoots(t, shared)
		job := jobFor(t, shared, true)
		site := filepath.Join(elsewhere, "ti.lnk")
		writeShortcut(t, site, `C:\Program Files (x86)\TI\TI.URL`)

		report := cleanSharedShortcuts(context.Background(), job, dest)
		if report.Error != "" || len(report.Removed) != 0 || !exists(site) {
			t.Fatalf("report = %+v, site exists = %v", report, exists(site))
		}
	})

	t.Run("пустой снимок: всё найденное считается новым", func(t *testing.T) {
		shared := t.TempDir()
		useSharedRoots(t, shared)
		site := filepath.Join(shared, "ti.lnk")
		writeShortcut(t, site, `C:\Program Files (x86)\TI\TI.URL`)

		report := cleanSharedShortcuts(context.Background(), shellJob{Before: map[string]shellEntryWire{}}, "")
		if report.Error != "" || len(report.Removed) != 1 || exists(site) {
			t.Fatalf("report = %+v, site exists = %v", report, exists(site))
		}
	})

	t.Run("общих каталогов нет", func(t *testing.T) {
		useSharedRoots(t)
		report := cleanSharedShortcuts(context.Background(), shellJob{}, "")
		if !strings.Contains(report.Error, errShellNoRoots.Error()) {
			t.Fatalf("report = %+v, want errShellNoRoots", report)
		}
	})

	t.Run("ошибка определения каталогов", func(t *testing.T) {
		previous := sharedRootsFn
		sharedRootsFn = func() ([]string, error) { return nil, errors.New("known folder недоступна") }
		t.Cleanup(func() { sharedRootsFn = previous })
		report := cleanSharedShortcuts(context.Background(), shellJob{}, "")
		if !strings.Contains(report.Error, "known folder недоступна") {
			t.Fatalf("report = %+v, want the resolve error", report)
		}
	})

	t.Run("относительная цель отвергается", func(t *testing.T) {
		shared := t.TempDir()
		useSharedRoots(t, shared)
		site := filepath.Join(shared, "ti.lnk")
		writeShortcut(t, site, `C:\Program Files (x86)\TI\TI.URL`)
		report := cleanSharedShortcuts(context.Background(), shellJob{Before: map[string]shellEntryWire{}}, `games\gta`)
		if !strings.Contains(report.Error, errShellBadTarget.Error()) || !exists(site) {
			t.Fatalf("report = %+v, site exists = %v", report, exists(site))
		}
	})

	t.Run("отменённый ctx", func(t *testing.T) {
		shared := t.TempDir()
		useSharedRoots(t, shared)
		job := jobFor(t, shared, true)
		site := filepath.Join(shared, "ti.lnk")
		writeShortcut(t, site, `C:\Program Files (x86)\TI\TI.URL`)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		report := cleanSharedShortcuts(ctx, job, dest)
		if !strings.Contains(report.Error, context.Canceled.Error()) || !exists(site) {
			t.Fatalf("report = %+v, site exists = %v", report, exists(site))
		}
	})

	t.Run("ошибка удаления одного ярлыка не прячет остальные", func(t *testing.T) {
		if runtime.GOOS != "windows" && os.Geteuid() == 0 {
			t.Skip("запущено от root: права каталога не мешают удалению")
		}
		shared := t.TempDir()
		useSharedRoots(t, shared)
		job := jobFor(t, shared, false)
		removable := filepath.Join(shared, "removable.lnk")
		writeShortcut(t, removable, `C:\Program Files (x86)\TI\TI.URL`)
		locked := filepath.Join(shared, "locked", "ti.lnk")
		if err := os.MkdirAll(filepath.Dir(locked), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(locked), err)
		}
		writeShortcut(t, locked, `C:\Program Files (x86)\TI\TI.URL`)
		release := lockForDelete(t, locked)
		defer release()

		report := cleanSharedShortcuts(context.Background(), job, dest)
		if report.Error == "" {
			t.Fatalf("report = %+v, want the delete error", report)
		}
		if len(report.Removed) != 1 || exists(removable) {
			t.Fatalf("report = %+v, removable exists = %v", report, exists(removable))
		}
		if !exists(locked) {
			t.Fatal("заблокированный ярлык исчез")
		}
	})
}

// lockForDelete делает файл неудаляемым средствами ОС: на Windows открытый
// хэндл без FILE_SHARE_DELETE, на остальных — каталог без права записи.
func lockForDelete(t *testing.T, path string) func() {
	t.Helper()
	if runtime.GOOS == "windows" {
		f, err := os.Open(path)
		if err != nil {
			t.Fatalf("open %s: %v", path, err)
		}
		return func() {
			if err := f.Close(); err != nil {
				t.Errorf("close %s: %v", path, err)
			}
		}
	}
	dir := filepath.Dir(path)
	if err := os.Chmod(dir, 0o500); err != nil { //nolint:gosec // G302: тест инварианта 5 требует каталог без права записи; 0600 снял бы execute-бит каталога
		t.Fatalf("chmod %s: %v", dir, err)
	}
	return func() {
		if err := os.Chmod(dir, 0o700); err != nil { //nolint:gosec // G302: возврат прав, выставленных выше по той же причине
			t.Errorf("chmod %s: %v", dir, err)
		}
	}
}

func TestRunWorkerSpecCleansSharedShortcuts(t *testing.T) {
	site := func(root string) string { return filepath.Join(root, "ti.lnk") }
	install := func(t *testing.T, root string, err error) func(context.Context, workerSpec, []string) (int, error) {
		return func(context.Context, workerSpec, []string) (int, error) {
			writeShortcut(t, site(root), `C:\Program Files (x86)\TI\TI.URL`)
			return 0, err
		}
	}
	cases := []struct {
		name        string
		installErr  error
		withJob     bool
		tamper      bool
		wantRemoved int
		wantReport  bool
		wantErr     string
		wantGone    bool
	}{
		{name: "установка прошла: воркер убирает и пишет итог", withJob: true, wantRemoved: 1, wantReport: true, wantGone: true},
		{name: "установка упала: ярлыки не трогаем", installErr: errors.New("boom"), withJob: true, wantErr: "boom"},
		{name: "уборку не просили", withJob: false},
		{name: "снимок подменён: итог с ошибкой, установка не падает", withJob: true, tamper: true, wantReport: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			shared := t.TempDir()
			useSharedRoots(t, shared)
			job := jobFor(t, shared, false)
			if tc.tamper {
				job.Before[strings.ToLower(filepath.Join(t.TempDir(), "evil.url"))] = shellEntryWire{}
			}
			dir := t.TempDir()
			spec := workerSpec{ID: "w1", Run: "r1", StatePath: filepath.Join(dir, "state.json"), Destination: filepath.Join(dir, "game")}
			if tc.withJob {
				spec.Shell = &job
			}
			previous := workerInstall
			workerInstall = install(t, shared, tc.installErr)
			t.Cleanup(func() { workerInstall = previous })

			runErr := runWorkerSpec(spec)
			if (runErr != nil) != (tc.installErr != nil) {
				t.Fatalf("runWorkerSpec error = %v, want %v", runErr, tc.installErr)
			}
			state, found, err := readWorkerState(spec.StatePath)
			if err != nil || !found || !state.Done || state.Run != "r1" || state.Error != tc.wantErr {
				t.Fatalf("state = %+v found = %v err = %v, want done run r1 error %q", state, found, err, tc.wantErr)
			}
			if (state.Shell != nil) != tc.wantReport {
				t.Fatalf("state.Shell = %+v, want report = %v", state.Shell, tc.wantReport)
			}
			if state.Shell != nil && len(state.Shell.Removed) != tc.wantRemoved {
				t.Fatalf("state.Shell = %+v, want %d removed", state.Shell, tc.wantRemoved)
			}
			if tc.tamper && state.Shell.Error == "" {
				t.Fatalf("state.Shell = %+v, want the validation error", state.Shell)
			}
			if exists(site(shared)) == tc.wantGone {
				t.Fatalf("ярлык существует = %v, want %v", exists(site(shared)), !tc.wantGone)
			}
		})
	}
}

func TestShellHandoffRecord(t *testing.T) {
	var none *shellHandoff
	none.record(workerState{Shell: &shellReport{Removed: []string{"a"}}})
	if len(none.reports()) != 0 {
		t.Fatal("nil handoff накопил итог")
	}

	handoff := &shellHandoff{}
	handoff.record(workerState{})
	if len(handoff.reports()) != 0 {
		t.Fatalf("состояние без итога записано: %+v", handoff.Reports)
	}
	handoff.record(workerState{Shell: &shellReport{Removed: []string{"a"}, Error: "x"}})
	if got := handoff.reports(); len(got) != 1 || got[0].Error != "x" || got[0].Removed[0] != "a" {
		t.Fatalf("reports = %+v", got)
	}
}

func TestRunElevatedCarriesShellJobAndReport(t *testing.T) {
	report := shellReport{Removed: []string{`c:\users\public\desktop\ti.lnk`}, Error: "boom"}
	job := shellJob{Game: true, Before: map[string]shellEntryWire{`c:\users\public\desktop\a.lnk`: {Size: 3, Mod: 9}}}
	cases := []struct {
		name  string
		start func(t *testing.T) workerHandle
	}{
		{name: "итог подхвачен опросом", start: func(t *testing.T) workerHandle { return longRunningProcess(t, 10) }},
		{name: "итог прочитан после выхода воркера", start: quickExitProcess},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			statePath := filepath.Join(dir, "state.json")
			handoff := &shellHandoff{Job: job}
			spec := runSpec{Path: `C:\fake\installer.exe`, ID: "sh1", StatePath: statePath, CancelPath: filepath.Join(dir, "cancel"), Shell: handoff}

			var sent *shellJob
			withWorkerSeams(t, func(launch runSpec) (workerHandle, error) {
				ws, err := readWorkerSpec(launch.Args[1])
				if err != nil {
					return nil, err
				}
				sent = ws.Shell
				if err := writeWorkerState(statePath, workerState{Run: ws.Run, Done: true, Shell: &report}); err != nil {
					return nil, err
				}
				return tc.start(t), nil
			})

			if _, err := runElevated(context.Background(), spec); err != nil {
				t.Fatalf("runElevated error = %v", err)
			}
			if sent == nil || !sent.Game || sent.Before[`c:\users\public\desktop\a.lnk`] != (shellEntryWire{Size: 3, Mod: 9}) {
				t.Fatalf("воркеру передан job = %+v", sent)
			}
			if len(handoff.Reports) != 1 || handoff.Reports[0].Error != "boom" || len(handoff.Reports[0].Removed) != 1 {
				t.Fatalf("handoff.Reports = %+v, want the worker's report", handoff.Reports)
			}
		})
	}
}

func TestRunElevatedWithoutShellJobSendsNone(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	spec := runSpec{Path: `C:\fake\installer.exe`, ID: "sh2", StatePath: statePath, CancelPath: filepath.Join(dir, "cancel")}
	var sent *shellJob
	withWorkerSeams(t, func(launch runSpec) (workerHandle, error) {
		ws, err := readWorkerSpec(launch.Args[1])
		if err != nil {
			return nil, err
		}
		sent = ws.Shell
		if err := writeWorkerState(statePath, workerState{Run: ws.Run, Done: true}); err != nil {
			return nil, err
		}
		return quickExitProcess(t), nil
	})
	if _, err := runElevated(context.Background(), spec); err != nil {
		t.Fatalf("runElevated error = %v", err)
	}
	if sent != nil {
		t.Fatalf("воркеру передан job = %+v, хотя уборку не просили", sent)
	}
}

func TestSignedBrokerSpecCoversShellJob(t *testing.T) {
	dir, pin := brokerDirs(t)
	spec := goodSpec(pin)
	spec.Run = "fresh"
	spec.Shell = &shellJob{Game: true, Before: map[string]shellEntryWire{`c:\users\public\desktop\a.lnk`: {Size: 1, Mod: 2}}}
	if err := os.WriteFile(spec.InstallerPath, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeSignedBrokerSpec(context.Background(), dir, spec, brokerTestPrivate); err != nil {
		t.Fatal(err)
	}
	signed, found, err := readBrokerSpec(dir, pin, brokerTestPublic)
	if err != nil || !found {
		t.Fatalf("валидная подпись отвергнута: %v", err)
	}
	if signed.Shell == nil || !signed.Shell.Game {
		t.Fatalf("job потерян при записи: %+v", signed.Shell)
	}

	signed.Shell.Game = false
	signed.Shell.Before = map[string]shellEntryWire{}
	if err := writeWorkerSpec(brokerSpecPath(dir), signed); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readBrokerSpec(dir, pin, brokerTestPublic); err == nil {
		t.Fatal("подменённый снимок ярлыков принят брокером")
	}
}

func TestDropShortcutsLeavesSharedFoldersToWorker(t *testing.T) {
	cases := []struct {
		name       string
		handoff    func() *shellHandoff
		wantShared bool
	}{
		{name: "воркер не использовался", handoff: func() *shellHandoff { return nil }, wantShared: false},
		{name: "задание было, итога нет", handoff: func() *shellHandoff { return &shellHandoff{} }, wantShared: false},
		{name: "воркер отчитался", handoff: func() *shellHandoff { return &shellHandoff{Reports: []shellReport{{Error: "access denied"}}} }, wantShared: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			user := t.TempDir()
			shared := t.TempDir()
			useSharedRoots(t, shared)
			before, err := takeShellSnapshot(context.Background(), []string{user, shared})
			if err != nil {
				t.Fatalf("takeShellSnapshot error = %v", err)
			}
			userSite := filepath.Join(user, "ti.lnk")
			sharedSite := filepath.Join(shared, "ti.lnk")
			writeShortcut(t, userSite, `C:\Program Files (x86)\TI\TI.URL`)
			writeShortcut(t, sharedSite, `C:\Program Files (x86)\TI\TI.URL`)

			(&Service{}).dropShortcuts(context.Background(), "id", before, "", false, tc.handoff())

			if exists(userSite) {
				t.Fatal("своя папка не убрана лаунчером")
			}
			if exists(sharedSite) != tc.wantShared {
				t.Fatalf("ярлык общей папки существует = %v, want %v", exists(sharedSite), tc.wantShared)
			}
		})
	}
}

func TestDropShortcutsKeepsEverythingWhenSharedFoldersUnknown(t *testing.T) {
	user := t.TempDir()
	shared := t.TempDir()
	before, err := takeShellSnapshot(context.Background(), []string{user, shared})
	if err != nil {
		t.Fatalf("takeShellSnapshot error = %v", err)
	}
	userSite := filepath.Join(user, "ti.lnk")
	writeShortcut(t, userSite, `C:\Program Files (x86)\TI\TI.URL`)
	previous := sharedRootsFn
	sharedRootsFn = func() ([]string, error) { return nil, errors.New("known folder недоступна") }
	t.Cleanup(func() { sharedRootsFn = previous })

	handoff := &shellHandoff{Reports: []shellReport{{}}}
	(&Service{}).dropShortcuts(context.Background(), "id", before, "", false, handoff)

	if !exists(userSite) {
		t.Fatal("лаунчер чистит наугад, не зная, какие каталоги принадлежат воркеру")
	}
}

func TestSilentInstallDelegatesSharedShortcuts(t *testing.T) {
	cases := []struct {
		name       string
		elevated   bool
		wantShared bool
	}{
		{name: "без воркера лаунчер чистит всё сам", elevated: false, wantShared: false},
		{name: "воркер вёл установку: общий каталог за ним", elevated: true, wantShared: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, downloads, _ := newTestService(t)
			root := t.TempDir()
			innoSource(t, root)
			downloads.add("d1", "Game", root)

			games := t.TempDir()
			dest := filepath.Join(games, "Game")
			s.roots = []string{games}
			user := t.TempDir()
			shared := t.TempDir()
			useSharedRoots(t, shared)
			previous := shortcutRootsFn
			shortcutRootsFn = func() ([]string, error) { return []string{user, shared}, nil }
			t.Cleanup(func() { shortcutRootsFn = previous })

			userSite := filepath.Join(user, "ti.lnk")
			sharedSite := filepath.Join(shared, "ti.lnk")
			var delegated bool
			s.runner = &fakeRunner{act: func(spec runSpec) {
				delegated = spec.Shell != nil
				writeShortcut(t, userSite, `C:\Program Files (x86)\TI\TI.URL`)
				writeShortcut(t, sharedSite, `C:\Program Files (x86)\TI\TI.URL`)
				mkFile(t, filepath.Join(dest, "Game.exe"), 8192)
				if tc.elevated && spec.Shell != nil {
					spec.Shell.Reports = append(spec.Shell.Reports, shellReport{Error: "access denied"})
				}
			}}

			item, err := s.Start("d1", StartOptions{Destination: dest})
			if err != nil {
				t.Fatalf("start: %v", err)
			}
			s.waitStatus(t, item.ID, StatusCompleted)
			s.waitJobDone(t, item.ID)

			if !delegated {
				t.Fatal("раннер не получил задание для воркера")
			}
			if exists(userSite) {
				t.Fatal("лаунчер не убрал ярлык из своего каталога")
			}
			if exists(sharedSite) != tc.wantShared {
				t.Fatalf("ярлык общего каталога существует = %v, want %v", exists(sharedSite), tc.wantShared)
			}
		})
	}
}

func TestFinishResumedLogsWorkerShortcutReport(t *testing.T) {
	sink := captureLogs(t)
	s, _, _ := newTestService(t)
	dest := filepath.Join(t.TempDir(), "Game")
	mkFile(t, filepath.Join(dest, "Game.exe"), 8192)
	const id = "resumed1"
	s.mu.Lock()
	s.items = append(s.items, &Installation{ID: id, Name: "Game", Type: TypeExeInstaller, Status: StatusInstalling, Destination: dest})
	s.mu.Unlock()

	state := workerState{Done: true, Shell: &shellReport{Removed: []string{`c:\users\public\desktop\ti.lnk`}, Error: "access denied"}}
	s.finishResumed(context.Background(), id, state)

	logs := sink.text()
	for _, want := range []string{"installer shortcuts removed by elevated worker", "elevated worker could not remove installer shortcuts", "access denied"} {
		if !strings.Contains(logs, want) {
			t.Fatalf("в логе нет %q:\n%s", want, logs)
		}
	}
}

func TestFinishResumedSkipsShortcutLogWithoutReport(t *testing.T) {
	sink := captureLogs(t)
	s, _, _ := newTestService(t)
	const id = "resumed2"
	s.mu.Lock()
	s.items = append(s.items, &Installation{ID: id, Name: "Game", Type: TypeExeInstaller, Status: StatusInstalling, Destination: filepath.Join(t.TempDir(), "Game")})
	s.mu.Unlock()

	s.finishResumed(context.Background(), id, workerState{Done: true})

	if strings.Contains(sink.text(), "installer shortcuts") {
		t.Fatalf("в логе итог уборки, которого не было:\n%s", sink.text())
	}
}
