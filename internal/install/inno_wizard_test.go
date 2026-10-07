package install

import (
	"context"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"unicode/utf16"

	"github.com/wailsapp/wails/v3/pkg/application"

	"typhon/internal/uierr"
)

func utf16Log(text string) []byte {
	units := utf16.Encode([]rune(text))
	out := []byte{0xff, 0xfe}
	for _, u := range units {
		out = binary.LittleEndian.AppendUint16(out, u)
	}
	return out
}

const wizardRefusedLog = "2026-10-07 10:00:01.123   Starting the installation process.\n" +
	"2026-10-07 10:00:01.456   Failed to proceed to next wizard page; showing wizard.\n" +
	"2026-10-07 10:00:01.457   Got EAbort exception.\n"

func TestInstallerFailureClassifiesWizardRefusal(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, data []byte) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	utf16Path := write("utf16.log", utf16Log(wizardRefusedLog))
	utf8Path := write("utf8.log", []byte(wizardRefusedLog))
	plainPath := write("plain.log", utf16Log("2026-10-07 10:00:01.123   Starting the installation process.\n"))
	emptyPath := write("empty.log", nil)

	for _, tc := range []struct {
		name      string
		engine    Engine
		code      int
		log       string
		wantNil   bool
		wantIs    error
		wantNotIs error
	}{
		{name: "utf16 log with the marker", engine: EngineInno, code: 1, log: utf16Path, wantIs: errInstallerNeedsInteractive, wantNotIs: errInstallerFail},
		{name: "utf16 log, cancel code does not hide the marker", engine: EngineInno, code: 2, log: utf16Path, wantIs: errInstallerNeedsInteractive, wantNotIs: errInstallerCancelled},
		{name: "utf8 log with the marker", engine: EngineInno, code: 4, log: utf8Path, wantIs: errInstallerNeedsInteractive},
		{name: "log without the marker", engine: EngineInno, code: 1, log: plainPath, wantIs: errInstallerFail, wantNotIs: errInstallerNeedsInteractive},
		{name: "empty log", engine: EngineInno, code: 1, log: emptyPath, wantIs: errInstallerFail, wantNotIs: errInstallerNeedsInteractive},
		{name: "no log file", engine: EngineInno, code: 1, log: filepath.Join(dir, "missing.log"), wantIs: errInstallerFail, wantNotIs: errInstallerNeedsInteractive},
		{name: "no log path", engine: EngineInno, code: 1, log: "", wantIs: errInstallerFail, wantNotIs: errInstallerNeedsInteractive},
		{name: "unreadable log path", engine: EngineInno, code: 1, log: dir, wantIs: errInstallerFail, wantNotIs: errInstallerNeedsInteractive},
		{name: "other engines do not read an Inno log", engine: EngineNsis, code: 1, log: utf16Path, wantIs: errInstallerFail, wantNotIs: errInstallerNeedsInteractive},
		{name: "success has no failure", engine: EngineInno, code: 0, log: utf16Path, wantNil: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := installerFailure(tc.engine, tc.code, tc.log)
			if tc.wantNil {
				if err != nil {
					t.Fatalf("installerFailure = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, tc.wantIs) {
				t.Fatalf("installerFailure = %v, want %v", err, tc.wantIs)
			}
			if tc.wantNotIs != nil && errors.Is(err, tc.wantNotIs) {
				t.Fatalf("installerFailure = %v, must not be %v", err, tc.wantNotIs)
			}
		})
	}
}

func TestInstallerFailureKeepsExitCodeInTheMessage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "setup.log")
	if err := os.WriteFile(path, utf16Log(wizardRefusedLog), 0o600); err != nil {
		t.Fatal(err)
	}
	err := installerFailure(EngineInno, 4, path)
	if code := uierr.Code(err); code != "install.installer_needs_interactive" {
		t.Fatalf("ui code = %q", code)
	}
	if !strings.Contains(err.Error(), "код 4") {
		t.Fatalf("message %q lost the exit code", err.Error())
	}
}

