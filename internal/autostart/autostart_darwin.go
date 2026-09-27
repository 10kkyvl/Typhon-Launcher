//go:build darwin

package autostart

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"typhon/internal/storage"
)

// На macOS встроенный в Wails механизм автозапуска (SMAppService) выбирает
// себя, когда приложение запущено из .app бандла на macOS 13+, и при ЛЮБОЙ
// ошибке регистрации, кроме «SMAppService недоступен на этой версии macOS»
// (только для macOS <13), не пытается упасть обратно на LaunchAgent —
// см. github.com/wailsapp/wails/v3@v3.0.0-beta.17/pkg/application/autostart_darwin.go:43-58.
// SMAppService при этом регистрирует приложение по коду и требует хотя бы
// ad-hoc подписи, а после каждой пересборки (включая каждое самообновление)
// сравнивает подпись заново; без стабильного Team ID это документированно
// приводит к тому, что система на каждой пересборке заводит новый Login
// Item вместо обновления старого, а не к предсказуемому переживанию
// обновления (github.com/sindresorhus/LaunchAtLogin-Legacy issue #100).
// У Typhon подписи не будет вообще — только ad-hoc codesign при каждой
// сборке (build/darwin/Taskfile.yml, README.md) — и это решённое
// ограничение, а не временное. Поэтому здесь используется собственная,
// более простая реализация: всегда LaunchAgent plist в
// ~/Library/LaunchAgents, без SMAppService вообще. Она не зависит от
// подписи и переживает самообновление, потому что self-update подменяет
// содержимое бандла через os.Rename по тому же абсолютному пути
// (internal/selfupdate/bundle_darwin.go) — путь в plist остаётся верным
// без повторной регистрации.
func init() {
	platformManager = func(Manager) Manager { return darwinLaunchAgent{} }
}

// darwinLabel — Label в LaunchAgent plist и базовое имя файла. Совпадает с
// CFBundleIdentifier (build/darwin/Info.plist) и keychainService
// (internal/account/credential_darwin.go): один macOS-идентификатор на все
// артефакты лаунчера.
const darwinLabel = "app.typhon.launcher"

type darwinLaunchAgent struct{}

func (darwinLaunchAgent) Enable() error {
	exe, err := resolvedExecutablePath()
	if err != nil {
		return err
	}
	dir, err := launchAgentsDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("autostart: создание %s: %w", dir, err)
	}
	path := launchAgentPath(dir)
	if err := storage.WriteAtomic(path, []byte(launchAgentPlist(exe))); err != nil {
		return fmt.Errorf("autostart: запись plist автозапуска: %w", err)
	}
	// LaunchAgent-plist принято держать читаемым для всех — так лежат
	// соседние plist в ~/Library/LaunchAgents. storage.WriteAtomic ставит
	// 0600 для нового файла и сохраняет прежний режим для существующего.
	//nolint:gosec // G302: plist без секретов, 0644 — обычный режим LaunchAgent-файлов в ~/Library/LaunchAgents
	if err := os.Chmod(path, 0o644); err != nil {
		return fmt.Errorf("autostart: chmod plist автозапуска: %w", err)
	}
	// Активация в текущей сессии — только удобство: обязательна лишь запись
	// на диск, остальное подхватит следующий вход в систему. Поэтому неудача
	// здесь не отменяет уже состоявшуюся регистрацию — но и потеряться
	// молча не должна: иначе стабильно не срабатывающий bootstrap никто
	// никогда не заметит.
	if err := launchctlBootstrap(path); err != nil {
		slog.Warn("autostart: не удалось подключить агент к текущей сессии",
			"path", path, "error", err)
	}
	return nil
}

func (darwinLaunchAgent) Disable() error {
	dir, err := launchAgentsDir()
	if err != nil {
		return err
	}
	path := launchAgentPath(dir)
	// Отключение от сессии тоже best-effort: plist снимается следующей
	// строкой, и без него агент не поднимется при следующем входе в любом
	// случае.
	if err := launchctlBootout(path); err != nil {
		slog.Debug("autostart: не удалось отключить агент от текущей сессии",
			"path", path, "error", err)
	}
	if err := os.Remove(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("autostart: удаление plist автозапуска: %w", err)
	}
	return nil
}

