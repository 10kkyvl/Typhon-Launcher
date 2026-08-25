package account

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"

	"typhon/internal/settings"
	"typhon/internal/storage"
)

const stateVersion = 1

type state struct {
	Guest bool `json:"guest"`
}

func statePath() (string, error) {
	dir, err := settings.ConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve config dir: %w", err)
	}
	return filepath.Join(dir, "account.json"), nil
}

func loadState(path string) (state, error) {
	var loaded state
	err := storage.Load(path, stateVersion, nil, &loaded)
	if errors.Is(err, fs.ErrNotExist) {
		return state{}, nil
	}
	if err != nil {
		return state{}, err
	}
	return loaded, nil
}

func (s *Service) setGuest(guest bool) error {
	s.mu.Lock()
	previous := s.guest
	s.guest = guest
	path := s.statePath
	s.mu.Unlock()

	if previous == guest {
		return nil
	}

	if err := storage.Save(path, stateVersion, state{Guest: guest}); err != nil {
		s.mu.Lock()
		s.guest = previous
		s.mu.Unlock()
		return fmt.Errorf("save account state: %w", err)
	}
	return nil
}

func (s *Service) isGuest() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.guest
}

func (s *Service) ContinueAsGuest() (State, error) {
	if err := s.setGuest(true); err != nil {
		return State{}, err
	}
	return State{Status: StatusGuest}, nil
}
