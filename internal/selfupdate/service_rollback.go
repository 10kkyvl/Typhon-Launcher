package selfupdate

import (
	"github.com/wailsapp/wails/v3/pkg/application"
)

// GetRollback says whether the launcher can go back to the version it was
// updated from. It reads two small files and never fails: the reason an
// answer is "no" travels in the result.
func (s *Service) GetRollback() RollbackInfo {
	return rollbackAvailability(s.dir, s.currentVersion)
}

// ApplyRollback hands the launcher over to a worker that puts the previous
// version and its data back, then quits. Like ApplyUpdate it returns before
// anything has changed, and the service stays busy until the process is gone.
func (s *Service) ApplyRollback(language string) error {
	if err := s.acquire(); err != nil {
		return err
	}
	return s.startRollback(language)
}

// startRollback is ApplyRollback for a caller that already holds the service.
func (s *Service) startRollback(language string) error {
	if err := launchRollback(s.dir, s.portable, s.currentVersion, language); err != nil {
		s.release()
		return err
	}

	s.mu.Lock()
	s.rollbackPending = true
	s.mu.Unlock()
	s.watchRestart()
	if a := application.Get(); a != nil {
		a.Quit()
	}
	return nil
}