func (darwinLaunchAgent) IsEnabled() (bool, error) {
	exe, err := resolvedExecutablePath()
	if err != nil {
		return false, err
	}
	dir, err := launchAgentsDir()
	if err != nil {
		return false, err
	}
	data, err := os.ReadFile(launchAgentPath(dir))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("autostart: чтение plist автозапуска: %w", err)
	}
	registered, err := launchAgentExecutable(data)
	if err != nil {
		// Файл лежит под нашим фиксированным именем — если его не удаётся
		// разобрать, это порча состояния, а не «автозапуск не настроен».
		// Молчаливое false здесь означало бы, что Apply никогда не заметит
		// и не исправит битый файл, если настройка уже стоит «включено».
		return false, fmt.Errorf("autostart: битый plist автозапуска: %w", err)
	}
	return registered == exe, nil
}

func launchAgentsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("autostart: домашний каталог: %w", err)
	}
	return filepath.Join(home, "Library", "LaunchAgents"), nil
}

func launchAgentPath(dir string) string {
	return filepath.Join(dir, darwinLabel+".plist")
}

// resolvedExecutablePath возвращает путь текущего исполняемого файла с
// разрешёнными симлинками. В отличие от похожей функции в Wails, ошибка
// EvalSymlinks здесь не проглатывается откатом на неразрешённый путь: раз
// процесс уже выполняется по пути из os.Executable, отказ разрешить
// симлинки на нём — не штатная ситуация, которую стоит скрывать.
func resolvedExecutablePath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("autostart: путь исполняемого файла: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return "", fmt.Errorf("autostart: разрешение симлинков %s: %w", exe, err)
	}
	return resolved, nil
}

const launchAgentTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<false/>
</dict>
</plist>
`

func launchAgentPlist(exe string) string {
	return fmt.Sprintf(launchAgentTemplate, xmlEscape(darwinLabel), xmlEscape(exe))
}

func xmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	return s
}

// launchAgentExecutable разбирает LaunchAgent plist и возвращает первую
// строку из массива ProgramArguments. Возвращает ошибку, если plist не
// разбирается вообще или в нём нет непустого ProgramArguments — по этому
// фиксированному пути лежит только наш файл, поэтому неожиданная форма
// значит порчу, а не чужой plist, который можно молча пропустить.
func launchAgentExecutable(data []byte) (string, error) {
	dec := xml.NewDecoder(strings.NewReader(string(data)))
	dec.Strict = false
	var lastKey string
	var inArray bool
	for {
		tok, err := dec.Token()
		if err != nil {
			return "", fmt.Errorf("разбор plist: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "key":
				var key string
				if err := dec.DecodeElement(&key, &t); err != nil {
					return "", fmt.Errorf("разбор ключа plist: %w", err)
				}
				lastKey = key
				continue
			case "array":
				if lastKey == "ProgramArguments" {
					inArray = true
				}
			case "string":
				if inArray {
					var s string
					if err := dec.DecodeElement(&s, &t); err != nil {
						return "", fmt.Errorf("разбор ProgramArguments: %w", err)
					}
					return s, nil
				}
			}
		case xml.EndElement:
			if t.Name.Local == "array" && inArray {
				return "", errors.New("ProgramArguments пуст")
			}
		}
	}
}

// launchctlBootstrap подключает plist к текущей GUI-сессии сразу, без
// перезахода. Подменяемая переменная — иначе тест с успешным bootstrap и
// RunAtLoad=true перезапустил бы сам тестовый бинарь через launchd.
var launchctlBootstrap = func(path string) error {
	//nolint:gosec // G204: путь строит сам пакет, не пользовательский ввод
	return exec.Command("launchctl", "bootstrap", fmt.Sprintf("gui/%d", os.Getuid()), path).Run()
}

var launchctlBootout = func(path string) error {
	//nolint:gosec // G204: путь строит сам пакет, не пользовательский ввод
	return exec.Command("launchctl", "bootout", fmt.Sprintf("gui/%d", os.Getuid()), path).Run()
}
