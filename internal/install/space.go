package install

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"typhon/internal/download"
	"typhon/internal/uierr"
)

// moveReserveBytes — запас под служебные файлы, который переносу в пределах
// одного тома нужен вместо места под копию.
const moveReserveBytes = 64 << 20

var (
	errMoveSource  = uierr.New("install.move_source_unavailable", "перенос невозможен: данные загрузки лежат не в её собственной папке, выберите копирование")
	errUnknownMode = uierr.New("install.mode_invalid", "неизвестный режим установки")

	errRenameRefused = errors.New("не удалось переименовать папку загрузки")
)

// moveEligible — можно ли отдать ContentRoot переименованием или удалением:
// только когда он целиком лежит в папке, которую загрузка создала сама. Для
// flat- и in-place-загрузок это был бы общий каталог загрузок (инвариант 16).
func moveEligible(d download.Download, contentRoot string) bool {
	if d.InPlace || d.Flat || contentRoot == "" {
		return false
	}
	root, ok := d.DataRoot()
	if !ok {
		return false
	}
	return inside(root, contentRoot)
}

// resolveMode выбирает режим портативной установки. Пустой запрос — перенос,
// если он возможен и загрузку не раздают; явный «move» при раздаче заменяется
// копированием (файлы нужны торрент-клиенту), а при небезопасном источнике
// отклоняется.
func resolveMode(requested string, d download.Download, contentRoot string) (string, error) {
	switch requested {
	case "":
		if d.Seeding || !moveEligible(d, contentRoot) {
			return ModeCopy, nil
		}
		return ModeMove, nil
	case ModeCopy:
		return ModeCopy, nil
	case ModeMove:
		if d.Seeding {
			return ModeCopy, nil
		}
		if !moveEligible(d, contentRoot) {
			return "", errMoveSource
		}
		return ModeMove, nil
	default:
		return "", fmt.Errorf("%w: %q", errUnknownMode, requested)
	}
}

// spaceNeeded — единственное место, где решается, сколько места нужно под
// установку: перенос в пределах тома не копирует данные.
func spaceNeeded(plan Plan, mode string, sameVolume bool) int64 {
	if plan.Type == TypePortable && mode == ModeMove && sameVolume {
		return moveReserveBytes
	}
	return requiredBytes(plan)
}

func cleanDestination(raw string) (string, error) {
	dest := strings.TrimSpace(raw)
	if dest == "" {
		return "", errNoDestination
	}
	if !filepath.IsAbs(dest) {
		return "", errRelativeDestination
	}
	return filepath.Clean(dest), nil
}

// PlanSpace пересчитывает место под установку по папке, которую пользователь
// ввёл в окне: том берётся от неё, а не от папки игр из настроек.
func (s *Service) PlanSpace(downloadID, destination, mode string) (SpaceInfo, error) {
	dest, err := cleanDestination(destination)
	if err != nil {
		return SpaceInfo{}, err
	}
	d, err := s.completedDownload(downloadID)
	if err != nil {
		return SpaceInfo{}, err
	}
	base, err := s.baseContext()
	if err != nil {
		return SpaceInfo{}, err
	}
	source, err := sourceDir(d)
	if err != nil {
		return SpaceInfo{}, err
	}
	plan, err := Inspect(base, source)
	if err != nil {
		return SpaceInfo{}, err
	}
	resolved := ""
	if plan.Type == TypePortable {
		resolved, err = resolveMode(mode, d, plan.ContentRoot)
		if err != nil {
			return SpaceInfo{}, err
		}
	}
	same := s.sameVolume(plan.ContentRoot, dest)
	need := spaceNeeded(plan, resolved, same)
	free, err := s.freeBytes(dest)
	if err != nil {
		return SpaceInfo{}, err
	}
	info := SpaceInfo{
		RequiredBytes: need,
		FreeBytes:     free,
		SameVolume:    same,
		Mode:          resolved,
		Enough:        free >= need,
		Suggested:     dest,
	}
	if ownDestination(plan.Type, plan.Silent) || plan.Type == TypeUnknown {
		if err := s.destinationAdvice(&info, dest, s.nameFor(d)); err != nil {
			return SpaceInfo{}, err
		}
	}
	return info, nil
}
