package account

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"

	"typhon/internal/storage"
)

const profileVersion = 1

type cachedProfile struct {
	User      CurrentUser `json:"user"`
	Avatar    AvatarImage `json:"avatar"`
	AvatarURL string      `json:"avatarUrl"`
}

func profilePathFrom(statePath string) string {
	return filepath.Join(filepath.Dir(statePath), "profile.json")
}

func loadProfile(path string) (cachedProfile, error) {
	var loaded cachedProfile
	err := storage.Load(path, profileVersion, nil, &loaded)
	if errors.Is(err, fs.ErrNotExist) {
		return cachedProfile{}, nil
	}
	if err != nil {
		return cachedProfile{}, err
	}
	return loaded, nil
}

func removeProfileFile(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove %s: %w", filepath.Base(path), err)
	}
	return nil
}

func (s *Service) currentProfile() cachedProfile {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.profile
}

// setProfile and forgetProfile both take profileMu before touching profile.json, so an
// epoch captured before a slow rememberProfile network call can never win a race against
// a forgetProfile that ran while that call was in flight (invariant IV.17).
func (s *Service) setProfile(epoch uint64, next cachedProfile) error {
	s.profileMu.Lock()
	defer s.profileMu.Unlock()

	s.mu.Lock()
	if epoch != s.profileEpoch {
		s.mu.Unlock()
		return nil
	}
	s.profile = next
	path := s.profilePath
	s.mu.Unlock()

	if err := storage.Save(path, profileVersion, next); err != nil {
		slog.Error("save cached profile", "error", err)
		s.mu.Lock()
		if epoch == s.profileEpoch {
			s.profile = cachedProfile{}
		}
		s.mu.Unlock()
		// Cache is derived state, not the source of truth (credential store), so a failed write degrades instead of failing the caller.
		if err := removeProfileFile(path); err != nil {
			return fmt.Errorf("discard stale profile cache: %w", err)
		}
		return nil
	}
	return nil
}

func (s *Service) forgetProfile() error {
	s.profileMu.Lock()
	defer s.profileMu.Unlock()

	s.mu.Lock()
	s.profile = cachedProfile{}
	s.profileEpoch++
	path := s.profilePath
	s.mu.Unlock()

	return removeProfileFile(path)
}

func (s *Service) rememberProfile(ctx context.Context, user CurrentUser) error {
	s.mu.Lock()
	epoch := s.profileEpoch
	previous := s.profile
	s.mu.Unlock()

	next := cachedProfile{User: user}
	switch {
	case user.AvatarURL == "":
	case user.AvatarURL == previous.AvatarURL && previous.Avatar.Data != "":
		next.Avatar = previous.Avatar
		next.AvatarURL = previous.AvatarURL
	default:
		avatar, err := s.client.FetchAvatar(ctx, user.AvatarURL)
		// User is the primary record; a failed download must not keep showing a stale avatar under a new URL.
		if err != nil {
			slog.Warn("fetch avatar for offline cache", "error", err)
		} else {
			next.Avatar = avatar
			next.AvatarURL = user.AvatarURL
		}
	}

	return s.setProfile(epoch, next)
}
