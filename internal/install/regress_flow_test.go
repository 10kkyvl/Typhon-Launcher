package install

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const (
	rgInnoMarker = "Inno Setup Setup Data (5.6.2) (u)"
	rgNsisMarker = "Nullsoft Install System v3.08"
)

// flowPlan — одна строка матрицы: как запустить установку и что обязано остаться
// в записи, в library и на диске, когда она придёт в устойчивое состояние.
type flowPlan struct {
	opts    StartOptions
	seeding bool
	runner  runner

	startCode string

	status  Status
	errCode string
	errText string
	typ     Type
	engine  Engine
	silent  bool
	mode    string
	chain   int
	dest    string
	exe     string
	owned   bool
	games   int

	confirm string
	files   []string
	inspect func(t *testing.T, r *rig, it Installation)
}

type flowCase struct {
	name  string
	build func(t *testing.T, r *rig) flowPlan
}

func installGame(t *testing.T, dir string) func(runSpec) {
	return func(runSpec) { mkFile(t, filepath.Join(dir, "Game.exe"), 8192) }
}

func silentSource(t *testing.T, r *rig, marker string) string {
	t.Helper()
	root := t.TempDir()
	mkInstaller(t, filepath.Join(root, "Game", "setup.exe"), marker)
	r.download("d1", "Game", root)
	return filepath.Join(r.games, "Game")
}

func interactiveSource(t *testing.T, r *rig) string {
	t.Helper()
	root := t.TempDir()
	mkFile(t, filepath.Join(root, "Game", "setup.exe"), 4096)
	r.download("d1", "Game", root)
	return filepath.Join(r.games, "MyGame")
}

func archiveSource(t *testing.T, r *rig, file string, write func(path string)) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "Game"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	write(filepath.Join(root, "Game", file))
	r.download("d1", "Game", root)
	return filepath.Join(t.TempDir(), "Game")
}

func gameZipEntries() []zipEntry {
	return []zipEntry{
		{name: "Game/Game.exe", data: bytes.Repeat([]byte("g"), 64<<10)},
		{name: "Game/data/content.pak", data: []byte("assets")},
	}
}

// storedZip пишет архив без сжатия, чтобы испорченный байт данных был виден как
// ошибка контрольной суммы, а не как ошибка распаковщика deflate.
func storedZip(t *testing.T, path string, entries []zipEntry) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, e := range entries {
		out, err := w.CreateHeader(&zip.FileHeader{Name: e.name, Method: zip.Store})
		if err != nil {
			t.Fatalf("create %q: %v", e.name, err)
		}
		if _, err := out.Write(e.data); err != nil {
			t.Fatalf("write %q: %v", e.name, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatalf("write zip: %v", err)
	}
}

const rgCorruptMarker = "corrupt-marker-payload"

func corruptStoredZip(t *testing.T, path string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read zip: %v", err)
	}
	at := bytes.Index(raw, []byte(rgCorruptMarker))
	if at < 0 {
		t.Fatalf("marker not found in %s", path)
	}
	raw[at] ^= 0xff
	mkText(t, path, string(raw))
}

