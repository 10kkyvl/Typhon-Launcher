package library

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"typhon/internal/uierr"
)

func notesOf(t *testing.T, s *Service, id string) string {
	t.Helper()
	for _, g := range s.GetGames() {
		if g.ID == id {
			return g.Notes
		}
	}
	t.Fatalf("game %s not found", id)
	return ""
}

func TestSetNotesNormalizes(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "hello", "hello"},
		{"keeps leading and inner formatting", "  a\n\n\tb", "  a\n\n\tb"},
		{"crlf", "a\r\nb\rc", "a\nb\nc"},
		{"trims trailing whitespace", "a  \n\n \t", "a"},
		{"empty", "", ""},
		{"only whitespace", " \r\n ", ""},
		{"max runes", strings.Repeat("я", MaxNotesChars), strings.Repeat("я", MaxNotesChars)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newSaveFixture(t)
			if err := f.s.SetNotes(f.a.ID, tc.in); err != nil {
				t.Fatal(err)
			}
			if got := notesOf(t, f.s, f.a.ID); got != tc.want {
				t.Fatalf("notes = %q, want %q", got, tc.want)
			}
			if got := notesOf(t, mustServiceAt(t, f.path), f.a.ID); got != tc.want {
				t.Fatalf("reloaded notes = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSetNotesOverwriteAndClear(t *testing.T) {
	f := newSaveFixture(t)
	for _, text := range []string{"one", "two", "two", ""} {
		if err := f.s.SetNotes(f.a.ID, text); err != nil {
			t.Fatal(err)
		}
		if got := notesOf(t, f.s, f.a.ID); got != text {
			t.Fatalf("notes = %q, want %q", got, text)
		}
	}
	data, err := os.ReadFile(f.path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"notes"`) {
		t.Fatalf("empty notes must be omitted from the file: %s", data)
	}
}

func TestSetNotesRejectsBadInput(t *testing.T) {
	tests := []struct {
		name string
		in   string
		code string
	}{
		{"too long", strings.Repeat("a", MaxNotesChars+1), "library.notes_too_long"},
		{"too long in runes not bytes", strings.Repeat("я", MaxNotesChars+1), "library.notes_too_long"},
		{"invalid utf8", "a\xffb", "library.notes_invalid"},
		{"nul", "a\x00b", "library.notes_invalid"},
		{"bell", "a\x07", "library.notes_invalid"},
		{"escape", "\x1b[0m", "library.notes_invalid"},
		{"c1 control", "a\u0085b", "library.notes_invalid"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newSaveFixture(t)
			if err := f.s.SetNotes(f.a.ID, "keep"); err != nil {
				t.Fatal(err)
			}
			if err := f.s.SetNotes(f.a.ID, tc.in); uierr.Code(err) != tc.code {
				t.Fatalf("err = %v, want code %s", err, tc.code)
			}
			if got := notesOf(t, f.s, f.a.ID); got != "keep" {
				t.Fatalf("notes = %q after a rejected write", got)
			}
		})
	}
}

func TestSetNotesUnknownID(t *testing.T) {
	f := newSaveFixture(t)
	if err := f.s.SetNotes("missing", "x"); !errors.Is(err, errNotFound) {
		t.Fatalf("err = %v, want errNotFound", err)
	}
}

func TestNotesLimit(t *testing.T) {
	f := newSaveFixture(t)
	if f.s.NotesLimit() != MaxNotesChars {
		t.Fatalf("NotesLimit = %d", f.s.NotesLimit())
	}
}

func TestSetNotesRollsBackWhenTheSaveFails(t *testing.T) {
	f := newSaveFixture(t)
	if err := f.s.SetNotes(f.a.ID, "before"); err != nil {
		t.Fatal(err)
	}
	before := f.snapshot()
	f.breakSaving(t)
	if err := f.s.SetNotes(f.a.ID, "after"); err == nil {
		t.Fatal("the save failed but SetNotes reported success")
	}
	if after := f.snapshot(); !reflect.DeepEqual(before, after) {
		t.Fatalf("memory kept a change that never reached the disk\nbefore %+v\nafter  %+v", before, after)
	}
	f.repairSaving(t)
	if _, err := f.s.SetStatus(f.a.ID, StatusPlaying); err != nil {
		t.Fatal(err)
	}
	if got := notesOf(t, mustServiceAt(t, f.path), f.a.ID); got != "before" {
		t.Fatalf("the failed write leaked into a later save: %q", got)
	}
}

func TestRemovedGameKeepsNotesInArchive(t *testing.T) {
	f := newSaveFixture(t)
	if err := f.s.SetNotes(f.a.ID, "remember"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.RemoveGame(f.a.ID); err != nil {
		t.Fatal(err)
	}
	for name, s := range map[string]*Service{"live": f.s, "reloaded": mustServiceAt(t, f.path)} {
		found := false
		for _, g := range s.GetHistoryGames() {
			if g.ID == f.a.ID {
				found = true
				if g.Notes != "remember" {
					t.Fatalf("%s: archived notes = %q", name, g.Notes)
				}
			}
		}
		if !found {
			t.Fatalf("%s: archived game missing", name)
		}
	}
}

func TestLoadWithoutNotesField(t *testing.T) {
	f := newSaveFixture(t)
	for _, g := range mustServiceAt(t, f.path).GetGames() {
		if g.Notes != "" {
			t.Fatalf("notes = %q for a library saved without the field", g.Notes)
		}
	}
}

func TestSyncSnapshotCarriesNoNotes(t *testing.T) {
	f := newSaveFixture(t)
	if err := f.s.SetNotes(f.a.ID, "private"); err != nil {
		t.Fatal(err)
	}
	for _, item := range f.s.SyncSnapshot() {
		if strings.Contains(reflect.ValueOf(item).String(), "private") {
			t.Fatal("sync snapshot leaks notes")
		}
		v := reflect.ValueOf(item)
		for i := 0; i < v.NumField(); i++ {
			if v.Type().Field(i).Name == "Notes" {
				t.Fatal("SyncGame has a Notes field")
			}
		}
	}
}
