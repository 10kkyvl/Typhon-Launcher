package install

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
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
		job := shellJob{Before: map[string]shellEntryWire{inRoot: {Size: 7, ModNsec: 42}}}
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
		if !ok || wire.Size != want.size || !time.Unix(wire.ModSec, wire.ModNsec).Equal(want.mod) {
			t.Fatalf("job.Before[%q] = %+v, want size %d mod %v", key, wire, want.size, want.mod)
		}
		if !slices.Equal(handoff.shared, []string{shared}) {
			t.Fatalf("handoff.shared = %v, want %v", handoff.shared, []string{shared})
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
		failure := errors.New("отказано в доступе")
		failRemoval(t, locked, failure)

		report := cleanSharedShortcuts(context.Background(), job, dest)
		if !strings.Contains(report.Error, failure.Error()) {
			t.Fatalf("report = %+v, want the delete error", report)
		}
		if len(report.Removed) != 1 || exists(removable) {
			t.Fatalf("report = %+v, removable exists = %v", report, exists(removable))
		}
		if !exists(locked) {
			t.Fatal("ярлык, удаление которого отказало, исчез")
		}
	})
}

// failRemoval подменяет удаление одной записи отказом: так тест не зависит от
// того, пускают ли права файловой системы, и одинаково работает от root.
func failRemoval(t *testing.T, path string, failure error) {
	t.Helper()
	previous := removeShellEntry
	removeShellEntry = func(entry *shellEntryFile) error {
		if samePath(entry.path, path) {
			return failure
		}
		return previous(entry)
	}
	t.Cleanup(func() { removeShellEntry = previous })
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

			runErr := runWorkerSpec(withInstaller(t, spec))
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

func TestInstallerFinished(t *testing.T) {
	dir := t.TempDir()
	success := filepath.Join(dir, "success.log")
	writeUTF16(t, success, "Registering files\r\n"+innoSuccessMarker+"\r\n")
	rollback := filepath.Join(dir, "rollback.log")
	writeUTF16(t, rollback, "Rolling back changes\r\n")
	cases := []struct {
		name    string
		engine  Engine
		code    int
		log     string
		want    bool
		wantErr bool
	}{
		{name: "ноль", engine: EngineInno, code: 0, want: true},
		{name: "перезагрузка", engine: EngineMsi, code: rebootExitCode, want: true},
		{name: "msi 1641", engine: EngineMsi, code: 1641, want: true},
		{name: "inno отменён", engine: EngineInno, code: 2, log: rollback},
		{name: "msi отменён", engine: EngineMsi, code: 1602},
		{name: "msi занят", engine: EngineMsi, code: 1618},
		{name: "падение после успеха", engine: EngineInno, code: 3221226525, log: success, want: true},
		{name: "падение без отметки", engine: EngineInno, code: 3221226525, log: rollback},
		{name: "лога нет", engine: EngineInno, code: 1, log: filepath.Join(dir, "нет.log")},
		{name: "лог не читается", engine: EngineInno, code: 1, log: dir, wantErr: true},
		{name: "msi лог не смотрит", engine: EngineMsi, code: 1603, log: success},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := installerFinished(tc.engine, tc.code, tc.log)
			if (err != nil) != tc.wantErr || got != tc.want {
				t.Fatalf("installerFinished = (%v, %v), want (%v, err %v)", got, err, tc.want, tc.wantErr)
			}
		})
	}
}

