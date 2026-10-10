//go:build windows

package install

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func msiSource(t *testing.T, r *rig) string {
	t.Helper()
	root := t.TempDir()
	mkFile(t, filepath.Join(root, "Game", "installer.msi"), 4096)
	r.download("d1", "Game", root)
	return filepath.Join(r.games, "Game")
}

// msiexec и UAC существуют только в Windows: на других ОС эти строки матрицы
// не имеют смысла, а не просто пропускаются.
func platformFlowCases() []flowCase {
	cases := []flowCase{
		{"msi silent through msiexec", func(t *testing.T, r *rig) flowPlan {
			dest := msiSource(t, r)
			run := newRgRunner(t, rgStep{act: installGame(t, dest), log: "msi log"})
			msiexec, err := systemExecutable("msiexec.exe")
			if err != nil {
				t.Fatalf("msiexec: %v", err)
			}
			return flowPlan{
				opts: StartOptions{Destination: dest}, runner: run, status: StatusCompleted, typ: TypeMsiInstaller,
				engine: EngineMsi, silent: true, dest: dest, exe: filepath.Join(dest, "Game.exe"), owned: true, games: 1,
				inspect: func(t *testing.T, _ *rig, _ Installation) {
					calls := run.calls()
					if len(calls) != 1 || calls[0].Path != msiexec {
						t.Fatalf("msi must go through %s: %+v", msiexec, calls)
					}
					args := strings.Join(calls[0].Args, " ")
					if !strings.HasPrefix(args, "/i ") || !strings.Contains(args, "/qn") || !strings.Contains(args, "TARGETDIR="+dest) {
						t.Fatalf("msiexec args = %q", args)
					}
				},
			}
		}},
		{"interactive installer with the UAC prompt declined", func(t *testing.T, r *rig) flowPlan {
			interactiveSource(t, r)
			worker := &rgWorker{t: t, startErr: windows.ERROR_CANCELLED}
			worker.install()
			return flowPlan{
				runner: elevatingRunner{}, status: StatusFailed, errCode: "install.elevation_declined", typ: TypeExeInstaller,
			}
		}},
		{"silent installer with the UAC prompt declined", func(t *testing.T, r *rig) flowPlan {
			dest := silentSource(t, r, rgInnoMarker)
			worker := &rgWorker{t: t, startErr: windows.ERROR_CANCELLED}
			worker.install()
			return flowPlan{
				opts: StartOptions{Destination: dest}, runner: elevatingRunner{}, status: StatusFailed,
				errCode: "install.elevation_declined", typ: TypeExeInstaller, engine: EngineInno, silent: true, dest: dest,
				inspect: func(t *testing.T, _ *rig, _ Installation) {
					if exists(dest) {
						t.Fatalf("declined install left %s behind", dest)
					}
				},
			}
		}},
	}
	exits := []struct {
		code    int
		success bool
		errCode string
	}{
		{code: 1641, success: true},
		{code: rebootExitCode, success: true},
		{code: 1603, errCode: "install.installer_failed"},
		{code: 1602, errCode: "install.installer_cancelled"},
		{code: 1618, errCode: "install.installer_busy"},
	}
	for _, exit := range exits {
		cases = append(cases, flowCase{fmt.Sprintf("msi exit code %d", exit.code), func(t *testing.T, r *rig) flowPlan {
			dest := msiSource(t, r)
			run := newRgRunner(t, rgStep{act: installGame(t, dest), code: exit.code})
			p := flowPlan{
				opts: StartOptions{Destination: dest}, runner: run, typ: TypeMsiInstaller, engine: EngineMsi, silent: true,
				dest: dest,
			}
			if exit.success {
				p.status, p.exe, p.owned, p.games = StatusCompleted, filepath.Join(dest, "Game.exe"), true, 1
				return p
			}
			p.status, p.errCode = StatusFailed, exit.errCode
			return p
		}})
	}
	return cases
}
