package install

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type Engine string

const (
	EngineUnknown       Engine = ""
	EngineInno          Engine = "inno"
	EngineNsis          Engine = "nsis"
	EngineInstallShield Engine = "installshield"
	EngineMsi           Engine = "msi"
)

const (
	engineScanMax   = 16 * 1024 * 1024
	engineScanChunk = 1 * 1024 * 1024
)

var (
	errEmptyInstallerPath  = errors.New("путь до установщика не задан")
	errNoSilent            = errors.New("установщик не поддерживает тихую установку")
	errRelativeDestination = errors.New("путь установки должен быть абсолютным")
	errInstallerCancelled  = errors.New("установка была отменена")
	errInstallerBusy       = errors.New("в системе уже выполняется другая установка")
	errEmptyInfPath        = errors.New("путь для inf-файла разведки не задан")
)

var innoMarkers = []string{"Inno Setup Setup Data", "Inno Setup Messages", "JR.Inno.Setup"}
var nsisMarkers = []string{"NullsoftInst", "Nullsoft Install System"}
var installShieldMarkers = []string{"InstallShield"}

var engineMarkerOverlap = longestMarkerLen() - 1

func longestMarkerLen() int {
	max := 0
	for _, group := range [][]string{innoMarkers, nsisMarkers, installShieldMarkers} {
		for _, m := range group {
			if len(m) > max {
				max = len(m)
			}
		}
	}
	return max
}

func DetectEngine(path string) (Engine, error) {
	if path == "" {
		return EngineUnknown, errEmptyInstallerPath
	}
	if _, err := os.Stat(path); err != nil {
		return EngineUnknown, fmt.Errorf("определение типа установщика %s: %w", path, err)
	}
	if strings.EqualFold(filepath.Ext(path), ".msi") {
		return EngineMsi, nil
	}
	return scanInstallerMarkers(path)
}

func scanInstallerMarkers(path string) (engine Engine, err error) {
	f, openErr := os.Open(path)
	if openErr != nil {
		return EngineUnknown, fmt.Errorf("открытие установщика %s: %w", path, openErr)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("закрытие установщика %s: %w", path, cerr)
			engine = EngineUnknown
		}
	}()

	var carry []byte
	var read int64
	var foundInno, foundNsis, foundInstallShield bool

	buf := make([]byte, engineScanChunk)
	for read < engineScanMax {
		toRead := buf
		if remaining := engineScanMax - read; remaining < int64(len(toRead)) {
			toRead = buf[:remaining]
		}
		n, readErr := f.Read(toRead)
		if n > 0 {
			window := append(append([]byte(nil), carry...), toRead[:n]...)
			if containsAny(window, innoMarkers) {
				foundInno = true
			}
			if containsAny(window, nsisMarkers) {
				foundNsis = true
			}
			if containsAny(window, installShieldMarkers) {
				foundInstallShield = true
			}
			if len(window) > engineMarkerOverlap {
				carry = append([]byte(nil), window[len(window)-engineMarkerOverlap:]...)
			} else {
				carry = append([]byte(nil), window...)
			}
			read += int64(n)
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return EngineUnknown, fmt.Errorf("чтение установщика %s: %w", path, readErr)
		}
		if n == 0 {
			break
		}
	}

	switch {
	case foundInno:
		return EngineInno, nil
	case foundNsis:
		return EngineNsis, nil
	case foundInstallShield:
		return EngineInstallShield, nil
	default:
		return EngineUnknown, nil
	}
}

func containsAny(data []byte, markers []string) bool {
	for _, m := range markers {
		if bytes.Contains(data, []byte(m)) {
			return true
		}
	}
	return false
}

func supportsSilent(engine Engine) bool {
	switch engine {
	case EngineInno, EngineNsis, EngineMsi:
		return true
	default:
		return false
	}
}

type silentPlan struct {
	Args    []string
	CmdLine string
	Tail    string
}

type installOptions struct {
	SkipShortcuts bool
	SkipExtras    bool
}

// Имена задач Inno задаёт автор установщика, но набор устоявшийся: репаки берут
// их из шаблонов. Неизвестное имя в /MERGETASKS установщик игнорирует, поэтому
// список безопасно держать с запасом. Компоненты (/COMPONENTS) сюда не входят:
// в них лежат и файлы самой игры, снять их — сломать установку.
var shortcutTasks = []string{
	"desktopicon", "desktopshortcut", "desktop", "commondesktopicon",
	"quicklaunchicon", "quicklaunch",
	"startmenu", "startmenuicon", "startmenushortcut", "createicons", "programsicon",
}

var extraTasks = []string{
	"directx", "dx", "dxsetup", "dxredist",
	"vcredist", "vcredist2010", "vcredist2013", "vcredist2015", "vcredist2019", "vcrun",
	"netfx", "dotnet", "dotnetfx", "framework",
	"physx", "openal", "xna",
	"associate", "fileassoc", "fileassociation", "assoc",
	"toolbar", "browser", "yandex", "chrome", "opera", "amigo", "webinstall", "offers",
}