func TestWorkerCleansSharedShortcutsOnlyAfterSuccess(t *testing.T) {
	cases := []struct {
		name        string
		engine      Engine
		code        int
		log         string
		logIsDir    bool
		wantCleaned bool
		wantErr     bool
	}{
		{name: "inno отменён кодом 2", engine: EngineInno, code: 2},
		{name: "inno упал без отметки в логе", engine: EngineInno, code: 1, log: "Rolling back changes\r\n"},
		{name: "msi отменён 1602", engine: EngineMsi, code: 1602},
		{name: "msi занят 1618", engine: EngineMsi, code: 1618},
		{name: "лог не читается", engine: EngineInno, code: 1, logIsDir: true, wantErr: true},
		{name: "gog упал после успеха", engine: EngineInno, code: 3221226525, log: "Registering files\r\n" + innoSuccessMarker + "\r\n", wantCleaned: true},
		{name: "чистый выход", engine: EngineInno, code: 0, wantCleaned: true},
		{name: "перезагрузка", engine: EngineMsi, code: rebootExitCode, wantCleaned: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			shared := t.TempDir()
			useSharedRoots(t, shared)
			job := jobFor(t, shared, false)
			dir := t.TempDir()
			logPath := filepath.Join(dir, "installer.log")
			if tc.log != "" {
				writeUTF16(t, logPath, tc.log)
			}
			if tc.logIsDir {
				if err := os.MkdirAll(logPath, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			site := filepath.Join(shared, "ti.lnk")
			spec := workerSpec{ID: "w1", Run: "r1", Engine: tc.engine, LogPath: logPath, StatePath: filepath.Join(dir, "state.json"), Destination: filepath.Join(dir, "games", "game"), Shell: &job}
			previous := workerInstall
			workerInstall = func(context.Context, workerSpec, []string) (int, error) {
				writeShortcut(t, site, `C:\Program Files (x86)\TI\TI.URL`)
				return tc.code, nil
			}
			t.Cleanup(func() { workerInstall = previous })

			if err := runWorkerSpec(withInstaller(t, spec)); err != nil {
				t.Fatalf("runWorkerSpec error = %v", err)
			}
			state, found, err := readWorkerState(spec.StatePath)
			if err != nil || !found || !state.Done || state.Code != tc.code || state.Shell == nil {
				t.Fatalf("state = %+v found = %v err = %v", state, found, err)
			}
			if state.Shell.Skipped == tc.wantCleaned {
				t.Fatalf("state.Shell = %+v, want skipped = %v", state.Shell, !tc.wantCleaned)
			}
			if (state.Shell.Error != "") != tc.wantErr {
				t.Fatalf("state.Shell = %+v, want error = %v", state.Shell, tc.wantErr)
			}
			if exists(site) == tc.wantCleaned {
				t.Fatalf("ярлык существует = %v, want %v", exists(site), !tc.wantCleaned)
			}
		})
	}
}

func TestRunWorkerSpecRejectsForgedJob(t *testing.T) {
	system := t.TempDir()
	windir := filepath.Join(system, "Windows")
	programFiles := filepath.Join(system, "Program Files")
	sep := string(filepath.Separator)
	fixed := func(path string) func(string) string { return func(string) string { return path } }
	volumeRoot := func(path string) string { return filepath.VolumeName(path) + sep }
	cases := []struct {
		name        string
		forgeBefore bool
		dest        func(shared string) string
		foldersErr  error
		wantErr     error
	}{
		{name: "пустой снимок и корень тома", forgeBefore: true, dest: volumeRoot, wantErr: errShellBroadTarget},
		{name: "пустой снимок и каталог первого уровня", forgeBefore: true, dest: func(shared string) string { return volumeRoot(shared) + "Games" }, wantErr: errShellBroadTarget},
		{name: "пустой снимок и сам общий каталог", forgeBefore: true, dest: func(shared string) string { return shared }, wantErr: errShellBroadTarget},
		{name: "каталог игры внутри общего каталога", dest: func(shared string) string { return filepath.Join(shared, "tools") }, wantErr: errShellBroadTarget},
		{name: "каталог игры над общим каталогом", dest: filepath.Dir, wantErr: errShellBroadTarget},
		{name: "пустой снимок и каталог внутри Windows", forgeBefore: true, dest: fixed(filepath.Join(windir, "System32")), wantErr: errShellBroadTarget},
		{name: "пустой снимок и сам Program Files", forgeBefore: true, dest: fixed(programFiles), wantErr: errShellBroadTarget},
		{name: "системные каталоги не определились", dest: fixed(filepath.Join(system, "games", "game")), foldersErr: errors.New("known folder недоступна"), wantErr: errShellBroadTarget},
		{name: "пустой снимок и пустой каталог игры", forgeBefore: true, dest: fixed(""), wantErr: errShellNoTarget},
		{name: "каталог игры не в каноническом виде", dest: fixed(filepath.Join(system, "games") + sep + ".." + sep + "game"), wantErr: errShellBadTarget},
		{name: "игра внутри Program Files", dest: fixed(filepath.Join(programFiles, "Game"))},
		{name: "обычный каталог игры", dest: fixed(filepath.Join(system, "games", "game"))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			shared := t.TempDir()
			useSharedRoots(t, shared)
			previous := systemFoldersFn
			systemFoldersFn = func() (string, []string, error) { return windir, []string{programFiles}, tc.foldersErr }
			t.Cleanup(func() { systemFoldersFn = previous })
			dest := tc.dest(shared)
			match := dest
			if match == "" {
				match = system
			}
			pointing := "[InternetShortcut]\r\nURL=file:///" + filepath.ToSlash(filepath.Join(match, "app.exe")) + "\r\n"
			victim := filepath.Join(shared, "victim.url")
			writeFile(t, victim, pointing)
			job := jobFor(t, shared, true)
			if tc.forgeBefore {
				job.Before = map[string]shellEntryWire{}
			}
			site := filepath.Join(shared, "ti.lnk")
			fresh := filepath.Join(shared, "fresh.url")
			state := filepath.Join(t.TempDir(), "state.json")
			install := workerInstall
			workerInstall = func(context.Context, workerSpec, []string) (int, error) {
				writeShortcut(t, site, `C:\Program Files (x86)\TI\TI.URL`)
				writeFile(t, fresh, pointing)
				return 0, nil
			}
			t.Cleanup(func() { workerInstall = install })

			if err := runWorkerSpec(withInstaller(t, workerSpec{ID: "w1", Run: "r1", StatePath: state, Destination: dest, Shell: &job})); err != nil {
				t.Fatalf("runWorkerSpec error = %v", err)
			}
			got, found, err := readWorkerState(state)
			if err != nil || !found || got.Shell == nil {
				t.Fatalf("state = %+v found = %v err = %v", got, found, err)
			}
			if exists(site) {
				t.Fatalf("ярлык сайта остался: %+v", got.Shell)
			}
			if !exists(victim) {
				t.Fatalf("удалён ярлык, существовавший до установки: %+v", got.Shell)
			}
			if tc.wantErr == nil {
				if got.Shell.Error != "" || exists(fresh) {
					t.Fatalf("state.Shell = %+v, fresh exists = %v, want the game shortcut removed", got.Shell, exists(fresh))
				}
				return
			}
			if !strings.Contains(got.Shell.Error, tc.wantErr.Error()) {
				t.Fatalf("state.Shell = %+v, want %v", got.Shell, tc.wantErr)
			}
			if !exists(fresh) {
				t.Fatalf("правило игры сработало по отвергнутому каталогу %q: %+v", dest, got.Shell)
			}
		})
	}
}

