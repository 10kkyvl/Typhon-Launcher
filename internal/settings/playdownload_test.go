package settings

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestPlayDownloadDefaultsToOff(t *testing.T) {
	d := Defaults()
	if d.PlayDownloadMode != PlayDownloadOff {
		t.Fatalf("mode = %q, want off", d.PlayDownloadMode)
	}
	if _, err := sanitize(d); err != nil {
		t.Fatalf("defaults must pass validation: %v", err)
	}
}

func TestPlayDownloadValidation(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*Settings)
		want error
	}{
		{"off", func(s *Settings) { s.PlayDownloadMode = PlayDownloadOff }, nil},
		{"pause", func(s *Settings) { s.PlayDownloadMode = PlayDownloadPause }, nil},
		{"limit", func(s *Settings) {
			s.PlayDownloadMode = PlayDownloadLimit
			s.PlayDownloadRateLimit, s.PlayUploadRateLimit = 1<<20, 1<<19
		}, nil},
		{"unknown mode", func(s *Settings) { s.PlayDownloadMode = "stop" }, ErrPlayDownloadModeInvalid},
		{"empty mode", func(s *Settings) { s.PlayDownloadMode = "" }, ErrPlayDownloadModeInvalid},
		{"limit without limits", func(s *Settings) { s.PlayDownloadMode = PlayDownloadLimit }, ErrPlayRateLimitRequired},
		{"limit without upload", func(s *Settings) {
			s.PlayDownloadMode = PlayDownloadLimit
			s.PlayDownloadRateLimit = 1 << 20
		}, ErrPlayRateLimitRequired},
		{"limit with negative download", func(s *Settings) {
			s.PlayDownloadMode = PlayDownloadLimit
			s.PlayDownloadRateLimit, s.PlayUploadRateLimit = -1, 1<<19
		}, ErrPlayRateLimitRequired},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := Defaults()
			c.mut(&s)
			_, err := sanitize(s)
			if !errors.Is(err, c.want) {
				t.Fatalf("error = %v, want %v", err, c.want)
			}
		})
	}
}

func TestPlayDownloadPatchRefusesABadMode(t *testing.T) {
	svc := mustServiceAt(t, filepath.Join(t.TempDir(), "settings.json"))
	_, err := svc.SaveSettingsPatch(map[string]json.RawMessage{"playDownloadMode": json.RawMessage(`"stop"`)})
	if !errors.Is(err, ErrPlayDownloadModeInvalid) {
		t.Fatalf("error = %v", err)
	}
	if got := svc.GetSettings().PlayDownloadMode; got != PlayDownloadOff {
		t.Fatalf("stored mode = %q after a refused save", got)
	}
}

func TestOldConfigWithoutPlayFieldsLoadsAsOff(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"theme":"dark"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := mustServiceAt(t, path).GetSettings().PlayDownloadMode; got != PlayDownloadOff {
		t.Fatalf("mode = %q, want off", got)
	}
}

func TestPlayRates(t *testing.T) {
	cases := []struct {
		name     string
		mode     string
		running  bool
		userDown int64
		userUp   int64
		wantDown int64
		wantUp   int64
	}{
		{"no game", PlayDownloadLimit, false, 0, 0, 0, 0},
		{"pause mode leaves rates", PlayDownloadPause, true, 7, 8, 7, 8},
		{"user unlimited", PlayDownloadLimit, true, 0, 0, 100, 50},
		{"user lower", PlayDownloadLimit, true, 10, 5, 10, 5},
		{"user higher", PlayDownloadLimit, true, 1000, 1000, 100, 50},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := Defaults()
			s.PlayDownloadMode = c.mode
			s.PlayDownloadRateLimit, s.PlayUploadRateLimit = 100, 50
			s.DownloadRateLimit, s.UploadRateLimit = c.userDown, c.userUp
			down, up := s.PlayRates(c.running)
			if down != c.wantDown || up != c.wantUp {
				t.Fatalf("rates = %d/%d, want %d/%d", down, up, c.wantDown, c.wantUp)
			}
		})
	}
}

func TestNegativePlayLimitsAreClamped(t *testing.T) {
	s := Defaults()
	s.PlayDownloadRateLimit, s.PlayUploadRateLimit = -5, -7
	got, err := sanitize(s)
	if err != nil {
		t.Fatal(err)
	}
	if got.PlayDownloadRateLimit != 0 || got.PlayUploadRateLimit != 0 {
		t.Fatalf("limits = %d/%d, want 0/0", got.PlayDownloadRateLimit, got.PlayUploadRateLimit)
	}
}