func TestInnoExitMessages(t *testing.T) {
	for _, tc := range []struct {
		code int
		want string
	}{
		{1, "установщик не смог запуститься"},
		{3, "критическая ошибка при подготовке к следующему этапу"},
		{4, "критическая ошибка во время установки"},
		{6, "прервано отладчиком"},
		{7, "проверка перед установкой не разрешила продолжить"},
		{8, "проверка перед установкой не разрешила продолжить без перезагрузки"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			err := exitError(EngineInno, tc.code)
			if !errors.Is(err, errInstallerFail) {
				t.Fatalf("exitError(inno, %d) = %v, want %v", tc.code, err, errInstallerFail)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("exitError(inno, %d) = %q, want description %q", tc.code, err.Error(), tc.want)
			}
		})
	}
	for _, code := range []int{2, 5} {
		if err := exitError(EngineInno, code); !errors.Is(err, errInstallerCancelled) {
			t.Fatalf("exitError(inno, %d) = %v, want %v", code, err, errInstallerCancelled)
		}
	}
}

type scriptRunner struct {
	mu    sync.Mutex
	steps []func(spec runSpec) (int, error)
	specs []runSpec
}

func (r *scriptRunner) run(_ context.Context, spec runSpec) (int, error) {
	r.mu.Lock()
	n := len(r.specs)
	r.specs = append(r.specs, spec)
	steps := r.steps
	r.mu.Unlock()
	if n >= len(steps) {
		return 1, errors.New("scriptRunner: unexpected extra run")
	}
	return steps[n](spec)
}

func (r *scriptRunner) calls() []runSpec {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]runSpec(nil), r.specs...)
}

func failWithWizardRefusal(spec runSpec) (int, error) {
	if err := os.WriteFile(spec.LogPath, utf16Log(wizardRefusedLog), 0o600); err != nil {
		return 0, err
	}
	return 1, nil
}

func TestSilentInnoWizardRefusalFailsWithNeedsInteractive(t *testing.T) {
	s, downloads, _ := newTestService(t)
	root := t.TempDir()
	innoSource(t, root)
	downloads.add("d1", "Game", root)
	games := t.TempDir()
	s.roots = []string{games}
	s.runner = &scriptRunner{steps: []func(runSpec) (int, error){failWithWizardRefusal}}

	item, err := s.Start("d1", StartOptions{Destination: filepath.Join(games, "Game")})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	failed := s.waitStatus(t, item.ID, StatusFailed)
	if !strings.HasPrefix(failed.Error, "typhon:install.installer_needs_interactive:") {
		t.Fatalf("error = %q, want the needs-interactive code", failed.Error)
	}
}