func TestCheckGameTarget(t *testing.T) {
	shared := t.TempDir()
	dir := t.TempDir()
	previous := systemFoldersFn
	systemFoldersFn = func() (string, []string, error) {
		return filepath.Join(dir, "Windows"), []string{filepath.Join(dir, "Program Files"), filepath.Join(dir, "Program Files (x86)")}, nil
	}
	t.Cleanup(func() { systemFoldersFn = previous })
	volume := filepath.VolumeName(dir) + string(filepath.Separator)
	cases := []struct {
		name   string
		target string
		want   error
	}{
		{name: "пусто", target: "", want: errShellNoTarget},
		{name: "относительный", target: filepath.Join("games", "gta"), want: errShellBadTarget},
		{name: "с точками", target: filepath.Join(dir, "a") + string(filepath.Separator) + "..", want: errShellBadTarget},
		{name: "корень тома", target: volume, want: errShellBroadTarget},
		{name: "первый уровень", target: filepath.Join(volume, "Games"), want: errShellBroadTarget},
		{name: "Windows", target: filepath.Join(dir, "Windows"), want: errShellBroadTarget},
		{name: "внутри Windows", target: filepath.Join(dir, "Windows", "System32"), want: errShellBroadTarget},
		{name: "Program Files", target: filepath.Join(dir, "Program Files"), want: errShellBroadTarget},
		{name: "Program Files (x86)", target: filepath.Join(dir, "Program Files (x86)"), want: errShellBroadTarget},
		{name: "общий каталог", target: shared, want: errShellBroadTarget},
		{name: "внутри общего каталога", target: filepath.Join(shared, "x"), want: errShellBroadTarget},
		{name: "над общим каталогом", target: filepath.Dir(shared), want: errShellBroadTarget},
		{name: "игра в Program Files", target: filepath.Join(dir, "Program Files", "Game")},
		{name: "игра в библиотеке", target: filepath.Join(dir, "Games", "GTA SA")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := checkGameTarget(tc.target, []string{shared})
			if tc.want == nil && err != nil || tc.want != nil && !errors.Is(err, tc.want) {
				t.Fatalf("checkGameTarget(%q) = %v, want %v", tc.target, err, tc.want)
			}
		})
	}
}