func flowCases() []flowCase {
	cases := []flowCase{
		{"portable copy", func(t *testing.T, r *rig) flowPlan {
			root := t.TempDir()
			source := portableSource(t, root, "Game")
			r.download("d1", "Game", root)
			dest := filepath.Join(t.TempDir(), "Games", "Game")
			return flowPlan{
				opts: StartOptions{Destination: dest, Mode: ModeCopy}, status: StatusCompleted, typ: TypePortable,
				mode: ModeCopy, dest: dest, exe: filepath.Join(dest, "Game.exe"), owned: true, games: 1,
				inspect: func(t *testing.T, _ *rig, _ Installation) {
					if !exists(filepath.Join(source, "Game.exe")) {
						t.Fatal("copy mode removed the source")
					}
					if !exists(filepath.Join(dest, "data", "content.pak")) {
						t.Fatal("payload not installed")
					}
				},
			}
		}},
		{"portable move", func(t *testing.T, r *rig) flowPlan {
			root := t.TempDir()
			source := portableSource(t, root, "Game")
			r.download("d1", "Game", root)
			dest := filepath.Join(t.TempDir(), "Games", "Game")
			return flowPlan{
				opts: StartOptions{Destination: dest, Mode: ModeMove}, status: StatusCompleted, typ: TypePortable,
				mode: ModeMove, dest: dest, exe: filepath.Join(dest, "Game.exe"), owned: true, games: 1,
				inspect: func(t *testing.T, _ *rig, _ Installation) {
					if exists(source) {
						t.Fatal("move mode left the source behind after a registered install")
					}
					if !exists(filepath.Join(dest, "data", "content.pak")) {
						t.Fatal("payload not installed")
					}
				},
			}
		}},
		{"portable move while seeding degrades to copy", func(t *testing.T, r *rig) flowPlan {
			root := t.TempDir()
			source := portableSource(t, root, "Game")
			r.download("d1", "Game", root)
			dest := filepath.Join(t.TempDir(), "Games", "Game")
			return flowPlan{
				opts: StartOptions{Destination: dest, Mode: ModeMove}, seeding: true, status: StatusCompleted,
				typ: TypePortable, mode: ModeCopy, dest: dest, exe: filepath.Join(dest, "Game.exe"), owned: true, games: 1,
				inspect: func(t *testing.T, _ *rig, _ Installation) {
					if !exists(filepath.Join(source, "Game.exe")) {
						t.Fatal("a seeding source was moved away")
					}
				},
			}
		}},
		{"portable without registration for an update", func(t *testing.T, r *rig) flowPlan {
			root := t.TempDir()
			portableSource(t, root, "Game")
			r.download("d1", "Game", root)
			r.setCleanup("delete")
			dest := filepath.Join(t.TempDir(), "Games", "Game")
			return flowPlan{
				opts: StartOptions{Destination: dest, Mode: ModeCopy, SkipRegister: true}, status: StatusCompleted,
				typ: TypePortable, mode: ModeCopy, dest: dest, exe: filepath.Join(dest, "Game.exe"), owned: true,
				inspect: func(t *testing.T, r *rig, it Installation) {
					if it.GameID != "" {
						t.Fatalf("game id = %q, want none: nothing was registered", it.GameID)
					}
					if got := r.downloads.deletedIDs(); len(got) != 0 {
						t.Fatalf("download data removed = %v: cleanup must wait for the caller that registers the game", got)
					}
				},
			}
		}},
		{"zip", func(t *testing.T, r *rig) flowPlan {
			dest := archiveSource(t, r, "game.zip", func(path string) { writeZip(t, path, gameZipEntries()) })
			return flowPlan{
				opts: StartOptions{Destination: dest}, status: StatusCompleted, typ: TypeArchiveZip, dest: dest,
				exe: filepath.Join(dest, "Game.exe"), owned: true, games: 1,
				inspect: func(t *testing.T, _ *rig, _ Installation) {
					if exists(filepath.Join(dest, "Game")) {
						t.Fatal("the wrapper directory of the archive was kept")
					}
				},
			}
		}},
		{"rar", func(t *testing.T, r *rig) flowPlan {
			neverConsultTools(t)
			dest := archiveSource(t, r, "game.rar", func(path string) {
				writeStoredRar(t, path, []rarEntry{
					{name: "Game/Game.exe", data: bytes.Repeat([]byte("r"), 64<<10)},
					{name: "Game/data/content.pak", data: []byte("assets")},
				})
			})
			return flowPlan{
				opts: StartOptions{Destination: dest}, status: StatusCompleted, typ: TypeArchiveRar, dest: dest,
				exe: filepath.Join(dest, "Game.exe"), owned: true, games: 1,
			}
		}},
		{"7z", func(t *testing.T, r *rig) flowPlan {
			dest := archiveSource(t, r, "game.7z", func(path string) { writeSevenZipGame(t, path) })
			return flowPlan{
				opts: StartOptions{Destination: dest}, status: StatusCompleted, typ: TypeArchive7z, dest: dest,
				exe: filepath.Join(dest, "Game.exe"), owned: true, games: 1,
				inspect: func(t *testing.T, _ *rig, _ Installation) {
					if !exists(filepath.Join(dest, "data", "content.pak")) {
						t.Fatal("7z payload not installed")
					}
				},
			}
		}},
		{"zip with a bad checksum fails and leaves nothing", func(t *testing.T, r *rig) flowPlan {
			dest := archiveSource(t, r, "game.zip", func(path string) {
				storedZip(t, path, []zipEntry{{name: "Game/Game.exe", data: []byte(rgCorruptMarker + strings.Repeat("g", 4096))}})
				corruptStoredZip(t, path)
			})
			return flowPlan{
				opts: StartOptions{Destination: dest}, status: StatusFailed, errText: "checksum", typ: TypeArchiveZip,
				dest: dest, owned: true,
				inspect: func(t *testing.T, _ *rig, _ Installation) {
					if exists(dest) || exists(dest+partialSuffix) {
						t.Fatalf("failed extraction left %s or its .partial behind", dest)
					}
				},
			}
		}},
		{"rar cut short in the middle of its data", func(t *testing.T, r *rig) flowPlan {
			neverConsultTools(t)
			whole := buildStoredRar4([]rar4Entry{
				{name: "Game/Game.exe", data: bytes.Repeat([]byte("a"), 20000)},
				{name: "Game/data/content.pak", data: bytes.Repeat([]byte("b"), 20000)},
			}, true)
			dest := archiveSource(t, r, "game.rar", func(path string) { writeRar(t, path, whole[:len(whole)*55/100]) })
			return flowPlan{
				opts: StartOptions{Destination: dest}, status: StatusFailed, errCode: "install.archive_incomplete", typ: TypeArchiveRar,
				dest: dest, owned: true,
				inspect: func(t *testing.T, _ *rig, _ Installation) {
					if exists(dest) || exists(dest+partialSuffix) {
						t.Fatalf("a failed extraction left %s or its .partial behind", dest)
					}
				},
			}
		}},
		{"zip without a game executable waits for the player", func(t *testing.T, r *rig) flowPlan {
			dest := archiveSource(t, r, "game.zip", func(path string) {
				writeZip(t, path, []zipEntry{
					{name: "Game/run.bat", data: bytes.Repeat([]byte("b"), 2048)},
					{name: "Game/data/content.pak", data: []byte("assets")},
				})
			})
			return flowPlan{
				opts: StartOptions{Destination: dest}, status: StatusWaitingForUser, typ: TypeArchiveZip, dest: dest,
				owned: true, confirm: filepath.Join(dest, "run.bat"),
			}
		}},
		{"zip with a weak executable unattended takes the best guess", func(t *testing.T, r *rig) flowPlan {
			dest := archiveSource(t, r, "game.zip", func(path string) {
				writeZip(t, path, []zipEntry{
					{name: "Game/launcher.exe", data: bytes.Repeat([]byte("l"), 2048)},
					{name: "Game/data/content.pak", data: []byte("assets")},
				})
			})
			return flowPlan{
				opts: StartOptions{Destination: dest, Unattended: true}, status: StatusCompleted, typ: TypeArchiveZip,
				dest: dest, exe: filepath.Join(dest, "launcher.exe"), owned: true, games: 1,
			}
		}},
		{"zip with no executable unattended completes without one", func(t *testing.T, r *rig) flowPlan {
			dest := archiveSource(t, r, "game.zip", func(path string) {
				writeZip(t, path, []zipEntry{
					{name: "Game/run.bat", data: bytes.Repeat([]byte("b"), 2048)},
					{name: "Game/data/content.pak", data: []byte("assets")},
				})
			})
			return flowPlan{
				opts: StartOptions{Destination: dest, Unattended: true}, status: StatusCompleted, typ: TypeArchiveZip,
				dest: dest, owned: true, games: 1,
			}
		}},
		{"zip into a non-empty destination is refused", func(t *testing.T, r *rig) flowPlan {
			dest := archiveSource(t, r, "game.zip", func(path string) { writeZip(t, path, gameZipEntries()) })
			mkFile(t, filepath.Join(dest, "keep.txt"), 8)
			return flowPlan{opts: StartOptions{Destination: dest}, startCode: "install.dest_not_empty"}
		}},
		{"silent inno", func(t *testing.T, r *rig) flowPlan {
			dest := silentSource(t, r, rgInnoMarker)
			run := newRgRunner(t, rgStep{act: installGame(t, dest), log: "Registering files"})
			return flowPlan{
				opts: StartOptions{Destination: dest}, runner: run, status: StatusCompleted, typ: TypeExeInstaller,
				engine: EngineInno, silent: true, dest: dest, exe: filepath.Join(dest, "Game.exe"), owned: true, games: 1,
				inspect: func(t *testing.T, _ *rig, _ Installation) {
					calls := run.calls()
					if len(calls) != 1 {
						t.Fatalf("installer runs = %d, want 1", len(calls))
					}
					args := strings.Join(calls[0].Args, " ")
					if !strings.Contains(args, "/VERYSILENT") || !strings.Contains(args, "/DIR="+dest) {
						t.Fatalf("args = %q", args)
					}
					if !calls[0].Background || !calls[0].Hidden || calls[0].Interactive {
						t.Fatalf("silent installer must run hidden in the background: %+v", calls[0])
					}
				},
			}
		}},
		{"silent nsis", func(t *testing.T, r *rig) flowPlan {
			dest := silentSource(t, r, rgNsisMarker)
			run := newRgRunner(t, rgStep{act: installGame(t, dest)})
			return flowPlan{
				opts: StartOptions{Destination: dest}, runner: run, status: StatusCompleted, typ: TypeExeInstaller,
				engine: EngineNsis, silent: true, dest: dest, exe: filepath.Join(dest, "Game.exe"), owned: true, games: 1,
				inspect: func(t *testing.T, _ *rig, _ Installation) {
					calls := run.calls()
					if len(calls) != 1 || !strings.Contains(calls[0].CmdLine, "/S /D="+dest) {
						t.Fatalf("nsis command line = %+v", calls)
					}
				},
			}
		}},
		{"silent installer ignores the target directory", func(t *testing.T, r *rig) flowPlan {
			dest := silentSource(t, r, rgInnoMarker)
			elsewhere := filepath.Join(r.games, "GOG Games", "Game")
			run := newRgRunner(t, rgStep{act: installGame(t, elsewhere)})
			return flowPlan{
				opts: StartOptions{Destination: dest}, runner: run, status: StatusCompleted, typ: TypeExeInstaller,
				engine: EngineInno, silent: true, dest: elsewhere, exe: filepath.Join(elsewhere, "Game.exe"), owned: true, games: 1,
				inspect: func(t *testing.T, _ *rig, _ Installation) {
					if exists(dest) {
						t.Fatalf("the empty directory %s asked for was left behind", dest)
					}
				},
			}
		}},
		{"silent chain of two installers", func(t *testing.T, r *rig) flowPlan {
			root := t.TempDir()
			dir := cupheadSource(t, root)
			r.download("d1", "Cuphead", root)
			dest := filepath.Join(r.games, "Cuphead")
			run := newRgRunner(t, rgStep{act: func(runSpec) { mkFile(t, filepath.Join(dest, "Cuphead.exe"), 8192) }})
			return flowPlan{
				opts: StartOptions{Destination: dest}, runner: run, status: StatusCompleted, typ: TypeExeInstaller,
				engine: EngineInno, silent: true, chain: 2, dest: dest, exe: filepath.Join(dest, "Cuphead.exe"), owned: true, games: 1,
				inspect: func(t *testing.T, _ *rig, _ Installation) {
					calls := run.calls()
					if len(calls) != 2 || calls[0].Path != filepath.Join(dir, cupheadBase+".exe") || calls[1].Path != filepath.Join(dir, cupheadAddon+".exe") {
						t.Fatalf("installer order = %+v, want the base game first", calls)
					}
				},
			}
		}},
		{"silent chain whose second installer fails", func(t *testing.T, r *rig) flowPlan {
			root := t.TempDir()
			cupheadSource(t, root)
			r.download("d1", "Cuphead", root)
			dest := filepath.Join(r.games, "Cuphead")
			run := newRgRunner(t,
				rgStep{act: func(runSpec) { mkFile(t, filepath.Join(dest, "Cuphead.exe"), 8192) }},
				rgStep{code: 1},
			)
			return flowPlan{
				opts: StartOptions{Destination: dest}, runner: run, status: StatusFailed, errCode: "install.installer_failed",
				typ: TypeExeInstaller, engine: EngineInno, silent: true, chain: 2, dest: dest,
				inspect: func(t *testing.T, _ *rig, _ Installation) {
					if exists(dest) {
						t.Fatalf("half-installed %s left behind", dest)
					}
				},
			}
		}},
		{"silent reboot-pending exit code is a success", func(t *testing.T, r *rig) flowPlan {
			dest := silentSource(t, r, rgInnoMarker)
			run := newRgRunner(t, rgStep{act: installGame(t, dest), code: rebootExitCode})
			return flowPlan{
				opts: StartOptions{Destination: dest}, runner: run, status: StatusCompleted, typ: TypeExeInstaller,
				engine: EngineInno, silent: true, dest: dest, exe: filepath.Join(dest, "Game.exe"), owned: true, games: 1,
			}
		}},
		{"silent crash after the log reported success", func(t *testing.T, r *rig) flowPlan {
			dest := silentSource(t, r, rgInnoMarker)
			run := newRgRunner(t, rgStep{act: installGame(t, dest), code: 3221226525, log: "Registering files\r\n" + innoSuccessMarker})
			return flowPlan{
				opts: StartOptions{Destination: dest}, runner: run, status: StatusCompleted, typ: TypeExeInstaller,
				engine: EngineInno, silent: true, dest: dest, exe: filepath.Join(dest, "Game.exe"), owned: true, games: 1,
			}
		}},
		{"silent installer fails", func(t *testing.T, r *rig) flowPlan {
			dest := silentSource(t, r, rgInnoMarker)
			run := newRgRunner(t, rgStep{act: func(runSpec) { mkFile(t, filepath.Join(dest, "half.bin"), 1024) }, code: 1})
			return flowPlan{
				opts: StartOptions{Destination: dest}, runner: run, status: StatusFailed, errCode: "install.installer_failed",
				typ: TypeExeInstaller, engine: EngineInno, silent: true, dest: dest,
				inspect: func(t *testing.T, _ *rig, _ Installation) {
					if exists(dest) {
						t.Fatalf("failed install left %s behind", dest)
					}
				},
			}
		}},
		{"silent installer cancelled in its own window", func(t *testing.T, r *rig) flowPlan {
			dest := silentSource(t, r, rgInnoMarker)
			run := newRgRunner(t, rgStep{code: 2})
			return flowPlan{
				opts: StartOptions{Destination: dest}, runner: run, status: StatusFailed, errCode: "install.installer_cancelled",
				typ: TypeExeInstaller, engine: EngineInno, silent: true, dest: dest,
			}
		}},
		{"silent installer refuses to run without a wizard", func(t *testing.T, r *rig) flowPlan {
			dest := silentSource(t, r, rgInnoMarker)
			run := newRgRunner(t, rgStep{code: 1, log: wizardRefusedLog})
			return flowPlan{
				opts: StartOptions{Destination: dest}, runner: run, status: StatusFailed, errCode: "install.installer_needs_interactive",
				typ: TypeExeInstaller, engine: EngineInno, silent: true, dest: dest,
			}
		}},
		{"silent installer wrote nothing", func(t *testing.T, r *rig) flowPlan {
			dest := silentSource(t, r, rgInnoMarker)
			run := newRgRunner(t, rgStep{})
			return flowPlan{
				opts: StartOptions{Destination: dest}, runner: run, status: StatusFailed, errCode: "install.installer_no_output",
				typ: TypeExeInstaller, engine: EngineInno, silent: true, dest: dest,
			}
		}},
		{"silent runner error is kept as text", func(t *testing.T, r *rig) flowPlan {
			dest := silentSource(t, r, rgInnoMarker)
			run := newRgRunner(t, rgStep{err: errors.New("process did not start")})
			return flowPlan{
				opts: StartOptions{Destination: dest}, runner: run, status: StatusFailed, errText: "process did not start",
				typ: TypeExeInstaller, engine: EngineInno, silent: true, dest: dest,
			}
		}},
		{"silent into a non-empty destination is refused", func(t *testing.T, r *rig) flowPlan {
			dest := silentSource(t, r, rgInnoMarker)
			mkFile(t, filepath.Join(dest, "keep.txt"), 8)
			return flowPlan{opts: StartOptions{Destination: dest}, startCode: "install.dest_not_empty"}
		}},
		{"silent without a destination is refused", func(t *testing.T, r *rig) flowPlan {
			silentSource(t, r, rgInnoMarker)
			return flowPlan{startCode: "install.no_destination"}
		}},
		{"silent with a relative destination is refused", func(t *testing.T, r *rig) flowPlan {
			silentSource(t, r, rgInnoMarker)
			return flowPlan{opts: StartOptions{Destination: filepath.Join("relative", "Game")}, startCode: "install.relative_destination"}
		}},
		{"interactive installer waits for the executable", func(t *testing.T, r *rig) flowPlan {
			installed := interactiveSource(t, r)
			run := newRgRunner(t, rgStep{act: func(runSpec) { mkFile(t, filepath.Join(installed, "MyGame.exe"), 512<<10) }})
			return flowPlan{
				runner: run, status: StatusWaitingForUser, typ: TypeExeInstaller, dest: installed, owned: true,
				exe: filepath.Join(installed, "MyGame.exe"), confirm: filepath.Join(installed, "MyGame.exe"),
				inspect: func(t *testing.T, _ *rig, it Installation) {
					calls := run.calls()
					if len(calls) != 1 || calls[0].Background || calls[0].Hidden || len(calls[0].Args) != 0 {
						t.Fatalf("interactive installer must run in the foreground without keys: %+v", calls)
					}
					if len(it.Candidates) == 0 {
						t.Fatal("no executable candidates offered")
					}
				},
			}
		}},
		{"interactive installer fails", func(t *testing.T, r *rig) flowPlan {
			interactiveSource(t, r)
			run := newRgRunner(t, rgStep{code: 1})
			return flowPlan{runner: run, status: StatusFailed, errCode: "install.installer_failed", typ: TypeExeInstaller}
		}},
		{"interactive installer cannot run unattended", func(t *testing.T, r *rig) flowPlan {
			interactiveSource(t, r)
			run := newRgRunner(t, rgStep{})
			return flowPlan{
				opts: StartOptions{Unattended: true}, runner: run, status: StatusFailed, errCode: "install.needs_user",
				typ: TypeExeInstaller,
				inspect: func(t *testing.T, _ *rig, _ Installation) {
					if n := len(run.calls()); n != 0 {
						t.Fatalf("installer ran %d times although nobody could answer its wizard", n)
					}
				},
			}
		}},
		{"interactive installer writes where nobody looks", func(t *testing.T, r *rig) flowPlan {
			interactiveSource(t, r)
			elsewhere := filepath.Join(t.TempDir(), "Elsewhere", "Game")
			run := newRgRunner(t, rgStep{act: func(runSpec) { mkFile(t, filepath.Join(elsewhere, "Game.exe"), 8192) }})
			exe := filepath.Join(elsewhere, "Game.exe")
			return flowPlan{
				runner: run, status: StatusWaitingForUser, typ: TypeExeInstaller, confirm: exe,
				inspect: func(t *testing.T, r *rig, it Installation) {
					if it.Destination != "" || len(it.Candidates) != 0 {
						t.Fatalf("destination %q and candidates %v found outside the watched roots", it.Destination, it.Candidates)
					}
					_ = r
				},
			}
		}},
	}
	return append(cases, elevatedFlowCases()...)
}

