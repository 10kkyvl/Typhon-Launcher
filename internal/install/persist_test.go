package install

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// requireUnwritableDir делает dir недоступным для записи через chmod: на
// Windows права POSIX не действуют так же, а под root chmod 0o500 не мешает
// записи вовсе, поэтому обе ситуации проверяются внутри теста (а не через
// t.Skip всего теста на уровне сборки) и пропускают именно этот сценарий.
func requireUnwritableDir(t *testing.T, dir string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX-права каталога не действуют на windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("запущено от root: chmod не мешает записи")
	}
	// G302: тест инварианта 5 (ошибка persist доходит до вызывающего) требует непроходимого
	// для записи каталога; 0600 снял бы execute-бит и запретил бы доступ к каталогу целиком.
	if err := os.Chmod(dir, 0o500); err != nil { //nolint:gosec // см. комментарий выше
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(dir, 0o700); err != nil { //nolint:gosec // G302: возврат прав, выставленных выше по той же причине (инвариант 5)
			t.Errorf("restore chmod: %v", err)
		}
	})
}

// TestSetStatusRollsBackAndReturnsPersistError закрывает finding 8: persist,
// проваленный из-за отсутствия прав на каталог состояния, не должен молча
// оставлять запись в новом (непронесённом на диск) статусе — setStatus
// обязан откатить память и вернуть ошибку вызывающему шагу job'а.
func TestSetStatusRollsBackAndReturnsPersistError(t *testing.T) {
	dir := t.TempDir()
	s := mustServiceAt(t, dir)

	const id = "persist1"
	s.mu.Lock()
	s.items = append(s.items, &Installation{ID: id, Name: "Game", Status: StatusPending})
	s.mu.Unlock()

	requireUnwritableDir(t, dir)

	err := s.setStatus(id, StatusPreparing)
	if err == nil {
		t.Fatal("setStatus error = nil, want a persist error")
	}
	if !strings.Contains(err.Error(), "не удалось сохранить состояние установки") {
		t.Fatalf("error = %v, want the persist-state wrapper text", err)
	}

	got, ok := s.snapshot(id)
	if !ok {
		t.Fatal("record disappeared")
	}
	if got.Status != StatusPending {
		t.Fatalf("status = %s, want rolled back to %s", got.Status, StatusPending)
	}
}

// TestJobFailsWhenStateCannotBePersisted закрывает finding 8 сквозь весь
// путь: setStatus, вызванный изнутри run(), не имеет смысла обрабатывать
// как успех — job обязан провалиться с текстом ошибки persist, который
// дойдёт до пользователя через item.Error.
func TestJobFailsWhenStateCannotBePersisted(t *testing.T) {
	dir := t.TempDir()
	s := mustServiceAt(t, dir)
	s.settings = newTestSettings(t)
	s.library = &fakeRegistrar{}

	root := t.TempDir()
	source := portableSource(t, root, "Game")
	dest := filepath.Join(t.TempDir(), "Game")

	const id = "persistjob1"
	s.mu.Lock()
	s.items = append(s.items, &Installation{
		ID: id, DownloadID: "d1", Name: "Game", Type: TypePortable, Mode: ModeCopy,
		Status: StatusPending, ContentRoot: source, Destination: dest, StartedAt: time.Now(),
	})
	s.mu.Unlock()

	requireUnwritableDir(t, dir)

	s.run(context.Background(), id)

	got, ok := s.snapshot(id)
	if !ok {
		t.Fatal("record disappeared")
	}
	if got.Status != StatusFailed {
		t.Fatalf("status = %s, want %s", got.Status, StatusFailed)
	}
	if !strings.Contains(got.Error, "не удалось сохранить состояние установки") {
		t.Fatalf("error = %q, want the persist-state wrapper text", got.Error)
	}
}

// TestStartRollsBackAppendWhenPersistFails проверяет откат аппенда в память
// (инвариант 4): если новую запись не удалось сохранить, Start не должен
// оставлять её в s.items, будто она была принята.
func TestStartRollsBackAppendWhenPersistFails(t *testing.T) {
	s, downloads, _ := newTestService(t)
	root := t.TempDir()
	portableSource(t, root, "Game")
	downloads.add("d1", "Game", root)

	requireUnwritableDir(t, s.store.dir)

	dest := filepath.Join(t.TempDir(), "Game")
	if _, err := s.Start("d1", StartOptions{Destination: dest, Mode: ModeCopy}); err == nil {
		t.Fatal("Start error = nil, want a persist error")
	} else if !strings.Contains(err.Error(), "не удалось сохранить состояние установки") {
		t.Fatalf("error = %v, want the persist-state wrapper text", err)
	}

	if got := s.List(); len(got) != 0 {
		t.Fatalf("items = %+v, want the append rolled back", got)
	}
}
