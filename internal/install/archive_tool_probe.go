package install

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"time"
)

// Переключатель -ol- появился в UnRAR 7.00 (WhatsNew.txt, «Version 7.00»,
// пункт 14).
const unrarLinkSwitchMajor = 7

// Версия 7-Zip, с которой подтверждено то, на что рассчитаны аргументы: в
// History.txt 25.00 закрыта CVE-2025-53816 (обработчик RAR), а 25.01 изменил
// код обработки ссылок ради безопасности; ключ -snl- проверен на 25.01.
var sevenZipFloor = toolVersion{major: 25, minor: 1}

var (
	errToolVersion        = errors.New("версия не определена или ниже безопасной (WinRAR 7.13+, 7-Zip 25.01+)")
	errBannerUnrecognised = errors.New("справка распаковщика не содержит версии")
	unrarBannerExp        = regexp.MustCompile(`(?im)^\s*unrar\s+(\d+)\.(\d+)`)
	sevenZipBannerExp     = regexp.MustCompile(`(?im)^\s*7-zip\s+(?:\([a-z]\)\s+)?(?:\[\d+\]\s+)?(\d+)\.(\d+)`)
)

var (
	toolBanner        = readToolBanner
	toolBannerTimeout = 5 * time.Second
)

type toolVersion struct{ major, minor int }

func (v toolVersion) atLeast(floor toolVersion) bool {
	return v.major > floor.major || v.major == floor.major && v.minor >= floor.minor
}

func (v toolVersion) String() string { return fmt.Sprintf("%d.%d", v.major, v.minor) }

// toolSet — распаковщики, которым можно доверить запись, и причины, по которым
// найденные отклонены.
type toolSet struct {
	tools   []archiveTool
	refused []error
}

func (s *toolSet) add(tool archiveTool, err error) {
	if err != nil {
		slog.Warn("archive tool refused", "error", err)
		s.refused = append(s.refused, err)
		return
	}
	s.tools = append(s.tools, tool)
}

func refusal(name, path string, reason error) error {
	return fmt.Errorf("%s (%s): %w: %w", name, filepath.Base(path), errToolVersion, reason)
}

func probeVersion(ctx context.Context, path string, exp *regexp.Regexp) (toolVersion, error) {
	banner, err := toolBanner(ctx, path)
	if err != nil {
		return toolVersion{}, err
	}
	match := exp.FindStringSubmatch(banner)
	if match == nil {
		return toolVersion{}, errBannerUnrecognised
	}
	major, majorErr := strconv.Atoi(match[1])
	minor, minorErr := strconv.Atoi(match[2])
	if err := errors.Join(majorErr, minorErr); err != nil {
		return toolVersion{}, fmt.Errorf("%w: %w", errBannerUnrecognised, err)
	}
	return toolVersion{major: major, minor: minor}, nil
}

// Там, где у UnRAR есть известная уязвимость пути (unrarFloor задан), версию,
// которую не удалось прочитать, нельзя отличить от уязвимой, и распаковщик
// отклоняется. Где её нет, версия нужна только для -ol-, и неизвестная
// считается старой.
func newUnrar(ctx context.Context, path string) (archiveTool, error) {
	version, err := probeVersion(ctx, path, unrarBannerExp)
	switch {
	case err != nil && unrarFloor != (toolVersion{}):
		return archiveTool{}, refusal("UnRAR", path, err)
	case err != nil:
		slog.Warn("unrar version is unknown, running it without -ol-", "path", path, "error", err)
		return unrarTool(path, false), nil
	case !version.atLeast(unrarFloor):
		return archiveTool{}, refusal("UnRAR", path, fmt.Errorf("версия %s ниже %s", version, unrarFloor))
	}
	return unrarTool(path, version.major >= unrarLinkSwitchMajor), nil
}

func newSevenZip(ctx context.Context, path string) (archiveTool, error) {
	version, err := probeVersion(ctx, path, sevenZipBannerExp)
	switch {
	case err != nil:
		return archiveTool{}, refusal("7-Zip", path, err)
	case !version.atLeast(sevenZipFloor):
		return archiveTool{}, refusal("7-Zip", path, fmt.Errorf("версия %s ниже %s", version, sevenZipFloor))
	}
	return sevenZipTool(path), nil
}

func readToolBanner(ctx context.Context, path string) (string, error) {
	return runBanner(ctx, path)
}

func runBanner(ctx context.Context, path string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, toolBannerTimeout)
	defer cancel()
	//nolint:gosec // G204: path is an absolute path to an existing file found by findArchiveTools (invariant 33); the arguments are fixed by the caller
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.SysProcAttr = toolProcAttr()
	cmd.WaitDelay = toolWaitDelay
	out, err := cmd.CombinedOutput()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return "", fmt.Errorf("справка %s: %w", filepath.Base(path), ctxErr)
	}
	var exitErr *exec.ExitError
	if err != nil && !errors.As(err, &exitErr) {
		return "", fmt.Errorf("запуск %s: %w", filepath.Base(path), err)
	}
	return string(out), nil
}