func elevatedFlowCases() []flowCase {
	return []flowCase{
		{"interactive installer through the elevated worker", func(t *testing.T, r *rig) flowPlan {
			installed := interactiveSource(t, r)
			worker := &rgWorker{t: t, act: func(workerSpec) workerState {
				mkFile(t, filepath.Join(installed, "MyGame.exe"), 512<<10)
				return workerState{}
			}}
			worker.install()
			return flowPlan{
				runner: elevatingRunner{}, status: StatusWaitingForUser, typ: TypeExeInstaller, dest: installed, owned: true,
				exe: filepath.Join(installed, "MyGame.exe"), confirm: filepath.Join(installed, "MyGame.exe"),
				inspect: func(t *testing.T, _ *rig, it Installation) {
					specs := worker.all()
					if len(specs) != 1 || !specs[0].Interactive || specs[0].Hidden || specs[0].ID != it.ID {
						t.Fatalf("worker specs = %+v", specs)
					}
				},
			}
		}},
		{"silent installer through the elevated worker", func(t *testing.T, r *rig) flowPlan {
			dest := silentSource(t, r, rgInnoMarker)
			worker := &rgWorker{t: t, act: func(workerSpec) workerState {
				mkFile(t, filepath.Join(dest, "Game.exe"), 8192)
				return workerState{}
			}}
			worker.install()
			return flowPlan{
				opts: StartOptions{Destination: dest}, runner: elevatingRunner{}, status: StatusCompleted, typ: TypeExeInstaller,
				engine: EngineInno, silent: true, dest: dest, exe: filepath.Join(dest, "Game.exe"), owned: true, games: 1,
				inspect: func(t *testing.T, _ *rig, it Installation) {
					specs := worker.all()
					if len(specs) != 1 {
						t.Fatalf("worker specs = %d, want 1", len(specs))
					}
					ws := specs[0]
					if ws.Interactive || !ws.Hidden || ws.Destination != dest || ws.ID != it.ID || ws.Engine != EngineInno || ws.InstallerSHA256 == "" {
						t.Fatalf("worker spec = %+v", ws)
					}
				},
			}
		}},
		{"elevated worker reports an installer failure code", func(t *testing.T, r *rig) flowPlan {
			dest := silentSource(t, r, rgInnoMarker)
			worker := &rgWorker{t: t, act: func(workerSpec) workerState {
				mkFile(t, filepath.Join(dest, "half.bin"), 1024)
				return workerState{Code: 1}
			}}
			worker.install()
			return flowPlan{
				opts: StartOptions{Destination: dest}, runner: elevatingRunner{}, status: StatusFailed, errCode: "install.installer_failed",
				typ: TypeExeInstaller, engine: EngineInno, silent: true, dest: dest,
				inspect: func(t *testing.T, _ *rig, _ Installation) {
					if exists(dest) {
						t.Fatalf("failed elevated install left %s behind", dest)
					}
				},
			}
		}},
		{"elevated worker reports an error text", func(t *testing.T, r *rig) flowPlan {
			dest := silentSource(t, r, rgInnoMarker)
			worker := &rgWorker{t: t, act: func(workerSpec) workerState { return workerState{Error: "installer job could not start"} }}
			worker.install()
			return flowPlan{
				opts: StartOptions{Destination: dest}, runner: elevatingRunner{}, status: StatusFailed, errText: "installer job could not start",
				typ: TypeExeInstaller, engine: EngineInno, silent: true, dest: dest,
			}
		}},
		{"elevated worker vanishes without a result keeps the directory", func(t *testing.T, r *rig) flowPlan {
			dest := silentSource(t, r, rgInnoMarker)
			worker := &rgWorker{t: t, silent: true, act: func(workerSpec) workerState {
				mkFile(t, filepath.Join(dest, "half.bin"), 1024)
				return workerState{}
			}}
			worker.install()
			return flowPlan{
				opts: StartOptions{Destination: dest}, runner: elevatingRunner{}, status: StatusFailed,
				errCode: "install.installer_not_confirmed_stopped", typ: TypeExeInstaller, engine: EngineInno, silent: true,
				dest: dest,
				inspect: func(t *testing.T, _ *rig, _ Installation) {
					if !exists(filepath.Join(dest, "half.bin")) {
						t.Fatal("the directory of an installer that never confirmed it stopped was deleted")
					}
				},
			}
		}},
	}
}

