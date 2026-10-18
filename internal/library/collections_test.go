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

func TestCollectionsCreateUpdateDelete(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)
	if got := s.GetCollections(); len(got) != 0 {
		t.Fatalf("fresh service has collections: %+v", got)
	}
	created, err := s.SaveCollection(Collection{Name: "  Co-op   night ", Tags: []string{"Co-op", "co-op"}, Genres: []string{" RPG ", ""}, Status: StatusBacklog})
	if err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || created.Name != "Co-op night" || !reflect.DeepEqual(created.Tags, []string{"Co-op"}) ||
		!reflect.DeepEqual(created.Genres, []string{"RPG"}) || created.CreatedAt.IsZero() || created.UpdatedAt.IsZero() {
		t.Fatalf("created = %+v", created)
	}

	updated, err := s.SaveCollection(Collection{ID: created.ID, Name: "Renamed"})
	if err != nil {
		t.Fatal(err)
	}
	if updated.ID != created.ID || updated.Name != "Renamed" || !updated.CreatedAt.Equal(created.CreatedAt) ||
		len(updated.Tags) != 0 || updated.Status != "" {
		t.Fatalf("updated = %+v", updated)
	}

	reloaded := mustServiceAt(t, path).GetCollections()
	if len(reloaded) != 1 || reloaded[0].Name != "Renamed" || reloaded[0].ID != created.ID {
		t.Fatalf("reloaded = %+v", reloaded)
	}

	if err := s.DeleteCollection(created.ID); err != nil {
		t.Fatal(err)
	}
	if got := mustServiceAt(t, path).GetCollections(); len(got) != 0 {
		t.Fatalf("after delete: %+v", got)
	}
	if err := s.DeleteCollection(created.ID); uierr.Code(err) != "library.collection_not_found" {
		t.Fatalf("second delete err = %v", err)
	}
	if _, err := s.SaveCollection(Collection{ID: created.ID, Name: "x"}); uierr.Code(err) != "library.collection_not_found" {
		t.Fatalf("update of a missing collection err = %v", err)
	}
}

func TestCollectionsValidation(t *testing.T) {
	tooManyGenres := manyTags(MaxCollectionGenre + 1)
	tests := []struct {
		name string
		in   Collection
		code string
	}{
		{"empty name", Collection{Name: ""}, "library.collection_name_invalid"},
		{"blank name", Collection{Name: "   "}, "library.collection_name_invalid"},
		{"long name", Collection{Name: strings.Repeat("я", MaxCollectionName+1)}, "library.collection_name_invalid"},
		{"name at the limit", Collection{Name: strings.Repeat("я", MaxCollectionName)}, ""},
		{"bad status", Collection{Name: "x", Status: "nope"}, "library.invalid_status"},
		{"bad tag", Collection{Name: "x", Tags: []string{"a\x00"}}, "library.tag_invalid"},
		{"too many tags", Collection{Name: "x", Tags: manyTags(MaxTags + 1)}, "library.too_many_tags"},
		{"too many genres", Collection{Name: "x", Genres: tooManyGenres}, "library.collection_genres_invalid"},
		{"long genre", Collection{Name: "x", Genres: []string{strings.Repeat("g", MaxCollectionName+1)}}, "library.collection_genres_invalid"},
		{"control char in genre", Collection{Name: "x", Genres: []string{"rpg\x01"}}, "library.collection_genres_invalid"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
			_, err := s.SaveCollection(tc.in)
			if code := uierr.Code(err); code != tc.code {
				t.Fatalf("code = %q (err %v), want %q", code, err, tc.code)
			}
			if tc.code != "" && len(s.GetCollections()) != 0 {
				t.Fatal("a rejected collection was stored")
			}
		})
	}
}

func TestCollectionsLimit(t *testing.T) {
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	for i := 0; i < MaxCollections; i++ {
		if _, err := s.SaveCollection(Collection{Name: "c"}); err != nil {
			t.Fatalf("collection %d: %v", i, err)
		}
	}
	if _, err := s.SaveCollection(Collection{Name: "one more"}); uierr.Code(err) != "library.too_many_collections" {
		t.Fatalf("err = %v", err)
	}
	first := s.GetCollections()[0]
	if _, err := s.SaveCollection(Collection{ID: first.ID, Name: "still editable"}); err != nil {
		t.Fatalf("update at the limit: %v", err)
	}
}

func TestCollectionsBrokenFileFailsTheConstructor(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "library.json")
	if err := os.WriteFile(filepath.Join(dir, collectionsName), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewServiceAt(path); err == nil {
		t.Fatal("a corrupt collections.json must stop the service from starting")
	}
}

func TestCollectionsUnreadableFileFailsTheConstructor(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "library.json")
	if err := os.Mkdir(filepath.Join(dir, collectionsName), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := NewServiceAt(path); err == nil {
		t.Fatal("an unreadable collections.json is not the same as a missing one")
	}
}

func TestCollectionsMissingFileMeansEmpty(t *testing.T) {
	dir := t.TempDir()
	s := mustServiceAt(t, filepath.Join(dir, "library.json"))
	if got := s.GetCollections(); got == nil || len(got) != 0 {
		t.Fatalf("collections = %#v, want empty non-nil", got)
	}
	if _, err := os.Stat(filepath.Join(dir, collectionsName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("loading must not create the file: %v", err)
	}
}

func TestCollectionsRollBackWhenTheSaveFails(t *testing.T) {
	setup := func(t *testing.T) (*Service, Collection) {
		t.Helper()
		s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
		c, err := s.SaveCollection(Collection{Name: "keep", Tags: []string{"a"}})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(s.collectionsPath); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(s.collectionsPath, 0o755); err != nil {
			t.Fatal(err)
		}
		return s, c
	}
	tests := []struct {
		name string
		run  func(s *Service, c Collection) error
	}{
		{"create", func(s *Service, _ Collection) error { _, err := s.SaveCollection(Collection{Name: "new"}); return err }},
		{"update", func(s *Service, c Collection) error {
			_, err := s.SaveCollection(Collection{ID: c.ID, Name: "changed"})
			return err
		}},
		{"delete", func(s *Service, c Collection) error { return s.DeleteCollection(c.ID) }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s, c := setup(t)
			if err := tc.run(s, c); err == nil {
				t.Fatal("the save failed but the operation reported success")
			}
			got := s.GetCollections()
			if len(got) != 1 || !reflect.DeepEqual(got[0], c) {
				t.Fatalf("memory kept a change that never reached the disk: %+v, want [%+v]", got, c)
			}
		})
	}
}

func TestGetCollectionsReturnsACopy(t *testing.T) {
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	if _, err := s.SaveCollection(Collection{Name: "x", Tags: []string{"a"}}); err != nil {
		t.Fatal(err)
	}
	got := s.GetCollections()
	got[0].Tags[0] = "mutated"
	got[0].Name = "mutated"
	if again := s.GetCollections(); again[0].Name != "x" || again[0].Tags[0] != "a" {
		t.Fatalf("the caller reached the service state: %+v", again)
	}
}
