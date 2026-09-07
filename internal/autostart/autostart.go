package autostart

import (
	"errors"
	"fmt"
	"reflect"
	"sync"
)

var ErrNilManager = errors.New("autostart: nil manager")

type Manager interface {
	Enable() error
	Disable() error
	IsEnabled() (bool, error)
}

type Service struct {
	mu  sync.Mutex
	mgr Manager
}

func NewService(mgr Manager) (*Service, error) {
	if isNilManager(mgr) {
		return nil, ErrNilManager
	}
	return &Service{mgr: mgr}, nil
}

func isNilManager(mgr Manager) bool {
	if mgr == nil {
		return true
	}
	v := reflect.ValueOf(mgr)
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Slice, reflect.Map, reflect.Chan, reflect.Func:
		return v.IsNil()
	default:
		return false
	}
}

func (s *Service) Apply(enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	current, err := s.mgr.IsEnabled()
	if err != nil {
		return fmt.Errorf("autostart: check state: %w", err)
	}
	if current == enabled {
		return nil
	}
	if enabled {
		if err := s.mgr.Enable(); err != nil {
			return fmt.Errorf("autostart: enable: %w", err)
		}
		return nil
	}
	if err := s.mgr.Disable(); err != nil {
		return fmt.Errorf("autostart: disable: %w", err)
	}
	return nil
}

// platformManager позволяет ОС-специфичному файлу этого пакета подставить в
// ForPlatform собственную реализацию Manager вместо fallback. Нужно только
// на macOS: см. autostart_darwin.go. На остальных платформах остаётся
// тождественной функцией.
var platformManager = func(fallback Manager) Manager { return fallback }

// ForPlatform возвращает Manager, который должен получить NewService на
// текущей платформе. Обычно это fallback без изменений (например,
// wails.Autostart) — кроме macOS, где встроенный в Wails механизм
// (SMAppService) требует стабильной подписи приложения. У Typhon подписи
// нет и не будет (ad-hoc codesign в build/darwin/Taskfile.yml), и без
// стабильного Team ID SMAppService теряет идентичность приложения между
// пересборками — включая каждое самообновление. Поэтому на macOS
// используется LaunchAgent-реализация из autostart_darwin.go, а fallback
// игнорируется.
func ForPlatform(fallback Manager) Manager {
	return platformManager(fallback)
}
