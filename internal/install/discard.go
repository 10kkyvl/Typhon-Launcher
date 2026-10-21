package install

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"

	"typhon/internal/platform"
)

var (
	errDiscardNoSource   = errors.New("источник установки недоступен, папка может быть единственной копией")
	errDiscardSourceIn   = errors.New("источник установки лежит внутри папки установки")
	errDiscardReferenced = errors.New("папку установки использует игра из библиотеки")
)

// rememberCreatedDestination записывает, что Destination создан этой установкой:
// пока каталога нет, всё, что в нём появится, принадлежит установке и уходит
// вместе с ней при отмене. Существующий каталог — чужой, его не
// трогаем. Запись переживает Retry и перезапуск (как OwnedDestination у
// установщиков).
func (s *Service) rememberCreatedDestination(id, destination string) error {
	if destination == "" {
		return errEmptyDestination
	}
	_, err := os.Lstat(destination)
	switch {
	case err == nil:
		return nil
	case !errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("stat %s: %w", destination, err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	item := s.findLocked(id)
	if item == nil || item.OwnedDestination == destination {
		return nil
	}
	prev := item.OwnedDestination
	item.OwnedDestination = destination
	if err := s.persistLocked(); err != nil {
		item.OwnedDestination = prev
		return wrapPersistError(err)
	}
	return nil
}

// holdDiscardLocked решает под s.mu, можно ли убирать Destination отменённой
// установки, и пока уборка идёт, закрывает его от новых установок: резерв той
// же .partial-папки, что и у удаления извне, не пускает Start и Retry в
// claimLocked. Возвращает release, либо nil — убирать нечего. Проваленная
// установка файлы оставляет: Retry подхватывает их (регистрация в библиотеке
// отказала уже после переноса).
func (s *Service) holdDiscardLocked(item *Installation) func() {
	if item.Status != StatusCancelled {
		return nil
	}
	if !controlled(item.Type) || item.Destination == "" || !samePath(item.OwnedDestination, item.Destination) {
		return nil
	}
	for _, other := range s.items {
		if other == item || !sharesPath(other.Destination, item.Destination) {
			continue
		}
		if active(other.Status) || other.Status == StatusCompleted {
			slog.Warn("keep the destination of a stopped install, another record uses it",
				"id", item.ID, "destination", item.Destination, "other", other.ID)
			return nil
		}
	}
	key, err := platform.PathKey(item.Destination + partialSuffix)
	if err != nil {
		slog.Warn("keep the destination of a stopped install, cannot reserve it", "id", item.ID, "error", err)
		return nil
	}
	if s.reservedPartials == nil {
		s.reservedPartials = map[string]int{}
	}
	s.reservedPartials[key]++
	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.reservedPartials[key]--; s.reservedPartials[key] <= 0 {
			delete(s.reservedPartials, key)
		}
	}
}

// discardAfter убирает Destination и снимает резерв. Ошибка уборки не меняет
// исхода отмены или провала, поэтому она в журнале, а запись сохраняет
// OwnedDestination: повторный Dismiss попробует снова.
func (s *Service) discardAfter(item Installation, release func()) {
	if release == nil {
		return
	}
	defer release()
	if err := s.discardDestination(item); err != nil {
		slog.Warn("remove destination of a stopped install", "id", item.ID, "destination", item.Destination, "error", err)
		return
	}
	slog.Info("removed destination of a stopped install", "id", item.ID, "destination", item.Destination)
}

// discardDestination удаляет данные, которые установка сама произвела из
// источника: источник обязан быть на месте (иначе папка может быть единственной
// копией, инвариант 9), а библиотека — не ссылаться на каталог (инвариант 16).
func (s *Service) discardDestination(item Installation) error {
	source := item.ContentRoot
	if archived(item.Type) {
		source = item.ArchivePath
	}
	if source == "" {
		return errDiscardNoSource
	}
	if sharesPath(source, item.Destination) {
		return fmt.Errorf("%w: %s", errDiscardSourceIn, source)
	}
	if _, err := os.Stat(source); err != nil {
		return fmt.Errorf("%w: %w", errDiscardNoSource, err)
	}
	if s.library == nil {
		return errNoLibrary
	}
	for _, game := range s.library.GetGames() {
		if sharesPath(game.InstallDir, item.Destination) {
			return fmt.Errorf("%w: %s", errDiscardReferenced, game.Title)
		}
	}
	if err := removableDir(item.Destination); err != nil {
		return err
	}
	if err := s.removeInstallDir(item.Destination); err != nil {
		return err
	}
	return s.forgetCreatedDestination(item.ID, item.Destination)
}

func (s *Service) forgetCreatedDestination(id, destination string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	item := s.findLocked(id)
	if item == nil || !samePath(item.OwnedDestination, destination) {
		return nil
	}
	prev := item.OwnedDestination
	item.OwnedDestination = ""
	if err := s.persistLocked(); err != nil {
		item.OwnedDestination = prev
		return wrapPersistError(err)
	}
	return nil
}
