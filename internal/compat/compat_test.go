package compat

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	s, err := NewServiceAt(filepath.Join(t.TempDir(), "compat.json"))
	if err != nil {
		t.Fatalf("NewServiceAt: %v", err)
	}
	return s
}

func TestUnknownUntilTried(t *testing.T) {
	s := newTestService(t)
	if got := s.Status("g1"); got.State != StateUnknown {
		t.Fatalf("State = %q, want %q", got.State, StateUnknown)
	}
	if got := s.Status("g1"); got.Attempts != 0 {
		t.Fatalf("Attempts = %d, want 0", got.Attempts)
	}
}

// Одна неудача — ещё не приговор: игра могла упасть от чего угодно, и
// клеймить её с первого раза значит врать пользователю.
func TestOneFailureIsNotBroken(t *testing.T) {
	s := newTestService(t)
	s.RecordLaunchFailure("g1", "launch_failed", "нет бутыля")

	got := s.Status("g1")
	if got.State != StateUnknown {
		t.Fatalf("State = %q, want %q", got.State, StateUnknown)
	}
	if got.Failures != 1 {
		t.Fatalf("Failures = %d, want 1", got.Failures)
	}
}

func TestTwoFailuresMarkBroken(t *testing.T) {
	s := newTestService(t)
	s.RecordLaunchFailure("g1", "launch_failed", "нет бутыля")
	s.RecordLaunchFailure("g1", "launch_failed", "нет бутыля")

	got := s.Status("g1")
	if got.State != StateBroken {
		t.Fatalf("State = %q, want %q", got.State, StateBroken)
	}
	if got.LastError != "нет бутыля" {
		t.Fatalf("LastError = %q", got.LastError)
	}
}

// Игра, закрывшаяся сама через пару секунд, не запустилась — даже если
// процесс успел появиться.
func TestQuickSelfExitCountsAsFailure(t *testing.T) {
	s := newTestService(t)
	s.RecordSession("g1", 3*time.Second, false)
	s.RecordSession("g1", 2*time.Second, false)

	if got := s.Status("g1"); got.State != StateBroken {
		t.Fatalf("State = %q, want %q", got.State, StateBroken)
	}
}

// Остановка пользователем ничего не говорит о совместимости: он мог закрыть
// игру через десять секунд просто потому, что передумал.
func TestUserStopIsNotAFailure(t *testing.T) {
	s := newTestService(t)
	s.RecordSession("g1", 3*time.Second, true)
	s.RecordSession("g1", 2*time.Second, true)

	if got := s.Status("g1"); got.State != StateUnknown {
		t.Fatalf("State = %q, want %q", got.State, StateUnknown)
	}
}

func TestLongSessionMarksWorking(t *testing.T) {
	s := newTestService(t)
	s.RecordSession("g1", 20*time.Minute, true)

	got := s.Status("g1")
	if got.State != StateWorks {
		t.Fatalf("State = %q, want %q", got.State, StateWorks)
	}
	if got.BestSeconds < 1200 {
		t.Fatalf("BestSeconds = %d", got.BestSeconds)
	}
}

// Один удачный запуск отменяет прошлые неудачи: игра доказала, что работает,
// и держать её в списке сломанных больше не за что.
func TestSuccessClearsBroken(t *testing.T) {
	s := newTestService(t)
	s.RecordLaunchFailure("g1", "launch_failed", "не поехало")
	s.RecordLaunchFailure("g1", "launch_failed", "не поехало")
	if got := s.Status("g1"); got.State != StateBroken {
		t.Fatalf("подготовка: State = %q", got.State)
	}

	s.RecordSession("g1", 30*time.Minute, true)

	if got := s.Status("g1"); got.State != StateWorks {
		t.Fatalf("State = %q, want %q", got.State, StateWorks)
	}
}

func TestBrokenListsOnlyBrokenGames(t *testing.T) {
	s := newTestService(t)
	s.RecordLaunchFailure("broken", "launch_failed", "не поехало")
	s.RecordLaunchFailure("broken", "launch_failed", "не поехало")
	s.RecordSession("fine", time.Hour, true)
	s.RecordLaunchFailure("once", "launch_failed", "разово")

	got := s.Broken()
	if len(got) != 1 || got[0].GameID != "broken" {
		t.Fatalf("Broken() = %+v, want только broken", got)
	}
}

func TestSurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "compat.json")
	first, err := NewServiceAt(path)
	if err != nil {
		t.Fatalf("NewServiceAt: %v", err)
	}
	first.RecordLaunchFailure("g1", "launch_failed", "нет бутыля")
	first.RecordLaunchFailure("g1", "launch_failed", "нет бутыля")

	second, err := NewServiceAt(path)
	if err != nil {
		t.Fatalf("NewServiceAt again: %v", err)
	}
	if got := second.Status("g1"); got.State != StateBroken {
		t.Fatalf("после перезапуска State = %q", got.State)
	}
}

// Битый файл журнала не должен мешать лаунчеру: совместимость — подсказка, а
// не состояние, без которого нельзя работать.
func TestCorruptFileStartsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "compat.json")
	if err := os.WriteFile(path, []byte("{ не json"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	s, err := NewServiceAt(path)
	if err != nil {
		t.Fatalf("NewServiceAt: %v", err)
	}
	if got := s.Status("g1"); got.State != StateUnknown {
		t.Fatalf("State = %q", got.State)
	}
}
