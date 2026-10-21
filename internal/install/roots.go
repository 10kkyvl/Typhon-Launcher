package install

import (
	"log/slog"

	"typhon/internal/platform"
	"typhon/internal/settings"
)

// chooseRoot picks the games folder a new installation is proposed in: the one
// used last while its volume is there, otherwise the one with the most free
// room, otherwise the main one. A volume whose free space cannot be read is
// not a candidate here; the space check on the proposed destination reports
// that failure to the user.
func (s *Service) chooseRoot(cfg settings.Settings) string {
	roots := cfg.GamesRoots()
	switch len(roots) {
	case 0:
		return ""
	case 1:
		return roots[0]
	}
	free := make(map[string]uint64, len(roots))
	for _, root := range roots {
		info, err := s.freeSpace(root)
		if err != nil {
			continue
		}
		free[root] = info.FreeBytes
	}
	if cfg.LastInstallRoot != "" {
		for _, root := range roots {
			if _, ok := free[root]; ok && platform.SamePath(root, cfg.LastInstallRoot) {
				return root
			}
		}
	}
	best := ""
	var bestFree uint64
	for _, root := range roots {
		avail, ok := free[root]
		if ok && (best == "" || avail > bestFree) {
			best, bestFree = root, avail
		}
	}
	if best == "" {
		return cfg.GamesPath
	}
	return best
}

func (s *Service) proposeDestinationFor(cfg settings.Settings, name string) string {
	return s.proposeDestination(s.chooseRoot(cfg), name)
}

// rememberInstallRoot records which games folder the user installed into, so
// the next proposal starts there. A failure to write it is only logged: the
// installation is already under way and the choice is a convenience.
func (s *Service) rememberInstallRoot(destination string) {
	if s.settings == nil || destination == "" {
		return
	}
	cfg := s.settings.GetSettings()
	for _, root := range cfg.GamesRoots() {
		if !platform.Inside(root, destination) || platform.SamePath(root, destination) {
			continue
		}
		if platform.SamePath(root, cfg.LastInstallRoot) {
			return
		}
		if _, err := s.settings.Update(func(next *settings.Settings) error {
			next.LastInstallRoot = root
			return nil
		}); err != nil {
			slog.Warn("remember install folder", "root", root, "error", err)
		}
		return
	}
}
