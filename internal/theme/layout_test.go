package theme

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestLayoutSpecParses(t *testing.T) {
	if layoutSpecErr != nil {
		t.Fatalf("layout.json: %v", layoutSpecErr)
	}
	for _, part := range layoutIDs.PartsWithID {
		if !slices.Contains(layoutIDs.Parts, part) {
			t.Errorf("partsWithId has %q, which is not in parts", part)
		}
	}
}

func TestParseLayoutSpecRejectsBrokenFile(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"garbage", `not json`},
		{"empty object", `{}`},
		{"unknown field", strings.Replace(string(layoutJSON), `"separator"`, `"extra": 1, "separator"`, 1)},
		{"repeated item", strings.Replace(string(layoutJSON), `"catalog", "installed"`, `"catalog", "catalog"`, 1)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseLayoutSpec([]byte(tc.raw)); err == nil {
				t.Fatal("broken layout.json parsed")
			}
		})
	}
}

func repeat(id string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = id
	}
	return out
}

func TestValidateLayout(t *testing.T) {
	long := strings.Repeat("a", maxLayoutIDBytes+1)
	cases := []struct {
		name    string
		layout  *Layout
		wantErr bool
		want    *Layout
	}{
		{name: "nil", layout: nil},
		{name: "all blocks empty", layout: &Layout{Sidebar: &SidebarLayout{}, Topbar: &TopbarLayout{Order: []string{}}, GamePage: &GamePageLayout{}}},
		{name: "empty layout object", layout: &Layout{}},
		{
			name: "full valid",
			layout: &Layout{
				Sidebar:  &SidebarLayout{Side: "right", Order: []string{"downloads", "-", "library", "-"}, Hidden: []string{"stats", "support"}},
				Topbar:   &TopbarLayout{Search: "center", Order: []string{"friends", "notifications", "bigpicture"}, Hidden: []string{"bigpicture"}},
				GamePage: &GamePageLayout{Main: []string{"summary", "achievements"}, Side: []string{"about", "notes"}, Hidden: []string{"reviews"}},
			},
			want: &Layout{
				Sidebar:  &SidebarLayout{Side: "right", Order: []string{"downloads", "-", "library", "-"}, Hidden: []string{"stats", "support"}},
				Topbar:   &TopbarLayout{Search: "center", Order: []string{"friends", "notifications", "bigpicture"}, Hidden: []string{"bigpicture"}},
				GamePage: &GamePageLayout{Main: []string{"summary", "achievements"}, Side: []string{"about", "notes"}, Hidden: []string{"reviews"}},
			},
		},
		{
			name:   "empty blocks dropped",
			layout: &Layout{Sidebar: &SidebarLayout{Side: "left"}, Topbar: &TopbarLayout{}, GamePage: &GamePageLayout{Hidden: []string{}}},
			want:   &Layout{Sidebar: &SidebarLayout{Side: "left"}},
		},
		{name: "default order", layout: &Layout{Sidebar: &SidebarLayout{Order: layoutIDs.Sidebar.DefaultOrder}}, want: &Layout{Sidebar: &SidebarLayout{Order: layoutIDs.Sidebar.DefaultOrder}}},
		{name: "max separators", layout: &Layout{Sidebar: &SidebarLayout{Order: repeat("-", layoutIDs.MaxSeparators)}}, want: &Layout{Sidebar: &SidebarLayout{Order: repeat("-", layoutIDs.MaxSeparators)}}},
		{name: "bad side", layout: &Layout{Sidebar: &SidebarLayout{Side: "top"}}, wantErr: true},
		{name: "long side", layout: &Layout{Sidebar: &SidebarLayout{Side: long}}, wantErr: true},
		{name: "bad search", layout: &Layout{Topbar: &TopbarLayout{Search: "right"}}, wantErr: true},
		{name: "unknown sidebar item", layout: &Layout{Sidebar: &SidebarLayout{Order: []string{"library", "store"}}}, wantErr: true},
		{name: "support is not orderable", layout: &Layout{Sidebar: &SidebarLayout{Order: []string{"support"}}}, wantErr: true},
		{name: "duplicate sidebar item", layout: &Layout{Sidebar: &SidebarLayout{Order: []string{"library", "-", "library"}}}, wantErr: true},
		{name: "too many separators", layout: &Layout{Sidebar: &SidebarLayout{Order: repeat("-", layoutIDs.MaxSeparators+1)}}, wantErr: true},
		{name: "order too long", layout: &Layout{Sidebar: &SidebarLayout{Order: append(slices.Clone(layoutIDs.Sidebar.Items), repeat("-", layoutIDs.MaxSeparators+1)...)}}, wantErr: true},
		{name: "long sidebar id", layout: &Layout{Sidebar: &SidebarLayout{Order: []string{long}}}, wantErr: true},
		{name: "unknown hidden", layout: &Layout{Sidebar: &SidebarLayout{Hidden: []string{"nope"}}}, wantErr: true},
		{name: "duplicate hidden", layout: &Layout{Sidebar: &SidebarLayout{Hidden: []string{"stats", "stats"}}}, wantErr: true},
		{name: "separator hidden", layout: &Layout{Sidebar: &SidebarLayout{Hidden: []string{"-"}}}, wantErr: true},
		{name: "unknown topbar action", layout: &Layout{Topbar: &TopbarLayout{Order: []string{"search"}}}, wantErr: true},
		{name: "duplicate topbar action", layout: &Layout{Topbar: &TopbarLayout{Order: []string{"friends", "friends"}}}, wantErr: true},
		{name: "unknown topbar hidden", layout: &Layout{Topbar: &TopbarLayout{Hidden: []string{"window"}}}, wantErr: true},
		{name: "duplicate topbar hidden", layout: &Layout{Topbar: &TopbarLayout{Hidden: []string{"bigpicture", "bigpicture"}}}, wantErr: true},
		{name: "unknown game section", layout: &Layout{GamePage: &GamePageLayout{Main: []string{"comments"}}}, wantErr: true},
		{name: "section twice in main", layout: &Layout{GamePage: &GamePageLayout{Main: []string{"notes", "notes"}}}, wantErr: true},
		{name: "section twice in side", layout: &Layout{GamePage: &GamePageLayout{Side: []string{"about", "about"}}}, wantErr: true},
		{name: "section in main and side", layout: &Layout{GamePage: &GamePageLayout{Main: []string{"notes"}, Side: []string{"notes"}}}, wantErr: true},
		{name: "unknown game hidden", layout: &Layout{GamePage: &GamePageLayout{Hidden: []string{"comments"}}}, wantErr: true},
		{name: "duplicate game hidden", layout: &Layout{GamePage: &GamePageLayout{Hidden: []string{"reviews", "reviews"}}}, wantErr: true},
		{name: "long game id", layout: &Layout{GamePage: &GamePageLayout{Side: []string{long}}}, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			th := validTheme("with-layout")
			th.Layout = tc.layout
			got, err := Validate(th)
			if tc.wantErr {
				if !errors.Is(err, ErrThemeLayout) {
					t.Fatalf("Validate() error = %v, want ErrThemeLayout", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
			if !reflect.DeepEqual(got.Layout, tc.want) {
				t.Fatalf("Layout = %s, want %s", layoutString(got.Layout), layoutString(tc.want))
			}
		})
	}
}

func layoutString(l *Layout) string {
	data, err := json.Marshal(l)
	if err != nil {
		return err.Error()
	}
	return string(data)
}

func TestSettingsIsNeverHideable(t *testing.T) {
	th := validTheme("hide-settings")
	th.Layout = &Layout{Sidebar: &SidebarLayout{Hidden: []string{"settings"}}}
	if _, err := Validate(th); !errors.Is(err, ErrThemeLayout) {
		t.Fatalf("hiding settings: error = %v, want ErrThemeLayout", err)
	}
	if slices.Contains(layoutIDs.Sidebar.Hideable, "settings") {
		t.Fatal("layout.json lists settings as hideable")
	}
}

func TestValidateDoesNotMutateLayout(t *testing.T) {
	th := validTheme("no-mutation")
	in := &Layout{Sidebar: &SidebarLayout{Side: "left"}, Topbar: &TopbarLayout{}}
	th.Layout = in
	if _, err := Validate(th); err != nil {
		t.Fatal(err)
	}
	if in.Topbar == nil {
		t.Fatal("Validate cleared a block on the caller's layout")
	}
}

const layoutThemeText = `{"version":1,"theme":{"id":"laid-out","name":"Раскладка","base":"dark","tokens":{"--bg":"#101010"},` +
	`"layout":{"sidebar":{"side":"right","order":["downloads","-","library"],"hidden":["support"]},` +
	`"topbar":{"search":"center","hidden":["bigpicture"]},"gamePage":{"main":["notes","summary"],"side":["about"],"hidden":["reviews"]}}}}`

const badLayoutThemeText = `{"version":1,"theme":{"id":"laid-out","name":"Раскладка","base":"dark","tokens":{"--bg":"#101010"},` +
	`"layout":{"sidebar":{"hidden":["settings"]}}}}`

func TestLayoutGoesThroughEveryEntryPoint(t *testing.T) {
	cases := []struct {
		name string
		run  func(t *testing.T, s *Service, text string) (Theme, error)
	}{
		{"ImportText", func(t *testing.T, s *Service, text string) (Theme, error) { return s.ImportText(text, "") }},
		{"Import", func(t *testing.T, s *Service, text string) (Theme, error) {
			return s.Import(importFilePath(t, "laid.typhontheme", []byte(text)))
		}},
		{"Save", func(t *testing.T, s *Service, text string) (Theme, error) {
			var f file
			if err := json.Unmarshal([]byte(text), &f); err != nil {
				t.Fatal(err)
			}
			return s.Save(f.Theme)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name+"/valid", func(t *testing.T) {
			statePath := filepath.Join(t.TempDir(), "themes.json")
			s := mustServiceAt(t, statePath)
			got, err := tc.run(t, s, layoutThemeText)
			if err != nil {
				t.Fatalf("error = %v", err)
			}
			if got.Layout == nil || got.Layout.Sidebar.Side != "right" || got.Layout.GamePage.Main[0] != "notes" {
				t.Fatalf("layout = %s", layoutString(got.Layout))
			}
			reloaded, err := mustServiceAt(t, statePath).Get("laid-out")
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(reloaded.Layout, got.Layout) {
				t.Fatalf("layout after restart = %s, want %s", layoutString(reloaded.Layout), layoutString(got.Layout))
			}
		})
		t.Run(tc.name+"/invalid", func(t *testing.T) {
			s := mustServiceAt(t, filepath.Join(t.TempDir(), "themes.json"))
			if _, err := tc.run(t, s, badLayoutThemeText); !errors.Is(err, ErrThemeLayout) {
				t.Fatalf("error = %v, want ErrThemeLayout", err)
			}
			if len(s.List()) != len(presets) {
				t.Fatal("refused layout stored a theme")
			}
		})
	}
}

func TestWatcherRefusesBadLayout(t *testing.T) {
	w := newWatched(t)
	w.s.pollOnce(t.Context())
	writeLater(t, w.path, badLayoutThemeText, time.Hour)
	w.s.pollOnce(t.Context())
	failed := w.rec.named("theme:source-failed")
	if len(failed) != 1 {
		t.Fatalf("theme:source-failed emitted %d times, want 1", len(failed))
	}
	failure, ok := failed[0].data.(SourceFailure)
	if !ok || failure.Code != "theme.invalid_layout" {
		t.Fatalf("failure = %#v, want code theme.invalid_layout", failed[0].data)
	}
	got, err := w.s.Get("live")
	if err != nil {
		t.Fatal(err)
	}
	if got.Layout != nil {
		t.Fatalf("refused layout reached the stored theme: %s", layoutString(got.Layout))
	}

	writeLater(t, w.path, strings.Replace(layoutThemeText, `"laid-out"`, `"live"`, 1), 2*time.Hour)
	w.s.pollOnce(t.Context())
	got, err = w.s.Get("live")
	if err != nil {
		t.Fatal(err)
	}
	if got.Layout == nil || got.Layout.Topbar.Search != "center" {
		t.Fatalf("reloaded layout = %s", layoutString(got.Layout))
	}
}

func TestExportWritesLayout(t *testing.T) {
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "themes.json"))
	saved, err := s.ImportText(layoutThemeText, "")
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "out.typhontheme")
	if err := s.Export(saved.ID, out); err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	data, err := os.ReadFile(filepath.Clean(out))
	if err != nil {
		t.Fatal(err)
	}
	back, err := decodeTheme(data, "")
	if err != nil {
		t.Fatalf("exported file does not import: %v", err)
	}
	if !reflect.DeepEqual(back.Layout, saved.Layout) {
		t.Fatalf("exported layout = %s, want %s", layoutString(back.Layout), layoutString(saved.Layout))
	}
}

func TestPlainThemeHasNoLayoutKey(t *testing.T) {
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "themes.json"))
	th := validTheme("plain")
	th.Layout = &Layout{Sidebar: &SidebarLayout{}}
	saved, err := s.Save(th)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "plain.typhontheme")
	if err := s.Export(saved.ID, out); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Clean(out))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"layout"`) {
		t.Fatalf("plain theme exported a layout: %s", data)
	}
}
