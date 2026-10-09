package install

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"typhon/internal/download"
)

// HandleDownloadGone вызывается, когда загрузка исчезла насовсем: установка,
// которая ждёт выбора по её файлам, больше никогда не получит ни выбора, ни
// файлов, а active() не даёт её даже убрать из списка.
//
//wails:ignore
func (s *Service) HandleDownloadGone(downloadID string) error {
	s.DropBroker(downloadID)
	if downloadID == "" {
		return nil
	}
	return s.cancelWaiting(func(item *Installation) bool { return item.DownloadID == downloadID })
}

func (s *Service) cancelWaiting(match func(*Installation) bool) error {
	s.mu.Lock()
	ids := make([]string, 0, 2)
	for _, item := range s.items {
		if item.Status == StatusWaitingForUser && match(item) {
			ids = append(ids, item.ID)
		}
	}
	s.mu.Unlock()
	var errs []error
	for _, id := range ids {
		// Пользователь мог успеть выбрать файл или отменить сам: запись уже
		// ушла из ожидания, и отменять нечего.
		if err := s.Cancel(id); err != nil && !errors.Is(err, errUnavailable) && !errors.Is(err, errNotFound) {
			errs = append(errs, fmt.Errorf("отменить установку %s: %w", id, err))
		}
	}
	return errors.Join(errs...)
}

// releaseOrphanedWaiting освобождает записи, застрявшие до появления
// HandleDownloadGone: загрузка давно удалена, а запись ждёт выбора по её
// файлам. Запись с Destination не трогается: игра уже в библиотеке, выбор
// exe загрузку не читает и может завершить установку. Берёт только загрузки
// записей, прочитанных при старте: у записей,
// созданных позже, удаление загрузки приходит через HandleDownloadGone.
// Отсутствием считается только download.ErrNotFound: неудачный запрос не
// доказывает, что загрузки нет. Менеджер загрузок стартует раньше (порядок
// Services в main.go) и читает свой файл в ServiceStartup.
func (s *Service) releaseOrphanedWaiting(ctx context.Context, downloadIDs []string) error {
	if s.downloads == nil {
		return nil
	}
	var errs []error
	seen := make(map[string]bool, len(downloadIDs))
	for _, id := range downloadIDs {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		if err := ctx.Err(); err != nil {
			return errors.Join(append(errs, err)...)
		}
		_, err := s.downloads.Get(id)
		if err == nil {
			continue
		}
		if !errors.Is(err, download.ErrNotFound) {
			slog.Warn("check download of a waiting install", "download_id", id, "error", err)
			continue
		}
		//nolint:contextcheck // Cancel и HandleDownloadGone берут контекст жизни сервиса из s.ctx (инварианты 19-20); сигнатура Cancel задана биндингом и ctx не принимает
		if err := s.HandleDownloadGone(id); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
