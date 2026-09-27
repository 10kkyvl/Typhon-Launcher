//go:build darwin && !devmock

package selfupdate

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"

	"typhon/internal/settings"
	"typhon/internal/uierr"
)

// Apply ставит новую версию на macOS: распаковывает бандл рядом с целевым и
// подменяет его переименованием. Установщика, как на Windows, здесь нет —
// приложение это каталог, и его замена атомарна сама по себе.
//
// Каталог распаковки берётся рядом с бандлом намеренно: переименование
// работает только внутри одного тома, а /Applications и системный временный
// каталог на разных томах бывают регулярно.
//
// Параметр installDir здесь не используется. На Windows это каталог, который
// лаунчер передаёт установщику; на macOS ставить некуда, кроме места, где
// бандл уже лежит, и оно выводится из пути перезапуска.
func Apply(ctx context.Context, archivePath, _, relaunchPath string) error {
	configDir, err := settings.ConfigDir()
	if err != nil {
		return err
	}
	if err := validateInstallerPath(configDir, archivePath); err != nil {
		return err
	}
	store, err := NewStore(configDir)
	if err != nil {
		return err
	}
	st, err := store.Load()
	if err != nil {
		return err
	}
	if st.Artifact == nil || st.ReadyPath == "" || st.ReadyPath != archivePath {
		return ErrNotReady
	}
	if err := VerifyFile(ctx, archivePath, *st.Artifact); err != nil {
		return err
	}

	current, err := bundleOf(relaunchPath)
	if err != nil {
		return err
	}
	parent := filepath.Dir(current)
	if err := validateInstallDir(parent); err != nil {
		return err
	}
	staging, err := os.MkdirTemp(parent, ".typhon-update-")
	if err != nil {
		return fmt.Errorf("selfupdate: каталог распаковки: %w", err)
	}
	defer func() {
		// Каталог распаковки — наш временный мусор: его остаток не повод
		// объявлять обновление несостоявшимся, но и молчать о нём незачем.
		if err := os.RemoveAll(staging); err != nil {
			slog.Warn("remove update staging dir", "path", staging, "error", err)
		}
	}()

	staged, err := extractBundle(archivePath, staging)
	if err != nil {
		return err
	}
	// Карантин переживает распаковку и остаётся на новом бандле: без снятия
	// Gatekeeper встретит обновлённый лаунчер как только что скачанный.
	dropQuarantine(ctx, staged)

	if err := swapBundle(staged, current); err != nil {
		return uierr.Wrap("selfupdate.installer_failed", err)
	}
	return nil
}

// bundleOf поднимается от исполняемого файла к бандлу:
// Typhon.app/Contents/MacOS/typhon.
func bundleOf(relaunchPath string) (string, error) {
	if relaunchPath == "" {
		return "", uierr.New("selfupdate.relaunch_path_empty", "selfupdate: relaunch path is empty")
	}
	dir := filepath.Dir(filepath.Dir(filepath.Dir(relaunchPath)))
	if filepath.Ext(dir) != ".app" {
		return "", uierr.New("selfupdate.not_a_bundle", "selfupdate: лаунчер запущен не из бандла .app")
	}
	return dir, nil
}

// dropQuarantine — удобство, а не часть установки: снять атрибут может не
// получиться, и объявлять из-за этого обновление несостоявшимся нельзя.
func dropQuarantine(ctx context.Context, path string) {
	//nolint:gosec // G204: path — наш собственный каталог распаковки
	if err := exec.CommandContext(ctx, "/usr/bin/xattr", "-dr", "com.apple.quarantine", path).Run(); err != nil {
		slog.Warn("drop quarantine", "path", path, "error", err)
	}
}