func TestRetryInteractiveRunsInstallerWithoutSilentKeys(t *testing.T) {
	s, downloads, _ := newTestService(t)
	root := t.TempDir()
	innoSource(t, root)
	downloads.add("d1", "Game", root)
	games := t.TempDir()
	dest := filepath.Join(games, "Game")
	s.roots = []string{games}
	runner := &scriptRunner{steps: []func(runSpec) (int, error){
		failWithWizardRefusal,
		func(runSpec) (int, error) {
			writeSized(t, filepath.Join(games, "ByWizard", "Game.exe"), 512<<10)
			return 0, nil
		},
		func(runSpec) (int, error) { return 5, nil },
	}}
	s.runner = runner

	item, err := s.Start("d1", StartOptions{Destination: dest})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	s.waitStatus(t, item.ID, StatusFailed)
	s.waitJobDone(t, item.ID)

	if err := s.RetryInteractive(item.ID); err != nil {
		t.Fatalf("RetryInteractive: %v", err)
	}
	waiting := s.waitStatus(t, item.ID, StatusWaitingForUser)
	s.waitJobDone(t, item.ID)

	calls := runner.calls()
	if len(calls) != 2 {
		t.Fatalf("runner calls = %d, want 2", len(calls))
	}
	if strings.Contains(strings.Join(calls[1].Args, " "), "/VERYSILENT") || calls[1].Background || calls[1].Hidden {
		t.Fatalf("manual attempt got silent launch parameters: %+v", calls[1])
	}
	if !waiting.Interactive || waiting.Silent || waiting.Error != "" {
		t.Fatalf("record after RetryInteractive = %+v", waiting)
	}
	if waiting.Destination != filepath.Join(games, "ByWizard") {
		t.Fatalf("destination = %q, want the folder the wizard chose", waiting.Destination)
	}

	s.mu.Lock()
	s.findLocked(item.ID).Status = StatusFailed
	s.mu.Unlock()
	if err := s.Retry(item.ID); err != nil {
		t.Fatalf("Retry: %v", err)
	}
	cancelled := s.waitStatus(t, item.ID, StatusFailed)
	s.waitJobDone(t, item.ID)
	calls = runner.calls()
	if len(calls) != 3 || strings.Contains(strings.Join(calls[2].Args, " "), "/VERYSILENT") {
		t.Fatalf("plain Retry went back to silent keys: %+v", calls)
	}
	if !cancelled.Interactive || !strings.HasPrefix(cancelled.Error, "typhon:install.installer_cancelled:") {
		t.Fatalf("exit code 5 of a manual run = %+v, want cancelled", cancelled)
	}
}

func TestRetryInteractiveRefusesWhatIsNotAFailedInstaller(t *testing.T) {
	for _, tc := range []struct {
		name string
		item Installation
		id   string
		want error
	}{
		{name: "unknown id", id: "nope", want: errNotFound},
		{name: "completed installer", item: Installation{ID: "a", Type: TypeExeInstaller, Status: StatusCompleted}, id: "a", want: errUnavailable},
		{name: "running installer", item: Installation{ID: "a", Type: TypeExeInstaller, Status: StatusInstalling}, id: "a", want: errUnavailable},
		{name: "waiting for user", item: Installation{ID: "a", Type: TypeExeInstaller, Status: StatusWaitingForUser}, id: "a", want: errUnavailable},
		{name: "cancelled installer", item: Installation{ID: "a", Type: TypeExeInstaller, Status: StatusCancelled}, id: "a", want: errUnavailable},
		{name: "failed archive", item: Installation{ID: "a", Type: TypeArchiveZip, Status: StatusFailed}, id: "a", want: errUnavailable},
		{name: "failed portable", item: Installation{ID: "a", Type: TypePortable, Status: StatusFailed}, id: "a", want: errUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, _, _ := newTestService(t)
			if tc.item.ID != "" {
				s.mu.Lock()
				s.items = append(s.items, &tc.item)
				s.mu.Unlock()
			}
			err := s.RetryInteractive(tc.id)
			if !errors.Is(err, tc.want) {
				t.Fatalf("RetryInteractive = %v, want %v", err, tc.want)
			}
			if tc.item.ID != "" {
				got, _ := s.snapshot(tc.item.ID)
				if got.Interactive {
					t.Fatalf("refused call still marked the record interactive: %+v", got)
				}
			}
		})
	}
}

func TestInterruptedRecordCarriesUICode(t *testing.T) {
	dir := t.TempDir()
	st := newStore(dir)
	if err := st.save([]Installation{{ID: "a", Name: "Game", Type: TypeExeInstaller, Status: StatusInstalling}}); err != nil {
		t.Fatal(err)
	}
	s := mustServiceAt(t, dir)
	if err := s.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatalf("startup: %v", err)
	}
	t.Cleanup(func() {
		if err := s.ServiceShutdown(); err != nil {
			t.Errorf("shutdown: %v", err)
		}
	})
	got, _ := s.snapshot("a")
	if got.Status != StatusInterrupted || !strings.HasPrefix(got.Error, "typhon:install.interrupted:") {
		t.Fatalf("interrupted record = %+v, want the interrupted ui code", got)
	}
}
