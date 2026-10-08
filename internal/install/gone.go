package install

import (
	"errors"
	"fmt"
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