func TestShellHandoffKeepsModTime(t *testing.T) {
	shared := t.TempDir()
	useSharedRoots(t, shared)
	path := filepath.Join(shared, "old.url")
	key := strings.ToLower(path)
	cases := []struct {
		name string
		mod  time.Time
	}{
		{name: "FILETIME около нуля", mod: time.Date(1601, 1, 2, 0, 0, 0, 0, time.UTC)},
		{name: "до 1678", mod: time.Date(1677, 9, 21, 0, 0, 0, 0, time.UTC)},
		{name: "после 2262", mod: time.Date(2263, 1, 1, 0, 0, 0, 0, time.UTC)},
		{name: "наносекунды", mod: time.Date(2026, 10, 3, 12, 0, 0, 123456789, time.UTC)},
		{name: "нулевое время", mod: time.Time{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := shellSnapshot{roots: []string{shared}, taken: true, entries: map[string]shellEntry{key: {size: 1, mod: tc.mod, path: path}}}
			handoff, err := newShellHandoff(before, false)
			if err != nil || handoff == nil {
				t.Fatalf("newShellHandoff = (%v, %v)", handoff, err)
			}
			data, err := json.Marshal(handoff.Job)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var job shellJob
			if err := json.Unmarshal(data, &job); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			snap, err := job.snapshot([]string{shared})
			if err != nil {
				t.Fatalf("snapshot: %v", err)
			}
			if got := snap.entries[key].mod; !got.Equal(tc.mod) {
				t.Fatalf("время %v после передачи воркеру стало %v", tc.mod, got)
			}
		})
	}
}

func TestShellHandoffRecord(t *testing.T) {
	var none *shellHandoff
	none.record(workerState{Shell: &shellReport{Removed: []string{"a"}}})
	if none.forInstaller() != nil {
		t.Fatal("nil handoff размножился")
	}

	handoff := &shellHandoff{shared: []string{"x"}}
	handoff.record(workerState{})
	if handoff.Report != nil {
		t.Fatalf("состояние без итога записано: %+v", handoff.Report)
	}
	report := &shellReport{Removed: []string{"a"}, Error: "x"}
	handoff.record(workerState{Shell: report})
	report.Error = "changed"
	if got := handoff.Report; got == nil || got.Error != "x" || len(got.Removed) != 1 || got.Removed[0] != "a" {
		t.Fatalf("report = %+v", got)
	}
	copied := handoff.forInstaller()
	if copied.Report != nil || !slices.Equal(copied.shared, handoff.shared) {
		t.Fatalf("forInstaller = %+v, want the job without a report", copied)
	}
}

