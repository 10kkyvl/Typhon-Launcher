package install

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"slices"

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

func (s *Service) usedByOtherLocked(item *Installation) *Installation {
	for _, other := range s.items {
		if other == item || !sharesPath(other.Destination, item.Destination) {
			continue
		}
		if active(other.Status) || other.Status == StatusCompleted {
			return other
		}
	}
	return nil
}

// holdDiscardLocked решает под s.mu, можно ли убирать Destination отменённой
// установки, и пока уборка идёт, закрывает его от новых установок: резерв той
// же .partial-папки, что и у удаления извне, не пускает Start и Retry в
// claimLocked. Возвращает release, либо nil — убирать нечего. Проваленная
// установка файлы оставляет, и Dismiss их не трогает: Retry подхватывает их
// (регистрация в библиотеке отказала уже после переноса), а распакованная
// игра может быть единственной копией. Убирает их только reclaimLeftover, когда
// новая установка занимает ту же папку.
func (s *Service) holdDiscardLocked(item *Installation) func() {
	if item.Status != StatusCancelled {
		return nil
	}
	return s.holdLocked(item)
}

func (s *Service) holdLocked(item *Installation) func() {
	if !controlled(item.Type) || item.Destination == "" || !samePath(item.OwnedDestination, item.Destination) {
		return nil
	}
	if other := s.usedByOtherLocked(item); other != nil {
		slog.Warn("keep the destination of a stopped install, another record uses it",
			"id", item.ID, "destination", item.Destination, "other", other.ID)
		return nil
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
	if err := s.discardBlocker(item); err != nil {
		return err
	}
	if err := s.removeInstallDir(item.Destination); err != nil {
		return err
	}
	return s.forgetCreatedDestination(item.ID, item.Destination)
}

// discardBlocker — все проверки, без которых Destination удалять нельзя; ничего
// не меняет, поэтому PlanSpace опирается на неё же, когда обещает, что Start
// уберёт остаток.
func (s *Service) discardBlocker(item Installation) error {
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
	return removableDir(item.Destination)
}

func (s *Service) leftoverLocked(dest string) *Installation {
	if dest == "" {
		return nil
	}
	for _, item := range s.items {
		if item.Status != StatusFailed || !controlled(item.Type) {
			continue
		}
		if !samePath(item.Destination, dest) || !samePath(item.OwnedDestination, dest) {
			continue
		}
		if s.usedByOtherLocked(item) == nil {
			return item
		}
	}
	return nil
}

func (s *Service) leftoverReusable(dest string) bool {
	s.mu.Lock()
	item := s.leftoverLocked(dest)
	if item == nil {
		s.mu.Unlock()
		return false
	}
	snap := snapshotOf(item)
	s.mu.Unlock()
	return s.discardBlocker(snap) == nil
}

func (s *Service) reclaimLeftover(dest string) (bool, error) {
	s.mu.Lock()
	item := s.leftoverLocked(dest)
	if item == nil {
		s.mu.Unlock()
		return false, nil
	}
	if s.partialReservedLocked(dest + partialSuffix) {
		s.mu.Unlock()
		return false, errPartialReserved
	}
	snap := snapshotOf(item)
	release := s.holdLocked(item)
	s.mu.Unlock()
	if release == nil {
		return false, nil
	}
	defer release()
	if err := s.discardBlocker(snap); err != nil {
		slog.Warn("keep the leftover of a failed install, it cannot be proven safe to remove",
			"id", snap.ID, "destination", dest, "error", err)
		return false, nil
	}
	if err := s.removeInstallDir(dest); err != nil {
		// Без этого следующая непустая папка с тем же именем выглядела бы остатком.
		return false, errors.Join(err, s.forgetCreatedDestination(snap.ID, dest))
	}
	if err := s.dropRecord(snap.ID); err != nil {
		return false, err
	}
	slog.Info("removed the leftover of a failed install before a new one", "id", snap.ID, "destination", dest)
	emit(eventRemoved, RemovedEvent{ID: snap.ID})
	return true, nil
}

func (s *Service) dropRecord(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	item := s.findLocked(id)
	if item == nil {
		return nil
	}
	idx := slices.Index(s.items, item)
	s.items = slices.Delete(s.items, idx, idx+1)
	if err := s.persistLocked(); err != nil {
		s.items = slices.Insert(s.items, idx, item)
		return wrapPersistError(err)
	}
	return nil
}

func (s *Service) prepareDestination(dest, name string) error {
	state, err := s.checkDest(dest)
	if err != nil {
		return fmt.Errorf("%w: %s: %w", errDestUnreadable, dest, err)
	}
	switch state {
	case destNotFolder:
		return fmt.Errorf("%w: %s", errDestNotFolder, dest)
	case destNotEmpty:
	default:
		return nil
	}
	reclaimed, err := s.reclaimLeftover(dest)
	if err != nil {
		return err
	}
	if reclaimed {
		return nil
	}
	suggested, err := s.suggestDestination(dest, name)
	if err != nil {
		return err
	}
	return fmt.Errorf("%w: выберите подпапку, например %s", errDestNotEmpty, suggested)
}

func (s *Service) destinationAdvice(info *SpaceInfo, dest, name string) error {
	info.Suggested = dest
	busy := s.insideGame(dest)
	if !busy {
		state, err := s.checkDest(dest)
		if err != nil {
			return fmt.Errorf("%w: %s: %w", errDestUnreadable, dest, err)
		}
		if state == destNotFolder {
			return fmt.Errorf("%w: %s", errDestNotFolder, dest)
		}
		if state != destNotEmpty {
			return nil
		}
		if s.leftoverReusable(dest) {
			info.Reusable = true
			return nil
		}
	}
	suggested, err := s.suggestDestination(dest, name)
	if err != nil {
		return err
	}
	info.DestinationBusy = true
	info.Suggested = suggested
	return nil
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
