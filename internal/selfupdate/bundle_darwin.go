//go:build darwin && !devmock

package selfupdate

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"typhon/internal/uierr"
)

// Артефакт обновления под macOS — zip с бандлом .app внутри. Не dmg: образ
// пришлось бы монтировать через hdiutil и отмонтировать при любой ошибке, а
// zip разбирается стандартной библиотекой и не оставляет за собой состояния.
var (
	errBundleEscapingPath = uierr.New("selfupdate.bundle_escaping_path", "selfupdate: путь в архиве ведёт наружу")
	errBundleNoApp        = uierr.New("selfupdate.bundle_no_app", "selfupdate: в архиве нет бандла .app")
	errBundleTooLarge     = uierr.New("selfupdate.bundle_too_large", "selfupdate: архив обновления неправдоподобно велик")
)

// bundleSizeLimit ограничивает распакованный размер: архив приходит из сети, и
// заявленный размер в заголовке верить нельзя.
const bundleSizeLimit = 1 << 30

// extractBundle распаковывает архив в dest и отдаёт путь до бандла .app.
func extractBundle(archivePath, dest string) (string, error) {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return "", fmt.Errorf("selfupdate: открыть %s: %w", archivePath, err)
	}
	defer func() {
		if err := reader.Close(); err != nil {
			slog.Debug("close update archive", "path", archivePath, "error", err)
		}
	}()

	root, err := filepath.Abs(dest)
	if err != nil {
		return "", err
	}
	bundle := ""
	written := int64(0)
	for _, file := range reader.File {
		target, err := safeJoin(root, file.Name)
		if err != nil {
			return "", err
		}
		if name, ok := bundleName(file.Name); ok && bundle == "" {
			bundle = filepath.Join(root, name)
		}
		n, err := extractEntry(file, target, root)
		if err != nil {
			return "", err
		}
		written += n
		if written > bundleSizeLimit {
			return "", errBundleTooLarge
		}
	}
	if bundle == "" {
		return "", errBundleNoApp
	}
	return bundle, nil
}

// safeJoin не пускает запись наружу каталога: имена в архиве пишет тот, кто
// его собрал, а мы разбираем его с правами пользователя.
func safeJoin(root, name string) (string, error) {
	if filepath.IsAbs(name) || strings.HasPrefix(name, "/") {
		return "", fmt.Errorf("%w: %s", errBundleEscapingPath, name)
	}
	target := filepath.Join(root, filepath.Clean(name))
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: %s", errBundleEscapingPath, name)
	}
	return target, nil
}

// bundleName находит верхнеуровневый .app: имя бандла нам нужно, чтобы знать,
// что именно подменять, а внутри архива он всегда один.
func bundleName(name string) (string, bool) {
	first, _, found := strings.Cut(filepath.ToSlash(name), "/")
	if !found || !strings.HasSuffix(first, ".app") {
		return "", false
	}
	return first, true
}

func extractEntry(file *zip.File, target, root string) (int64, error) {
	mode := file.Mode()
	switch {
	case mode.IsDir():
		return 0, os.MkdirAll(target, 0o755)
	case mode&os.ModeSymlink != 0:
		return 0, extractSymlink(file, target, root)
	case !mode.IsRegular():
		// Сокеты и устройства в бандле лаунчера взяться неоткуда, а молча
		// пропустить их честнее, чем пытаться воспроизвести.
		return 0, nil
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return 0, err
	}
	src, err := file.Open()
	if err != nil {
		return 0, err
	}
	defer func() {
		if err := src.Close(); err != nil {
			slog.Debug("close archive entry", "name", file.Name, "error", err)
		}
	}()

	//nolint:gosec // G304: target получен через safeJoin внутри нашего временного каталога
	dst, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode.Perm())
	if err != nil {
		return 0, err
	}
	written, copyErr := io.Copy(dst, io.LimitReader(src, bundleSizeLimit+1))
	closeErr := dst.Close()
	if copyErr != nil {
		return written, copyErr
	}
	if closeErr != nil {
		return written, closeErr
	}
	// O_CREATE применяет umask, а бит исполнения лаунчеру нужен точно.
	return written, os.Chmod(target, mode.Perm())
}

func extractSymlink(file *zip.File, target, root string) error {
	src, err := file.Open()
	if err != nil {
		return err
	}
	defer func() {
		if err := src.Close(); err != nil {
			slog.Debug("close archive entry", "name", file.Name, "error", err)
		}
	}()

	raw, err := io.ReadAll(io.LimitReader(src, 4096))
	if err != nil {
		return err
	}
	link := string(raw)
	// Симлинк наружу — тот же побег, только в две ступени: сам файл лежит
	// внутри, а запись по нему уходит куда угодно.
	resolved := link
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(filepath.Dir(target), link)
	}
	rel, relErr := filepath.Rel(root, resolved)
	if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(link) {
		return fmt.Errorf("%w: %s -> %s", errBundleEscapingPath, file.Name, link)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	return os.Symlink(link, target)
}

// swapBundle ставит новый бандл на место старого. Старый сначала отодвигается,
// а не удаляется: если переезд не удался, вернуть его — единственный способ не
// оставить пользователя вообще без лаунчера.
func swapBundle(staged, current string) error {
	if _, err := os.Stat(staged); err != nil {
		return fmt.Errorf("selfupdate: новый бандл недоступен: %w", err)
	}
	backup := fmt.Sprintf("%s.old-%d", current, time.Now().UnixNano())
	hadCurrent := true
	if err := os.Rename(current, backup); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("selfupdate: отодвинуть текущий бандл: %w", err)
		}
		hadCurrent = false
	}
	if err := os.Rename(staged, current); err != nil {
		if hadCurrent {
			if restoreErr := os.Rename(backup, current); restoreErr != nil {
				return fmt.Errorf("selfupdate: подмена не удалась (%w) и старый бандл не вернулся: %w", err, restoreErr)
			}
		}
		return fmt.Errorf("selfupdate: поставить новый бандл: %w", err)
	}
	if hadCurrent {
		// Обновление уже состоялось: оставшийся дубликат — мусор, а не сбой,
		// поэтому в лог, а не в ошибку.
		if err := os.RemoveAll(backup); err != nil {
			slog.Warn("remove replaced bundle", "path", backup, "error", err)
		}
	}
	return nil
}
