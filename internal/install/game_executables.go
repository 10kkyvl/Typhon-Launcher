package install

import (
	"fmt"
	"strings"
)

func (s *Service) ExecutableCandidates(gameID string) ([]Candidate, error) {
	if s.library == nil {
		return nil, errNoLibrary
	}
	game, err := s.library.Find(gameID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(game.InstallDir) == "" {
		return nil, errEmptyDestination
	}
	ctx, err := s.baseContext()
	if err != nil {
		return nil, err
	}
	candidates, err := FindExecutables(ctx, game.InstallDir, game.Title)
	if err != nil {
		return nil, fmt.Errorf("поиск исполняемых файлов: %w", err)
	}
	return candidates, nil
}
