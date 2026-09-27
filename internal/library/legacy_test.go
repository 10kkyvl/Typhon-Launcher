package library

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadMigratesLegacyCompleted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	at := time.Date(2026, 8, 22, 10, 0, 0, 0, time.UTC)
	raw := fmt.Sprintf(`[{"id":"a","title":"A","completed":true,"completedAt":%q},{"id":"b","title":"B","completed":false},{"id":"c","title":"C","status":"paused","statusAt":%q},{"id":"d","title":"D","favorite":true},{"id":"e","title":"E","status":"backlog"},{"id":"f","title":"F","completed":true}]`, at.Format(time.RFC3339), at.Format(time.RFC3339))
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	s := mustServiceAt(t, path)
	games := s.GetGames()
	byID := map[string]Game{}
	for _, g := range games {
		byID[g.ID] = g
	}
	if byID["a"].Status != StatusCompleted || byID["a"].StatusAt == nil || !byID["a"].StatusAt.Equal(at) {
		t.Fatalf("a = %+v, want completed at %s", byID["a"], at)
	}
	if byID["b"].Status != "" || byID["b"].StatusAt != nil {
		t.Fatalf("b = %+v, want no status", byID["b"])
	}
	if byID["c"].Status != StatusPaused {
		t.Fatalf("c = %+v, want paused kept", byID["c"])
	}
	if byID["a"].FavoriteAt != nil {
		t.Fatalf("a = %+v, want no favorite stamp", byID["a"])
	}
	epoch := time.Unix(0, 0).UTC()
	if byID["d"].FavoriteAt == nil || !byID["d"].FavoriteAt.Equal(epoch) {
		t.Fatalf("d = %+v, want favorite stamped at the epoch", byID["d"])
	}
	if byID["d"].StatusAt != nil {
		t.Fatalf("d = %+v, want no status stamp", byID["d"])
	}
	if byID["e"].StatusAt == nil || !byID["e"].StatusAt.Equal(epoch) {
		t.Fatalf("e = %+v, want status stamped at the epoch", byID["e"])
	}
	if byID["f"].Status != StatusCompleted || byID["f"].StatusAt == nil || !byID["f"].StatusAt.Equal(epoch) {
		t.Fatalf("f = %+v, want legacy completed stamped at the epoch", byID["f"])
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	reread, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(reread) != raw {
		t.Fatal("load must not persist migrated stamps")
	}
	if _, err := s.SetStatus("b", StatusBacklog); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"completed":`) || strings.Contains(string(data), `"completedAt":`) {
		t.Fatalf("legacy keys survived persist: %s", data)
	}
}
