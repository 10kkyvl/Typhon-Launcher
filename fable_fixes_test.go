package main

import (
	"path/filepath"
	"testing"
	"typhon/internal/library"
)

func TestAuditFix002(t *testing.T) {
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
		t.Fatalf("002 reproduced: adapter removal fails (%v), record remains", err)
	}
}
