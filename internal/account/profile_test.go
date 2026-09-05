package account

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"typhon/internal/storage"
)

func TestLoadProfile(t *testing.T) {
	tests := []struct {
		name    string
		write   func(t *testing.T, path string)
		wantErr bool
		want    cachedProfile
	}{
		{
			name:  "missing file returns empty state",
			write: func(t *testing.T, path string) {},
		},
		{
			name: "corrupt json is an error",
			write: func(t *testing.T, path string) {
				if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
					t.Fatalf("write corrupt profile: %v", err)
				}
			},
			wantErr: true,
		},
		{
			name: "valid cache round-trips",
			write: func(t *testing.T, path string) {
				profile := cachedProfile{
					User:      sampleUser(),
					Avatar:    AvatarImage{Data: "YWJj", MIME: "image/png"},
					AvatarURL: "https://cdn.example/a.png",
				}
				if err := storage.Save(path, profileVersion, profile); err != nil {
					t.Fatalf("seed profile cache: %v", err)
				}
			},
			want: cachedProfile{
				User:      withProfileDefaults(sampleUser()),
				Avatar:    AvatarImage{Data: "YWJj", MIME: "image/png"},
				AvatarURL: "https://cdn.example/a.png",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "profile.json")
			tt.write(t, path)

			got, err := loadProfile(path)
			if tt.wantErr {
				if err == nil {
					t.Fatal("loadProfile() error = nil, want an error for the corrupt file")
				}
				return
			}
			if err != nil {
				t.Fatalf("loadProfile() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("loadProfile() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestLoadProfileAppliesDefaultsToLegacyCache(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profile.json")
	legacy := map[string]any{
		"user":      map[string]any{"id": "u1", "username": "old", "displayName": "Old", "email": "o@example.com", "createdAt": "2024-01-02T03:04:05Z"},
		"avatar":    map[string]any{"data": "", "mime": ""},
		"avatarUrl": "",
	}
	if err := storage.Save(path, profileVersion, legacy); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadProfile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.User.Profile.ShowStats || len(loaded.User.Profile.Showcase) != 1 {
		t.Fatalf("profile = %+v, want defaults", loaded.User.Profile)
	}
}

func TestLoadProfileAppliesDefaultsToOldStyleProfileCache(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profile.json")
	legacy := map[string]any{
		"user": map[string]any{
			"id": "u1", "username": "old", "displayName": "Old", "email": "o@example.com",
			"createdAt": "2024-01-02T03:04:05Z",
			"profile": map[string]any{
				"showStats":    false,
				"showPlaying":  true,
				"showActivity": true,
				"showOnline":   true,
				"showcase":     []string{"favorites"},
			},
		},
		"avatar":    map[string]any{"data": "", "mime": ""},
		"avatarUrl": "",
	}
	if err := storage.Save(path, profileVersion, legacy); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadProfile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := loaded.User.Profile
	if got.Visibility != VisibilityFriends {
		t.Errorf("visibility = %q, want %q", got.Visibility, VisibilityFriends)
	}
	if !got.ShowPlaytime || !got.ShowLibrary {
		t.Errorf("profile = %+v, want the two new toggles defaulted to true", got)
	}
	if got.ShowStats {
		t.Errorf("profile = %+v, want showStats to keep its cached value (false)", got)
	}
}

func TestNewServiceFailsOnCorruptProfileCache(t *testing.T) {
	path := statePathFor(t)
	profPath := profilePathFrom(path)
	if err := os.WriteFile(profPath, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("write corrupt profile: %v", err)
	}

	if _, err := newService(&fakeStore{}, "https://account-api.invalid", path); err == nil {
		t.Fatal("newService() error = nil, want a failure for the corrupt profile cache")
	}
}

func TestForgetProfileNotExistIsNotError(t *testing.T) {
	s := &Service{profilePath: filepath.Join(t.TempDir(), "profile.json"), profile: cachedProfile{User: sampleUser()}}
	if err := s.forgetProfile(); err != nil {
		t.Fatalf("forgetProfile() error = %v, want nil for a missing file", err)
	}
	if got := s.currentProfile(); got.User.ID != "" {
		t.Errorf("profile in memory = %+v, want cleared", got)
	}
}

func TestSetProfileWriteFailureDropsCacheAndFile(t *testing.T) {
	blocker := unwritableStateDir(t)
	profPath := filepath.Join(blocker, "profile.json")

	s := &Service{profilePath: profPath, profile: cachedProfile{User: CurrentUser{ID: "stale"}}}
	if err := s.setProfile(s.profileEpoch, cachedProfile{User: sampleUser()}); err != nil {
		t.Fatalf("setProfile() error = %v, want nil (a failed write degrades instead of failing the caller)", err)
	}
	if got := s.currentProfile(); got.User.ID != "" {
		t.Errorf("profile in memory = %+v, want cleared after the failed write", got)
	}
	if _, err := os.Stat(profPath); err == nil || !os.IsNotExist(err) {
		t.Errorf("profile file exists at %s, want absent", profPath)
	}
}

func TestRememberProfileSkipsRefetchWhenAvatarUnchanged(t *testing.T) {
	requests := 0
	avatarSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		if _, err := w.Write([]byte("\x89PNG\r\n\x1a\n")); err != nil {
			t.Error(err)
		}
	}))
	defer avatarSrv.Close()

	s, err := newService(&fakeStore{}, "https://account-api.invalid", statePathFor(t))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	user := sampleUser()
	user.AvatarURL = avatarSrv.URL + "/a.png"

	if err := s.rememberProfile(context.Background(), user); err != nil {
		t.Fatalf("rememberProfile() #1 error = %v", err)
	}
	if err := s.rememberProfile(context.Background(), user); err != nil {
		t.Fatalf("rememberProfile() #2 error = %v", err)
	}
	if requests != 1 {
		t.Fatalf("avatar requests = %d, want 1 (the second call must reuse the cache)", requests)
	}
	if got := s.currentProfile(); got.Avatar.Data == "" {
		t.Fatal("cached avatar is empty after a successful fetch")
	}
}

func TestRememberProfileClearsAvatarOnFetchFailure(t *testing.T) {
	avatarSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer avatarSrv.Close()

	s, err := newService(&fakeStore{}, "https://account-api.invalid", statePathFor(t))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	user := sampleUser()
	user.AvatarURL = avatarSrv.URL + "/missing.png"

	if err := s.rememberProfile(context.Background(), user); err != nil {
		t.Fatalf("rememberProfile() error = %v", err)
	}
	got := s.currentProfile()
	if got.User.ID != user.ID {
		t.Fatalf("cached user = %+v, want %+v", got.User, user)
	}
	if got.Avatar.Data != "" || got.AvatarURL != "" {
		t.Fatalf("cached avatar = %+v url=%q, want cleared after a failed download", got.Avatar, got.AvatarURL)
	}
}

func TestRememberProfileClearsCacheWhenAvatarURLEmpty(t *testing.T) {
	s, err := newService(&fakeStore{}, "https://account-api.invalid", statePathFor(t))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	withAvatar := sampleUser()
	withAvatar.AvatarURL = "https://cdn.example/a.png"
	s.mu.Lock()
	s.profile = cachedProfile{
		User:      withAvatar,
		Avatar:    AvatarImage{Data: "YWJj", MIME: "image/png"},
		AvatarURL: withAvatar.AvatarURL,
	}
	s.mu.Unlock()

	withoutAvatar := sampleUser()
	withoutAvatar.AvatarURL = ""
	if err := s.rememberProfile(context.Background(), withoutAvatar); err != nil {
		t.Fatalf("rememberProfile() error = %v", err)
	}
	got := s.currentProfile()
	if got.Avatar.Data != "" || got.AvatarURL != "" {
		t.Fatalf("cached avatar = %+v url=%q, want cleared when AvatarURL is empty", got.Avatar, got.AvatarURL)
	}
}

func TestSetProfileWriteFailureAndCleanupBothFail(t *testing.T) {
	profPath := filepath.Join(t.TempDir(), "profile.json")
	if err := os.Mkdir(profPath, 0o755); err != nil {
		t.Fatalf("mkdir profile path: %v", err)
	}
	if err := os.WriteFile(filepath.Join(profPath, "keep.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("seed non-empty dir: %v", err)
	}

	s := &Service{profilePath: profPath, profile: cachedProfile{User: CurrentUser{ID: "stale"}}}
	err := s.setProfile(s.profileEpoch, cachedProfile{User: sampleUser()})
	if err == nil {
		t.Fatal("setProfile() error = nil, want the wrapped cleanup failure")
	}
	if !strings.Contains(err.Error(), "discard stale profile cache") {
		t.Fatalf("error = %v, want it to mention the cleanup failure", err)
	}
}

func TestForgetProfileReturnsErrorWhenRemovalFails(t *testing.T) {
	profPath := filepath.Join(t.TempDir(), "profile.json")
	if err := os.Mkdir(profPath, 0o755); err != nil {
		t.Fatalf("mkdir profile path: %v", err)
	}
	if err := os.WriteFile(filepath.Join(profPath, "keep.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("seed non-empty dir: %v", err)
	}

	s := &Service{profilePath: profPath, profile: cachedProfile{User: sampleUser()}}
	if err := s.forgetProfile(); err == nil {
		t.Fatal("forgetProfile() error = nil, want the removal failure")
	}
}

func TestRememberProfileDoesNotResurrectAfterConcurrentLogout(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	avatarSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
		if _, err := w.Write([]byte("\x89PNG\r\n\x1a\n")); err != nil {
			t.Error(err)
		}
	}))
	defer avatarSrv.Close()

	s, err := newService(&fakeStore{}, "https://account-api.invalid", statePathFor(t))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	user := sampleUser()
	user.AvatarURL = avatarSrv.URL + "/a.png"

	rememberDone := make(chan error, 1)
	go func() {
		rememberDone <- s.rememberProfile(context.Background(), user)
	}()

	<-started // rememberProfile has captured its epoch and is now blocked inside FetchAvatar

	if err := s.forgetProfile(); err != nil {
		t.Fatalf("forgetProfile() error = %v", err)
	}

	close(release)

	if err := <-rememberDone; err != nil {
		t.Fatalf("rememberProfile() error = %v", err)
	}

	if got := s.currentProfile(); got.User.ID != "" {
		t.Fatalf("profile in memory = %+v, want cleared: a slow rememberProfile resurrected a logged-out cache", got)
	}
	if _, err := os.Stat(s.profilePath); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("profile cache file exists after a concurrent logout, want it absent")
	}
}
