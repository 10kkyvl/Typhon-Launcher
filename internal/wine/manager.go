package wine

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// bottleTemplate — шаблон CrossOver, из которого заводятся все наши бутыли.
// Один на всех сознательно: игры различаются твиками поверх, а не базой, и
// общий шаблон делает поломку воспроизводимой.
const bottleTemplate = "win10_64"

var errNoGamesPath = errors.New("wine: путь папки игр не задан")

// Manager владеет бутылями Typhon внутри каталога бутылей CrossOver.
type Manager struct {
	rt         Runtime
	BottlesDir string

	// psOutput подменяет чтение таблицы процессов в тестах: настоящий ps на
	// машине сборки покажет что угодно, кроме нужного.
	psOutput func() (string, error)
}

func NewManager(rt Runtime) *Manager {
	return &Manager{rt: rt, BottlesDir: defaultBottlesDir()}
}

func defaultBottlesDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "Library", "Application Support", "CrossOver", "Bottles")
}

// Runtime отдаёт рантайм, с которым менеджер работает: версия нужна
// вызывающим для диагностики.
func (m *Manager) Runtime() Runtime { return m.rt }

// Ensure возвращает бутыль под этот каталог установки, заводя его при первом
// обращении. Буква диска возвращается на папку игр при каждом вызове.
func (m *Manager) Ensure(destDir, gamesPath string) (Bottle, error) {
	if strings.TrimSpace(gamesPath) == "" {
		return Bottle{}, errNoGamesPath
	}
	dest, err := filepath.Abs(destDir)
	if err != nil {
		return Bottle{}, fmt.Errorf("путь установки %s: %w", destDir, err)
	}
	games, err := filepath.Abs(gamesPath)
	if err != nil {
		return Bottle{}, fmt.Errorf("путь папки игр %s: %w", gamesPath, err)
	}

	if existing, ok := m.Lookup(dest); ok {
		if err := ensureDrive(existing.Path, existing.Drive, games); err != nil {
			return Bottle{}, err
		}
		return existing, nil
	}

	name := bottleName(dest)
	path := filepath.Join(m.BottlesDir, name)
	//nolint:gosec // G204: путь до cxbottle получен из Detect, имя бутыля построено bottleName
	cmd := exec.Command(m.rt.CxBottle, "--bottle", name, "--create",
		"--template", bottleTemplate, "--description", "Typhon: "+filepath.Base(dest))
	if out, err := cmd.CombinedOutput(); err != nil {
		return Bottle{}, fmt.Errorf("создание бутыля %s: %w: %s", name, err, strings.TrimSpace(string(out)))
	}

	letter, err := freeDrive(filepath.Join(path, "dosdevices"))
	if err != nil {
		return Bottle{}, err
	}
	if err := ensureDrive(path, letter, games); err != nil {
		return Bottle{}, err
	}
	b := Bottle{Key: dest, Name: name, Path: path, Drive: letter, Games: games}
	if err := writeMarker(path, b); err != nil {
		return Bottle{}, err
	}
	return b, nil
}

// List перечисляет только наши бутыли: чужие, заведённые пользователем в
// CrossOver руками, метки не имеют и нас не касаются.
func (m *Manager) List() ([]Bottle, error) {
	if m.BottlesDir == "" {
		return nil, errors.New("wine: каталог бутылей неизвестен")
	}
	entries, err := os.ReadDir(m.BottlesDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("чтение %s: %w", m.BottlesDir, err)
	}
	out := make([]Bottle, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if b, ok := readMarker(filepath.Join(m.BottlesDir, entry.Name())); ok {
			out = append(out, b)
		}
	}
	return out, nil
}

// Lookup находит бутыль по любому пути внутри каталога установки: библиотека
// знает только путь исполняемого файла, а ключ бутыля — каталог установки.
func (m *Manager) Lookup(anyPath string) (Bottle, bool) {
	path, err := filepath.Abs(anyPath)
	if err != nil {
		return Bottle{}, false
	}
	list, err := m.List()
	if err != nil {
		return Bottle{}, false
	}
	best := Bottle{}
	found := false
	for _, b := range list {
		if !underKey(path, b.Key) {
			continue
		}
		// Вложенные каталоги установки маловероятны, но если есть —
		// выигрывает самый конкретный.
		if !found || len(b.Key) > len(best.Key) {
			best, found = b, true
		}
	}
	return best, found
}

func underKey(path, key string) bool {
	return path == key || strings.HasPrefix(path, key+string(filepath.Separator))
}

// Remove сносит бутыль установки. Отсутствие бутыля не ошибка: удаление игры,
// поставленной до появления macOS-поддержки, обязано доходить до конца.
func (m *Manager) Remove(destDir string) error {
	b, ok := m.Lookup(destDir)
	if !ok {
		return nil
	}
	//nolint:gosec // G204: имя бутыля прочитано из нашей же метки
	cmd := exec.Command(m.rt.CxBottle, "--bottle", b.Name, "--delete", "--force")
	if out, err := cmd.CombinedOutput(); err != nil {
		// cxbottle мог не справиться, но каталог всё равно надо убрать:
		// осиротевший бутыль хуже, чем лишняя строка в логе.
		if rmErr := os.RemoveAll(b.Path); rmErr != nil {
			return fmt.Errorf("удаление бутыля %s: %w: %s", b.Name, err, strings.TrimSpace(string(out)))
		}
		return nil
	}
	if err := os.RemoveAll(b.Path); err != nil {
		return fmt.Errorf("удаление каталога бутыля %s: %w", b.Path, err)
	}
	return nil
}
