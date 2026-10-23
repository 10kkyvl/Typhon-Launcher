package jumplist

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"
)

func testConfig(t *testing.T) Config {
	t.Helper()
	return Config{
		AppID:    "Typhon.Launcher",
		Launcher: filepath.Join(t.TempDir(), "typhon.exe"),
		Labels:   Labels{Recent: "Recent games", Library: "Library", Downloads: "Downloads"},
	}
}

func TestPlayArgs(t *testing.T) {
	cases := []struct {
		name    string
		id      string
		want    string
		wantErr error
	}{
		{"plain id", "a1b2c3", "--play a1b2c3", nil},
		{"dashes and dots", "g-1.2_x", "--play g-1.2_x", nil},
		{"empty", "", "", ErrBadGameID},
		{"space", "a b", "", ErrBadGameID},
		{"tab", "a\tb", "", ErrBadGameID},
		{"quote", `a"b`, "", ErrBadGameID},
		{"newline", "a\nb", "", ErrBadGameID},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := PlayArgs(tc.id)
			if !errors.Is(err, tc.wantErr) || got != tc.want {
				t.Fatalf("PlayArgs(%q) = %q, %v; want %q, %v", tc.id, got, err, tc.want, tc.wantErr)
			}
		})
	}
}

func TestOpenArgs(t *testing.T) {
	if got := OpenArgs(RouteLibrary); got != "--open library" {
		t.Fatalf("OpenArgs(library) = %q", got)
	}
	if got := OpenArgs(RouteDownloads); got != "--open downloads" {
		t.Fatalf("OpenArgs(downloads) = %q", got)
	}
}

func TestRecentEntries(t *testing.T) {
	cfg := testConfig(t)
	gameExe := filepath.Join(t.TempDir(), "game.exe")
	cases := []struct {
		name    string
		games   []Game
		removed map[string]struct{}
		want    []Entry
		wantErr error
	}{
		{"no games", nil, nil, []Entry{}, nil},
		{
			"icon comes from the game executable",
			[]Game{{ID: "g1", Title: "Alpha", Executable: gameExe}},
			nil,
			[]Entry{{Title: "Alpha", Args: "--play g1", Icon: gameExe}},
			nil,
		},
		{
			"missing or relative executable falls back to the launcher icon",
			[]Game{{ID: "g1", Title: "Alpha"}, {ID: "g2", Title: "Beta", Executable: "game.exe"}},
			nil,
			[]Entry{
				{Title: "Alpha", Args: "--play g1", Icon: cfg.Launcher},
				{Title: "Beta", Args: "--play g2", Icon: cfg.Launcher},
			},
			nil,
		},
		{
			"removed games are not put back",
			[]Game{{ID: "g1", Title: "Alpha"}, {ID: "g2", Title: "Beta"}, {ID: "g3", Title: "Gamma"}},
			map[string]struct{}{"--play g2": {}},
			[]Entry{
				{Title: "Alpha", Args: "--play g1", Icon: cfg.Launcher},
				{Title: "Gamma", Args: "--play g3", Icon: cfg.Launcher},
			},
			nil,
		},
		{
			"everything removed leaves no entries",
			[]Game{{ID: "g1", Title: "Alpha"}},
			map[string]struct{}{"--play g1": {}},
			[]Entry{},
			nil,
		},
		{
			"title is flattened to one line",
			[]Game{{ID: "g1", Title: " Dark\tSouls \n III "}},
			nil,
			[]Entry{{Title: "Dark Souls III", Args: "--play g1", Icon: cfg.Launcher}},
			nil,
		},
		{
			"blank title shows the id",
			[]Game{{ID: "g1", Title: "  "}},
			nil,
			[]Entry{{Title: "g1", Args: "--play g1", Icon: cfg.Launcher}},
			nil,
		},
		{
			"an id that cannot be a command line argument is an error",
			[]Game{{ID: "g 1", Title: "Alpha"}},
			nil,
			nil,
			ErrBadGameID,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := recentEntries(cfg, tc.games, tc.removed)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("entries = %+v\nwant      %+v", got, tc.want)
			}
		})
	}
}

func TestTaskEntries(t *testing.T) {
	cfg := testConfig(t)
	want := []Entry{
		{Title: "Library", Args: "--open library", Icon: cfg.Launcher},
		{Title: "Downloads", Args: "--open downloads", Icon: cfg.Launcher},
	}
	if got := taskEntries(cfg); !reflect.DeepEqual(got, want) {
		t.Fatalf("taskEntries = %+v, want %+v", got, want)
	}
}

func TestConfigValidate(t *testing.T) {
	good := testConfig(t)
	cases := []struct {
		name   string
		mutate func(*Config)
		want   error
	}{
		{"valid", func(*Config) {}, nil},
		{"no app id", func(c *Config) { c.AppID = "" }, ErrNoAppID},
		{"app id with a space", func(c *Config) { c.AppID = "Typhon Launcher" }, ErrBadAppID},
		{"app id too long", func(c *Config) { c.AppID = string(make([]byte, 129)) }, ErrBadAppID},
		{"no launcher", func(c *Config) { c.Launcher = "" }, ErrNoLauncher},
		{"relative launcher", func(c *Config) { c.Launcher = "typhon.exe" }, ErrRelLauncher},
		{"no category", func(c *Config) { c.Labels.Recent = "" }, errNoCategory},
		{"no library task", func(c *Config) { c.Labels.Library = "" }, errNoTaskLabels},
		{"no downloads task", func(c *Config) { c.Labels.Downloads = "" }, errNoTaskLabels},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := good
			tc.mutate(&cfg)
			if err := cfg.validate(); !errors.Is(err, tc.want) {
				t.Fatalf("validate = %v, want %v", err, tc.want)
			}
		})
	}
}