func TestDelegatedRoots(t *testing.T) {
	shared := []string{"shared"}
	done := func() *shellHandoff { return &shellHandoff{shared: shared, Report: &shellReport{}} }
	cases := []struct {
		name    string
		workers []*shellHandoff
		want    []string
	}{
		{name: "установщиков нет", workers: nil},
		{name: "без воркера", workers: []*shellHandoff{nil}},
		{name: "воркер не отчитался", workers: []*shellHandoff{{shared: shared}}},
		{name: "воркер пропустил уборку", workers: []*shellHandoff{{shared: shared, Report: &shellReport{Skipped: true}}}},
		{name: "второй установщик без воркера", workers: []*shellHandoff{done(), {shared: shared}}},
		{name: "первый установщик без воркера", workers: []*shellHandoff{nil, done()}},
		{name: "все через воркер", workers: []*shellHandoff{done(), done()}, want: shared},
		{name: "воркер отчитался ошибкой", workers: []*shellHandoff{{shared: shared, Report: &shellReport{Error: "access denied"}}}, want: shared},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := delegatedRoots(tc.workers); !slices.Equal(got, tc.want) {
				t.Fatalf("delegatedRoots = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRunElevatedCarriesShellJobAndReport(t *testing.T) {
	report := shellReport{Removed: []string{`c:\users\public\desktop\ti.lnk`}, Error: "boom"}
	job := shellJob{Game: true, Before: map[string]shellEntryWire{`c:\users\public\desktop\a.lnk`: {Size: 3, ModNsec: 9}}}
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
			spec := runSpec{Path: `C:\fake\installer.exe`, InstallerPath: installerFixture(t, dir), ID: "sh1", StatePath: statePath, CancelPath: filepath.Join(dir, "cancel"), Shell: handoff}

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
			if sent == nil || !sent.Game || sent.Before[`c:\users\public\desktop\a.lnk`] != (shellEntryWire{Size: 3, ModNsec: 9}) {
				t.Fatalf("воркеру передан job = %+v", sent)
			}
			if handoff.Report == nil || handoff.Report.Error != "boom" || len(handoff.Report.Removed) != 1 {
				t.Fatalf("handoff.Report = %+v, want the worker's report", handoff.Report)
			}
		})
	}
}

func TestRunElevatedWithoutShellJobSendsNone(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	spec := runSpec{Path: `C:\fake\installer.exe`, InstallerPath: installerFixture(t, dir), ID: "sh2", StatePath: statePath, CancelPath: filepath.Join(dir, "cancel")}
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
	spec.Shell = &shellJob{Game: true, Before: map[string]shellEntryWire{`c:\users\public\desktop\a.lnk`: {Size: 1, ModNsec: 2}}}
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
	reported := func(shared string) *shellHandoff {
		return &shellHandoff{shared: []string{shared}, Report: &shellReport{Error: "access denied"}}
	}
	cases := []struct {
		name       string
		workers    func(shared string) []*shellHandoff
		wantShared bool
	}{
		{name: "воркер не использовался", workers: func(string) []*shellHandoff { return nil }, wantShared: false},
		{name: "задание было, итога нет", workers: func(shared string) []*shellHandoff {
			return []*shellHandoff{{shared: []string{shared}}}
		}, wantShared: false},
		{name: "воркер отчитался", workers: func(shared string) []*shellHandoff { return []*shellHandoff{reported(shared)} }, wantShared: true},
		{name: "воркер пропустил уборку", workers: func(shared string) []*shellHandoff {
			return []*shellHandoff{{shared: []string{shared}, Report: &shellReport{Skipped: true}}}
		}, wantShared: false},
		{name: "второй установщик цепочки шёл без воркера", workers: func(shared string) []*shellHandoff {
			return []*shellHandoff{reported(shared), {shared: []string{shared}}}
		}, wantShared: false},
		{name: "оба установщика через воркер", workers: func(shared string) []*shellHandoff {
			return []*shellHandoff{reported(shared), reported(shared)}
		}, wantShared: true},
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

			(&Service{}).dropShortcuts(context.Background(), "id", before, "", false, tc.workers(shared))

			if exists(userSite) {
				t.Fatal("своя папка не убрана лаунчером")
			}
			if exists(sharedSite) != tc.wantShared {
				t.Fatalf("ярлык общей папки существует = %v, want %v", exists(sharedSite), tc.wantShared)
			}
		})
	}
}

