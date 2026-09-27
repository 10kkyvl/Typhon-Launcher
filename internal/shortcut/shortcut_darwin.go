//go:build darwin && !devmock

package shortcut

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"typhon/internal/storage"
)

func Supported() bool { return true }

func DesktopDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("shortcut: домашний каталог: %w", err)
	}
	dir := filepath.Join(home, "Desktop")
	info, err := os.Stat(dir)
	if err != nil {
		return "", fmt.Errorf("shortcut: каталог Desktop %s: %w", dir, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("shortcut: %s не является каталогом", dir)
	}
	return dir, nil
}

// Create собирает настоящий бандл .app по пути path: Contents/Info.plist и
// исполняемый shell-скрипт в Contents/MacOS, который запускает Link.Target.
// Оба файла пишутся детерминированно по одному и тому же path (имя
// исполняемого файла — базовое имя бандла), поэтому повторный вызов с тем же
// path просто перезаписывает оба файла и не оставляет мусора от старой
// версии — отдельного шага удаления перед записью не требуется.
func Create(path string, link Link) error {
	if path == "" {
		return errors.New("shortcut: путь ярлыка пуст")
	}
	if link.Target == "" {
		return errors.New("shortcut: цель ярлыка (Target) пуста")
	}
	dir := filepath.Dir(path)
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("shortcut: каталог назначения %s: %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("shortcut: %s не является каталогом", dir)
	}

	name := strings.TrimSuffix(filepath.Base(path), ".app")
	if name == "" {
		return fmt.Errorf("shortcut: %s не является именем .app", path)
	}

	macOSDir := filepath.Join(path, "Contents", "MacOS")
	if err := os.MkdirAll(macOSDir, 0o755); err != nil {
		return fmt.Errorf("shortcut: создание %s: %w", macOSDir, err)
	}

	// Icon игнорируется: это путь к исполняемому файлу игры (.exe) или к
	// произвольному файлу-источнику иконки, а бандлу macOS нужен .icns —
	// конвертации в проекте нет, а без неё CFBundleIconFile указывал бы на
	// несуществующий ресурс. Finder в этом случае просто покажет бандлу
	// стандартную иконку приложения.
	plistPath := filepath.Join(path, "Contents", "Info.plist")
	if err := storage.WriteAtomic(plistPath, []byte(bundleInfoPlist(name))); err != nil {
		return fmt.Errorf("shortcut: запись Info.plist: %w", err)
	}

	execPath := filepath.Join(macOSDir, name)
	if err := storage.WriteAtomic(execPath, []byte(bundleLauncherScript(link))); err != nil {
		return fmt.Errorf("shortcut: запись исполняемого файла бандла: %w", err)
	}
	// storage.WriteAtomic сохраняет режим существующего файла или ставит
	// 0600 для нового: ни то, ни другое не даёт бита запуска, который нужен
	// исполняемому файлу бандла.
	//nolint:gosec // G302: это исполняемый файл бандла .app, ему нужен бит запуска; 0600 (как хочет gosec) сделал бы бандл незапускаемым
	if err := os.Chmod(execPath, 0o755); err != nil {
		return fmt.Errorf("shortcut: chmod исполняемого файла бандла: %w", err)
	}
	return nil
}

const plistTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleName</key>
	<string>%s</string>
	<key>CFBundleExecutable</key>
	<string>%s</string>
	<key>CFBundleIdentifier</key>
	<string>%s</string>
	<key>CFBundlePackageType</key>
	<string>APPL</string>
	<key>CFBundleInfoDictionaryVersion</key>
	<string>6.0</string>
	<key>NSHighResolutionCapable</key>
	<true/>
</dict>
</plist>
`

func bundleInfoPlist(name string) string {
	return fmt.Sprintf(plistTemplate, xmlEscape(name), xmlEscape(name), xmlEscape(bundleIdentifier(name)))
}

// bundleIdentifier строит CFBundleIdentifier из имени бандла: только ASCII
// буквы/цифры, остальное — дефис. Название игры не обязано быть валидным
// bundle id (там может быть кириллица, эмодзи, амперсанд), а бандл всё равно
// нужно собрать. Совпадение идентификаторов у разных ярлыков не страшно: они
// не регистрируются в системе, а просто запускаются двойным щелчком.
func bundleIdentifier(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		slug = "game"
	}
	return "com.typhon.shortcut." + slug
}

func xmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	return s
}

// bundleLauncherScript формирует shell-скрипт, который cd'ается в WorkDir
// (если задан) и запускает Target с аргументами Args. Args — единая строка
// в стиле Windows (её же собирают вызывающие пакеты, ориентируясь на
// SetArguments из shortcut_windows.go), поэтому она разбивается на отдельные
// аргументы splitArgs так же, как Windows разобрал бы командную строку
// ярлыка при запуске, а не передаётся процессу одним слитным словом.
func bundleLauncherScript(link Link) string {
	var b strings.Builder
	b.WriteString("#!/bin/sh\n")
	if link.WorkDir != "" {
		fmt.Fprintf(&b, "cd %s || exit 1\n", shellQuote(link.WorkDir))
	}
	b.WriteString("exec")
	fmt.Fprintf(&b, " %s", shellQuote(link.Target))
	for _, arg := range splitArgs(link.Args) {
		fmt.Fprintf(&b, " %s", shellQuote(arg))
	}
	b.WriteString("\n")
	return b.String()
}

// shellQuote оборачивает s в одинарные кавычки для POSIX sh. Каждая
// одинарная кавычка внутри s заменяется на четыре символа: закрывающая
// кавычка, обратный слэш, экранированная кавычка, открывающая кавычка —
// классический приём для POSIX sh, где внутри одинарных кавычек нет вообще
// никаких escape-последовательностей. Без этого одинарная кавычка в пути к
// игре разорвала бы скрипт, либо — что хуже — позволила бы остатку строки
// выполниться как команда shell.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// splitArgs разбивает Args на отдельные аргументы так же, как Windows
// разбирает командную строку ярлыка при запуске: пробел — разделитель,
// двойные кавычки группируют кусок с пробелами в один аргумент. Кавычки в
// результат не попадают — их роль исчерпывается группировкой, как и в
// CommandLineToArgvW.
func splitArgs(args string) []string {
	var result []string
	var cur strings.Builder
	inQuotes := false
	hasCur := false
	for _, r := range args {
		switch {
		case r == '"':
			inQuotes = !inQuotes
			hasCur = true
		case unicode.IsSpace(r) && !inQuotes:
			if hasCur {
				result = append(result, cur.String())
				cur.Reset()
				hasCur = false
			}
		default:
			cur.WriteRune(r)
			hasCur = true
		}
	}
	if hasCur {
		result = append(result, cur.String())
	}
	return result
}