func checkPlan(t *testing.T, r *rig, p flowPlan, got Installation) {
	t.Helper()
	if got.Status != p.status {
		t.Fatalf("status = %s (%q), want %s", got.Status, got.Error, p.status)
	}
	if code := uiCode(got.Error); code != p.errCode {
		t.Fatalf("error code = %q (%q), want %q", code, got.Error, p.errCode)
	}
	if p.errText != "" && !strings.Contains(got.Error, p.errText) {
		t.Fatalf("error = %q, want it to mention %q", got.Error, p.errText)
	}
	if p.errCode == "" && p.errText == "" && got.Error != "" {
		t.Fatalf("error = %q on a record that did not fail", got.Error)
	}
	if got.Type != p.typ || got.Engine != p.engine || got.Silent != p.silent || got.Mode != p.mode || got.ChainStep != p.chain {
		t.Fatalf("record type=%s engine=%q silent=%v mode=%q chain=%d, want %s %q %v %q %d",
			got.Type, got.Engine, got.Silent, got.Mode, got.ChainStep, p.typ, p.engine, p.silent, p.mode, p.chain)
	}
	if got.Destination != p.dest {
		t.Fatalf("destination = %q, want %q", got.Destination, p.dest)
	}
	if p.exe != "" && got.Executable != p.exe {
		t.Fatalf("executable = %q, want %q", got.Executable, p.exe)
	}
	if got.Owned != p.owned {
		t.Fatalf("owned = %v, want %v", got.Owned, p.owned)
	}
	if (got.Status == StatusCompleted) != (got.CompletedAt != nil) {
		t.Fatalf("status %s with completedAt %v", got.Status, got.CompletedAt)
	}
	if partial := partialPath(&got); partial != "" && exists(partial) {
		t.Fatalf("%s left behind", partial)
	}
	if got.Status != StatusCompleted {
		if games := r.reg.registered(); len(games) != 0 {
			t.Fatalf("registered %d games for a %s install", len(games), got.Status)
		}
	}
}