func TestDropShortcutsCleansUserFoldersWhenSharedRootsFail(t *testing.T) {
	user := t.TempDir()
	shared := t.TempDir()
	useSharedRoots(t, shared)
	before, err := takeShellSnapshot(context.Background(), []string{user, shared})
	if err != nil {
		t.Fatalf("takeShellSnapshot error = %v", err)
	}
	handoff, err := newShellHandoff(before, false)
	if err != nil || handoff == nil {
		t.Fatalf("newShellHandoff = (%v, %v)", handoff, err)
	}
	worker := handoff.forInstaller()
	worker.record(workerState{Shell: &shellReport{}})
	userSite := filepath.Join(user, "ti.lnk")
	sharedSite := filepath.Join(shared, "ti.lnk")
	writeShortcut(t, userSite, `C:\Program Files (x86)\TI\TI.URL`)
	writeShortcut(t, sharedSite, `C:\Program Files (x86)\TI\TI.URL`)
	previous := sharedRootsFn
	sharedRootsFn = func() ([]string, error) { return nil, errors.New("known folder недоступна") }
	t.Cleanup(func() { sharedRootsFn = previous })

	(&Service{}).dropShortcuts(context.Background(), "id", before, "", false, []*shellHandoff{worker})

	if exists(userSite) {
		t.Fatal("своя папка не убрана, потому что общие каталоги не определились повторно")
	}
	if !exists(sharedSite) {
		t.Fatal("лаунчер полез в общую папку, которую убирал воркер")
	}
}

func TestSilentChainCleansSharedShortcutOfUnelevatedInstaller(t *testing.T) {
	s, downloads, _ := newTestService(t)
	root := t.TempDir()
	dir := cupheadSource(t, root)
	downloads.add("d1", "Cuphead", root)
	games := t.TempDir()
	dest := filepath.Join(games, "Cuphead")
	s.roots = []string{games}
	user := t.TempDir()
	shared := t.TempDir()
	useSharedRoots(t, shared)
	previous := shortcutRootsFn
	shortcutRootsFn = func() ([]string, error) { return []string{user, shared}, nil }
	t.Cleanup(func() { shortcutRootsFn = previous })

	addonSite := filepath.Join(shared, "addon.lnk")
	runner := &chainRunner{act: func(spec runSpec) {
		mkFile(t, filepath.Join(dest, "Cuphead.exe"), 8192)
		if spec.InstallerPath == filepath.Join(dir, cupheadAddon+".exe") {
			writeShortcut(t, addonSite, `C:\Program Files (x86)\TI\TI.URL`)
			return
		}
		spec.Shell.record(workerState{Shell: &shellReport{}})
	}}
	s.runner = runner

	item, err := s.Start("d1", StartOptions{Destination: dest})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	s.waitStatus(t, item.ID, StatusCompleted)
	s.waitJobDone(t, item.ID)

	calls := runner.calls()
	if len(calls) != 2 || calls[0].Shell == nil || calls[0].Shell == calls[1].Shell {
		t.Fatalf("calls = %d, у каждого установщика должно быть своё задание для воркера", len(calls))
	}
	if exists(addonSite) {
		t.Fatal("ярлык неэлевированного установщика цепочки остался в общей папке")
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
				if tc.elevated {
					spec.Shell.record(workerState{Shell: &shellReport{Error: "access denied"}})
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