func declinedTasks(opts installOptions) string {
	names := make([]string, 0, len(shortcutTasks)+len(extraTasks))
	if opts.SkipShortcuts {
		names = append(names, shortcutTasks...)
	}
	if opts.SkipExtras {
		names = append(names, extraTasks...)
	}
	if len(names) == 0 {
		return ""
	}
	declined := make([]string, 0, len(names))
	for _, name := range names {
		declined = append(declined, "!"+name)
	}
	return strings.Join(declined, ",")
}

func silentArgs(engine Engine, installerPath, dest, logPath string, opts installOptions) (silentPlan, error) {
	if installerPath == "" {
		return silentPlan{}, errNoExecutable
	}
	if dest == "" {
		return silentPlan{}, errEmptyDestination
	}
	if !filepath.IsAbs(dest) {
		return silentPlan{}, errRelativeDestination
	}
	dest = filepath.Clean(dest)

	switch engine {
	case EngineInno:
		args := []string{"/SP-", "/VERYSILENT", "/SUPPRESSMSGBOXES", "/NORESTART", "/DIR=" + dest}
		if opts.SkipShortcuts {
			args = append(args, "/NOICONS")
		}
		if tasks := declinedTasks(opts); tasks != "" {
			args = append(args, "/MERGETASKS="+tasks)
		}
		if logPath != "" {
			args = append(args, "/LOG="+logPath)
		}
		return silentPlan{Args: args}, nil
	case EngineMsi:
		args := []string{"/i", installerPath, "/qn", "/norestart", "TARGETDIR=" + dest, "INSTALLDIR=" + dest, "APPDIR=" + dest}
		if logPath != "" {
			args = append(args, "/L*v", logPath)
		}
		return silentPlan{Args: args}, nil
	case EngineNsis:
		tail := "/S /D=" + dest
		return silentPlan{CmdLine: quoteArg(installerPath) + " " + tail, Tail: tail}, nil
	default:
		return silentPlan{}, errNoSilent
	}
}

// discoverPlan строит план разведочного запуска Inno с /SAVEINF: установщик
// всё равно распакует файлы (Inno не умеет писать инф без реального прогона),
// но цель вызова — получить список Components из inf-файла, а не установку.
func discoverPlan(engine Engine, installerPath, dest, infPath string) (silentPlan, bool, error) {
	if installerPath == "" {
		return silentPlan{}, false, errEmptyInstallerPath
	}
	if dest == "" {
		return silentPlan{}, false, errEmptyDestination
	}
	if !filepath.IsAbs(dest) {
		return silentPlan{}, false, errRelativeDestination
	}
	if infPath == "" {
		return silentPlan{}, false, errEmptyInfPath
	}
	dest = filepath.Clean(dest)

	switch engine {
	case EngineInno:
		args := []string{"/SP-", "/VERYSILENT", "/SUPPRESSMSGBOXES", "/NORESTART", "/DIR=" + dest, "/SAVEINF=" + infPath}
		return silentPlan{Args: args}, true, nil
	default:
		return silentPlan{}, false, nil
	}
}

func planWithComponents(plan silentPlan, components []string) silentPlan {
	if len(components) == 0 {
		return plan
	}
	plan.Args = append(plan.Args, "/COMPONENTS="+strings.Join(components, ","))
	return plan
}

func quoteArg(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	backslashes := 0
	for _, r := range s {
		switch r {
		case '\\':
			backslashes++
		case '"':
			b.WriteString(strings.Repeat(`\`, backslashes*2+1))
			b.WriteByte('"')
			backslashes = 0
		default:
			if backslashes > 0 {
				b.WriteString(strings.Repeat(`\`, backslashes))
				backslashes = 0
			}
			b.WriteRune(r)
		}
	}
	if backslashes > 0 {
		b.WriteString(strings.Repeat(`\`, backslashes*2))
	}
	b.WriteByte('"')
	return b.String()
}

var innoExitMessages = map[int]string{
	1: "не удалось запустить установщик",
	3: "внутренняя ошибка подготовки",
	4: "ошибка инициализации",
	6: "прервано отладчиком",
	7: "прервано скриптом установки",
	8: "прервано скриптом установки",
}

var msiExitMessages = map[int]string{
	1603: "фатальная ошибка установки",
	1619: "пакет не открывается",
	1620: "повреждённый пакет",
	1625: "установка запрещена политикой",
	1638: "продукт уже установлен",
}

func exitError(engine Engine, code int) error {
	if code == 0 || code == rebootExitCode {
		return nil
	}
	if engine == EngineMsi && code == 1641 {
		return nil
	}
	if (engine == EngineInno && (code == 2 || code == 5)) || (engine == EngineMsi && code == 1602) {
		return exitCodeError(errInstallerCancelled, engine, code)
	}
	if engine == EngineMsi && code == 1618 {
		return exitCodeError(errInstallerBusy, engine, code)
	}
	return exitCodeError(errInstallerFail, engine, code)
}

func exitCodeError(base error, engine Engine, code int) error {
	var describe string
	switch engine {
	case EngineInno:
		describe = innoExitMessages[code]
	case EngineMsi:
		describe = msiExitMessages[code]
	}
	if describe == "" {
		return fmt.Errorf("%w: код %d", base, code)
	}
	return fmt.Errorf("%w: код %d (%s)", base, code, describe)
}
