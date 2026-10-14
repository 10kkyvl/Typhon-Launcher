package library

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"typhon/internal/uierr"
)

func manyTags(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = "t" + strings.Repeat("x", i)
	}
	return out
}

func TestNormalizeTags(t *testing.T) {
	long := strings.Repeat("я", MaxTagLen)
	many := manyTags(MaxTags + 1)
	tests := []struct {
		name string
		in   []string
		want []string
		code string
	}{
		{"nil clears", nil, nil, ""},
		{"empty clears", []string{}, nil, ""},
		{"blank dropped", []string{"", "  ", "\t"}, nil, ""},
		{"trim and collapse", []string{"  co   op  ", "rpg"}, []string{"co op", "rpg"}, ""},
		{"dedupe ignores case keeps first", []string{"RPG", "rpg", "Rpg", "Co-op"}, []string{"RPG", "Co-op"}, ""},
		{"keeps input order", []string{"zebra", "apple"}, []string{"zebra", "apple"}, ""},
		{"max length in runes", []string{long}, []string{long}, ""},
		{"too long", []string{long + "я"}, nil, "library.tag_too_long"},
		{"control character", []string{"a\x00b"}, nil, "library.tag_invalid"},
		{"escape character", []string{"\x1b[0m"}, nil, "library.tag_invalid"},
		{"exactly the limit", many[:MaxTags], many[:MaxTags], ""},
		{"over the limit", many, nil, "library.too_many_tags"},
		{"duplicates do not count toward the limit", append(append([]string{}, many[:MaxTags]...), "T", "TX"), many[:MaxTags], ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeTags(tc.in)
			if code := uierr.Code(err); code != tc.code {
				t.Fatalf("code = %q (err %v), want %q", code, err, tc.code)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSetTagsPersistsAndNormalizes(t *testing.T) {
	f := newSaveFixture(t)
	g, err := f.s.SetTags(f.a.ID, []string{" Co  op ", "co op", "RPG"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(g.Tags, []string{"Co op", "RPG"}) {
		t.Fatalf("tags = %q", g.Tags)
	}
	for _, r := range mustServiceAt(t, f.path).GetGames() {
		if r.ID == f.a.ID && !reflect.DeepEqual(r.Tags, []string{"Co op", "RPG"}) {
			t.Fatalf("reloaded tags = %q", r.Tags)
		}
	}
	cleared, err := f.s.SetTags(f.a.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cleared.Tags != nil {
		t.Fatalf("tags after clearing = %q", cleared.Tags)
	}
	data, err := os.ReadFile(f.path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"tags"`) {
		t.Fatalf("empty tags must be omitted from the file: %s", data)
	}
}

func TestSetTagsRejectsBadInputWithoutTouchingTheGame(t *testing.T) {
	f := newSaveFixture(t)
	if _, err := f.s.SetTags(f.a.ID, []string{"keep"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.SetTags(f.a.ID, []string{"a\x07"}); uierr.Code(err) != "library.tag_invalid" {
		t.Fatalf("err = %v", err)
	}
	if _, err := f.s.SetTags("missing", []string{"x"}); !errors.Is(err, errNotFound) {
		t.Fatalf("err = %v, want errNotFound", err)
	}
	for _, g := range f.s.GetGames() {
		if g.ID == f.a.ID && !reflect.DeepEqual(g.Tags, []string{"keep"}) {
			t.Fatalf("tags = %q", g.Tags)
		}
	}
}

func TestSetTagsAndSetHiddenRollBackWhenTheSaveFails(t *testing.T) {
	tests := []struct {
		name string
		run  func(f *saveFixture) error
	}{
		{"SetTags", func(f *saveFixture) error { _, err := f.s.SetTags(f.a.ID, []string{"x"}); return err }},
		{"SetHidden", func(f *saveFixture) error { _, err := f.s.SetHidden(f.a.ID, true); return err }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newSaveFixture(t)
			before := f.snapshot()
			f.breakSaving(t)
			if err := tc.run(f); err == nil {
				t.Fatal("the save failed but the operation reported success")
			}
			if after := f.snapshot(); !reflect.DeepEqual(before, after) {
				t.Fatalf("memory kept a change that never reached the disk\nbefore %+v\nafter  %+v", before, after)
			}
			f.repairSaving(t)
			if _, err := f.s.SetStatus(f.a.ID, StatusPlaying); err != nil {
				t.Fatal(err)
			}
			for _, g := range mustServiceAt(t, f.path).GetGames() {
				if g.ID == f.a.ID && (len(g.Tags) != 0 || g.Hidden) {
					t.Fatalf("the failed operation leaked into a later save: %+v", g)
				}
			}
		})
	}
}

func TestSetHidden(t *testing.T) {
	f := newSaveFixture(t)
	g, err := f.s.SetHidden(f.a.ID, true)
	if err != nil || !g.Hidden {
		t.Fatalf("hide: %+v, %v", g, err)
	}
	visible := false
	for _, x := range f.s.GetGames() {
		visible = visible || x.ID == f.a.ID
	}
	if !visible {
		t.Fatal("a hidden game must stay in GetGames: the frontend filters it")
	}
	for _, r := range mustServiceAt(t, f.path).GetGames() {
		if r.ID == f.a.ID && !r.Hidden {
			t.Fatal("hidden flag was not persisted")
		}
	}
	g, err = f.s.SetHidden(f.a.ID, false)
	if err != nil || g.Hidden {
		t.Fatalf("unhide: %+v, %v", g, err)
	}
	if _, err := f.s.SetHidden("missing", true); !errors.Is(err, errNotFound) {
		t.Fatalf("err = %v, want errNotFound", err)
	}
}

func TestOldLibraryWithoutTagsAndHiddenStillLoads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	seed := `[{"id": "a", "title": "A", "installDir": "", "executable": "", "favorite": true}]`
	if err := os.WriteFile(path, []byte(seed), 0o600); err != nil {
		t.Fatal(err)
	}
	games := mustServiceAt(t, path).GetGames()
	if len(games) != 1 || games[0].Tags != nil || games[0].Hidden || !games[0].Favorite {
		t.Fatalf("games = %+v", games)
	}
}

func TestAllTags(t *testing.T) {
	f := newSaveFixture(t)
	if got := f.s.AllTags(); len(got) != 0 {
		t.Fatalf("tags = %q, want none", got)
	}
	if _, err := f.s.SetTags(f.a.ID, []string{"RPG", "Co-op"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.SetTags(f.u.ID, []string{"rpg", "Indie"}); err != nil {
		t.Fatal(err)
	}
	want := []string{"RPG", "Co-op", "Indie"}
	if got := f.s.AllTags(); !reflect.DeepEqual(got, want) {
		t.Fatalf("tags = %q, want %q", got, want)
	}
}

func TestAllTagsIgnoresRemovedGames(t *testing.T) {
	f := newSaveFixture(t)
	if _, err := f.s.SetTags(f.u.ID, []string{"gone"}); err != nil {
		t.Fatal(err)
	}
	if err := f.s.RemoveGame(f.u.ID); err != nil {
		t.Fatal(err)
	}
	if got := f.s.AllTags(); len(got) != 0 {
		t.Fatalf("tags = %q, want none", got)
	}
}