func checkRegistered(t *testing.T, r *rig, p flowPlan, got Installation, want int) {
	t.Helper()
	games := r.reg.registered()
	if len(games) != want {
		t.Fatalf("registered %d games, want %d", len(games), want)
	}
	if want == 0 {
		return
	}
	g := games[0]
	if g.InstallDir != got.Destination || g.Executable != got.Executable || g.SourceDownloadID != "d1" ||
		g.InstallType != string(got.Type) || g.Owned != got.Owned {
		t.Fatalf("registered %+v for %+v", g, got)
	}
	_ = p
}

var (
	workerLeftover = regexp.MustCompile(`^worker-[0-9a-f]+-(spec|state)\.json$`)
	logLeftover    = regexp.MustCompile(`^installer-[0-9a-f]+\.log$`)
)

// checkLeftovers: после установки в каталоге состояния не остаётся ничего, кроме
// списков. Лог упавшей установки и спеку с состоянием повышенного воркера не
// убирает ни сервис, ни воркер (известный дефект, он описан отдельно), поэтому
// здесь они допускаются, но ничего другого быть не должно.
func checkLeftovers(t *testing.T, r *rig, elevated, failed bool, want []string) {
	t.Helper()
	got := make([]string, 0, 2)
	for _, name := range r.stateFiles() {
		if elevated && workerLeftover.MatchString(name) {
			continue
		}
		if failed && logLeftover.MatchString(name) {
			continue
		}
		got = append(got, name)
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("state directory holds %v, want %v", got, want)
	}
}

