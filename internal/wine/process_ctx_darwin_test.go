//go:build darwin && !devmock

package wine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Подвисший ps не должен держать цикл детекта и остановку сервиса: перечисление
// процессов обязано вернуться по отмене контекста, а не по завершению ps.
func TestProcessListingReturnsWhenTheContextExpiresWhilePsHangs(t *testing.T) {
	bin := t.TempDir()
	fakePS := filepath.Join(bin, "ps")
	//nolint:forbidigo // тестовый исполняемый скрипт, не состояние пользователя
	if err := os.WriteFile(fakePS, []byte("#!/bin/sh\nsleep 30\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	m, _, _ := newTestManager(t)
	m.psOutput = nil
	ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
	defer cancel()

	started := time.Now()
	_, err := m.AllProcesses(ctx)
	elapsed := time.Since(started)
	if err == nil {
		t.Fatal("AllProcesses returned no error although ps never finished")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("AllProcesses error = %v, want the context deadline", err)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("AllProcesses took %s, the hung ps was not cancelled", elapsed)
	}
}
