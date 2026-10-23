package main

import (
	"path/filepath"
	"testing"
	"time"

	"typhon/internal/accountsync"
	"typhon/internal/library"
)

func TestSyncLibraryRemoveDeletesRecord(t *testing.T) {
	s, err := library.NewServiceAt(filepath.Join(t.TempDir(), "library.json"))
	if err != nil {
		t.Fatal(err)
	}
	g, err := s.AddCatalogGame("catalog-c1", "Game", "")
	if err != nil {
		t.Fatal(err)
	}
	err = (syncLibrary{svc: s}).Remove("catalog-c1")
	_, findErr := s.Find(g.ID)
	if err != nil || findErr == nil {
		t.Fatalf("Remove: err = %v, record still present = %v", err, findErr == nil)
	}
}

func TestSyncLibraryCarriesTheCompletionDateBothWays(t *testing.T) {
	s, err := library.NewServiceAt(filepath.Join(t.TempDir(), "library.json"))
	if err != nil {
		t.Fatal(err)
	}
	g, err := s.AddCatalogGame("catalog-c1", "Game", "")
	if err != nil {
		t.Fatal(err)
	}
	adapter := syncLibrary{svc: s}
	status := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	done := status.Add(-24 * time.Hour)
	err = adapter.Apply([]accountsync.Game{{CanonicalGameID: "catalog-c1", Status: library.StatusMastered, StatusAt: &status, CompletedAt: &done}})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	stored, err := s.Find(g.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != library.StatusMastered || stored.CompletedAt == nil || !stored.CompletedAt.Equal(done) {
		t.Fatalf("stored game = %+v, want mastered with the synced date", stored)
	}
	snapshot, err := adapter.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot) != 1 || snapshot[0].CompletedAt == nil || !snapshot[0].CompletedAt.Equal(done) {
		t.Fatalf("snapshot = %+v, want the date to come back", snapshot)
	}
}
