package install

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/wailsapp/wails/v3/pkg/application"

	"typhon/internal/uierr"
)

type recordedLog struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *recordedLog) Enabled(context.Context, slog.Level) bool { return true }
func (h *recordedLog) WithAttrs([]slog.Attr) slog.Handler       { return h }
func (h *recordedLog) WithGroup(string) slog.Handler            { return h }
func (h *recordedLog) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r.Clone())
	return nil
}

func (h *recordedLog) errorAttr(t *testing.T, msg string) error {
	t.Helper()
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, r := range h.records {
		if r.Message != msg {
			continue
		}
		var found error
		r.Attrs(func(a slog.Attr) bool {
			if a.Key != "error" {
				return true
			}
			if err, ok := a.Value.Any().(error); ok {
				found = err
			}
			return false
		})
		return found
	}
	t.Fatalf("no %q record was logged", msg)
	return nil
}

func captureSlog(t *testing.T) *recordedLog {
	t.Helper()
	h := &recordedLog{}
	previous := slog.Default()
	slog.SetDefault(slog.New(h))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return h
}

func bigInnoLog(last string) string {
	var b strings.Builder
	b.WriteString("2026-10-08 12:00:01.000 Log opened.\r\n")
	b.WriteString(`2026-10-08 12:00:02.000 Source filename: C:\Users\egor\Downloads\setup.exe` + "\r\n")
	b.WriteString(`2026-10-08 12:00:03.000 Extracting C:\Games\Dark Quest\data.bin` + "\r\n")
	b.WriteString("2026-10-08 12:00:04.000 Connecting to 192.168.1.15\r\n")
	for i := range 400 {
		fmt.Fprintf(&b, "2026-10-08 12:01:%02d.000 Filler line %d to push the head out of the tail\r\n", i%60, i)
	}
	b.WriteString("2026-10-08 12:02:00.000 " + last + "\r\n")
	return b.String()
}

func checkReportedTail(t *testing.T, err error, last string) {
	t.Helper()
	if err == nil {
		t.Fatal("the failure record has no error attribute, so the report would carry no installer log")
	}
	text := err.Error()
	if !strings.Contains(text, last) {
		t.Fatalf("error attribute does not carry the end of the installer log %q:\n%s", last, text)
	}
	for _, leak := range []string{"egor", "192.168.1.15", "Dark Quest"} {
		if strings.Contains(text, leak) {
			t.Fatalf("error attribute leaks %q:\n%s", leak, text)
		}
	}
	if strings.Contains(text, "Log opened") {
		t.Fatalf("error attribute carries the head of the log, not its end:\n%s", text)
	}
	if len(text) > 1800 {
		t.Fatalf("error attribute is %d bytes, over the budget a report message has", len(text))
	}
	if !errors.Is(err, errInstallerFail) {
		t.Fatalf("error attribute lost the exit cause: %v", err)
	}
	if got := uierr.Code(err); got != "install.installer_failed" {
		t.Fatalf("uierr code = %q", got)
	}
}

func TestSilentInstallerFailureReportCarriesLogTail(t *testing.T) {
	const last = "Rolling back changes."
	rec := captureSlog(t)
	s, downloads, _ := newTestService(t)
	root := t.TempDir()
	innoSource(t, root)
	downloads.add("d1", "Game", root)

	games := t.TempDir()
	dest := filepath.Join(games, "Game")
	s.roots = []string{games}
	s.runner = &fakeRunner{code: 1, act: func(spec runSpec) {
		writeUTF16(t, argValue(spec, "/LOG="), bigInnoLog(last))
	}}

	item, err := s.Start("d1", StartOptions{Destination: dest})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	failed := s.waitStatus(t, item.ID, StatusFailed)
	if strings.Contains(failed.Error, last) {
		t.Fatalf("the installer log leaked into the user-facing error: %q", failed.Error)
	}
	checkReportedTail(t, rec.errorAttr(t, "silent installer failed"), last)
}

func TestResumedSilentInstallerFailureReportCarriesLogTail(t *testing.T) {
	const last = "Rolling back changes."
	rec := captureSlog(t)
	dir := t.TempDir()
	s := mustServiceAt(t, dir)
	s.settings = newTestSettings(t)
	s.downloads = newFakeDownloads()
	s.library = &fakeRegistrar{}

	dest := filepath.Join(t.TempDir(), "Game")
	exe := filepath.Join(dest, "Game.exe")
	mkFile(t, exe, 4096)

	const id = "resumed-failed-with-log"
	item := Installation{
		ID: id, DownloadID: "d1", Name: "Game", Type: TypeExeInstaller,
		Status: StatusInstalling, Destination: dest, Executable: exe,
		Engine: EngineInno, Silent: true, StartedAt: time.Now(),
	}
	if err := s.store.save([]Installation{item}); err != nil {
		t.Fatalf("seed store: %v", err)
	}
	if err := writeWorkerState(s.workerStatePath(id), workerState{PID: 999999999, Done: true, Code: 1}); err != nil {
		t.Fatalf("write worker state: %v", err)
	}
	if err := os.WriteFile(s.installerLogPath(id), []byte(bigInnoLog(last)), 0o600); err != nil {
		t.Fatalf("write installer log: %v", err)
	}

	restore := resumeWatchPollInterval
	resumeWatchPollInterval = 10 * time.Millisecond
	t.Cleanup(func() { resumeWatchPollInterval = restore })

	if err := s.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatalf("startup: %v", err)
	}
	t.Cleanup(func() {
		if err := s.ServiceShutdown(); err != nil {
			t.Errorf("shutdown: %v", err)
		}
	})
	s.waitStatus(t, id, StatusFailed)
	checkReportedTail(t, rec.errorAttr(t, "resumed silent installer failed"), last)
}

func TestReportLogTail(t *testing.T) {
	long := strings.Repeat("строка журнала установщика\n", 200) + "последняя строка"
	tests := []struct {
		name      string
		in        string
		want      string
		wantEmpty bool
		maxLen    int
	}{
		{name: "empty", in: "", wantEmpty: true},
		{name: "short", in: "Rolling back changes.", want: "Rolling back changes.", maxLen: installerReportTailLimit},
		{name: "path is redacted", in: `Extracting C:\Users\egor\AppData\Local\Temp\a.bin: access denied`, want: "<path>", maxLen: installerReportTailLimit},
		{name: "long keeps the end and cuts on a rune", in: long, want: "последняя строка", maxLen: installerReportTailLimit},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := reportLogTail(tc.in)
			if tc.wantEmpty {
				if got != "" {
					t.Fatalf("reportLogTail = %q, want empty", got)
				}
				return
			}
			if !strings.Contains(got, tc.want) {
				t.Fatalf("reportLogTail = %q, want it to contain %q", got, tc.want)
			}
			if strings.Contains(got, "egor") {
				t.Fatalf("reportLogTail leaks the user name: %q", got)
			}
			if len(got) > tc.maxLen {
				t.Fatalf("reportLogTail is %d bytes, limit %d", len(got), tc.maxLen)
			}
			if !utf8.ValidString(got) {
				t.Fatalf("reportLogTail cut inside a rune: %q", got)
			}
		})
	}
}
