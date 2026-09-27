package install

import (
	"testing"

	"typhon/internal/download"
	"typhon/internal/platform"
)

// TestHandleDownloadCompletedTreatsZeroFreeSpaceAsFull закрывает находку:
// HandleDownloadCompleted пропускал предупреждение "not enough space" при
// info.FreeBytes == 0 (условие требовало FreeBytes > 0), хотя s.freeBytes
// честно пробрасывает ошибку получения свободного места и никогда не
// подменяет её нулём (service.go, freeBytes/checkSpace) — то есть
// FreeBytes == 0 здесь всегда означает "том забит под ноль", а не "неизвестно".
// При старом условии авто-установка всё равно стартовала и обращалась к
// freeBytes второй раз изнутри Start->checkSpace: подсчёт вызовов freeSpace
// отличает "остановились на предупреждении" (1 вызов) от "пошли пытаться
// установить, провалились уже в Start" (2 вызова).
func TestHandleDownloadCompletedTreatsZeroFreeSpaceAsFull(t *testing.T) {
	s, downloads, _ := newTestService(t)
	root := t.TempDir()
	portableSource(t, root, "Game")

	yes := true
	downloads.mu.Lock()
	downloads.items["d1"] = download.Download{
		ID: "d1", Name: "Game", Destination: root, Status: download.StatusCompleted,
		Origin: download.Origin{AutoInstall: &yes},
	}
	downloads.mu.Unlock()

	calls := 0
	s.freeSpace = func(string) (platform.StorageInfo, error) {
		calls++
		return platform.StorageInfo{FreeBytes: 0}, nil
	}

	s.HandleDownloadCompleted(downloads.items["d1"])

	if calls != 1 {
		t.Fatalf("freeSpace called %d times, want 1: a fully-zero volume must be treated as no space and stop right at HandleDownloadCompleted's own check, not fall through to Start", calls)
	}
	if got := s.List(); len(got) != 0 {
		t.Fatalf("installations = %+v, want none: auto install must not have started", got)
	}
}