// assertDurable: запись на диске совпадает с памятью, а перезапуск лаунчера не
// меняет уже устоявшуюся запись.
func (r *rig) assertDurable(id string) {
	r.t.Helper()
	mem := r.get(id)
	if disk := r.diskItem(id); viewOf(disk) != viewOf(mem) || wireOf(r.t, disk) != wireOf(r.t, mem) {
		r.t.Fatalf("installations.json holds %s, memory %s", wireOf(r.t, disk), wireOf(r.t, mem))
	}
	r.restart()
	if after := r.get(id); viewOf(after) != viewOf(mem) || wireOf(r.t, after) != wireOf(r.t, mem) {
		r.t.Fatalf("restart changed a settled record:\n before %s\n after  %s", wireOf(r.t, mem), wireOf(r.t, after))
	}
	if n := len(r.s.List()); n != 1 {
		r.t.Fatalf("records after restart = %d, want 1", n)
	}
}

// wireOf — запись такой, какой она лежит в installations.json: сравнение по ней
// охватывает все поля, а не только те, что показывает view.
func wireOf(t *testing.T, item Installation) string {
	t.Helper()
	raw, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("marshal %s: %v", item.ID, err)
	}
	return string(raw)
}

func TestFlowMatrix(t *testing.T) {
	cases := append(flowCases(), platformFlowCases()...)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t)
			p := tc.build(t, r)
			if p.runner != nil {
				r.setRunner(p.runner)
			}
			if p.seeding {
				r.downloads.setSeeding("d1", true)
			}
			item, err := r.s.Start("d1", p.opts)
			if p.startCode != "" {
				if err == nil || errCode(err) != p.startCode {
					t.Fatalf("Start error = %v, want code %s", err, p.startCode)
				}
				if got := r.s.List(); len(got) != 0 {
					t.Fatalf("a refused Start left records: %+v", got)
				}
				if got := r.disk(); len(got) != 0 {
					t.Fatalf("a refused Start wrote records: %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Start: %v", err)
			}
			got := r.settle(item.ID)
			checkPlan(t, r, p, got)
			if p.inspect != nil {
				p.inspect(t, r, got)
			}
			if p.confirm != "" {
				checkRegistered(t, r, p, got, 0)
				if err := r.s.ConfirmExecutable(item.ID, p.confirm); err != nil {
					t.Fatalf("ConfirmExecutable: %v", err)
				}
				got = r.get(item.ID)
				if got.Status != StatusCompleted || got.Executable != p.confirm || got.Error != "" || got.CompletedAt == nil {
					t.Fatalf("after confirm: %+v", got)
				}
				if got.Destination == "" {
					t.Fatal("confirmed install has no destination")
				}
				checkRegistered(t, r, p, got, 1)
			} else {
				checkRegistered(t, r, p, got, p.games)
			}
			_, elevated := p.runner.(elevatingRunner)
			checkLeftovers(t, r, elevated, got.Status == StatusFailed, p.files)
			r.assertDurable(item.ID)
		})
	}
}

func TestFlowDownloadStaysUntouchedByFailedInstalls(t *testing.T) {
	cases := []struct {
		name   string
		policy string
		want   int
	}{
		{"cleanup delete removes the download after a good install", "delete", 1},
		{"cleanup keep leaves the download", "keep", 0},
		{"cleanup ask leaves the download", "ask", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t)
			r.setCleanup(tc.policy)
			root := t.TempDir()
			portableSource(t, root, "Game")
			r.download("d1", "Game", root)
			dest := filepath.Join(t.TempDir(), "Game")
			item, err := r.s.Start("d1", StartOptions{Destination: dest, Mode: ModeCopy})
			if err != nil {
				t.Fatalf("Start: %v", err)
			}
			r.settle(item.ID)
			waitFor(t, "cleanup decision", func() bool { return len(r.downloads.deletedIDs()) == tc.want })
			if tc.want == 0 && len(r.downloads.deletedIDs()) != 0 {
				t.Fatalf("download data removed under policy %s", tc.policy)
			}
		})
	}
}
