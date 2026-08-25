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
