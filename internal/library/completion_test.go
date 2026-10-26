package library

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"typhon/internal/uierr"
)

func day(n int) time.Time { return time.Date(2026, 9, n, 12, 0, 0, 0, time.UTC) }

func TestMasteredIsAValidStatus(t *testing.T) {
	if !ValidStatus(StatusMastered) {
		t.Fatal("ValidStatus rejects mastered")
	}
	if ValidStatus("won") {
		t.Fatal("ValidStatus accepts an unknown status")
	}
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	saved, err := s.SaveCollection(Collection{Name: "Done", Status: StatusMastered})
	if err != nil {
		t.Fatalf("SaveCollection with mastered: %v", err)
	}
	if saved.Status != StatusMastered {
		t.Fatalf("collection status = %q", saved.Status)
	}
}

func TestSetStatusKeepsCompletionDateByTransition(t *testing.T) {
	tests := []struct {
		name     string
		steps    []string
		wantStep int
	}{
		{"into completed stamps now", []string{StatusCompleted}, 0},
		{"into mastered stamps now", []string{StatusMastered}, 0},
		{"completed to mastered keeps the date", []string{StatusCompleted, StatusMastered}, 0},
		{"mastered to completed keeps the date", []string{StatusMastered, StatusCompleted}, 0},
		{"completed to playing clears", []string{StatusCompleted, StatusPlaying}, -1},
		{"mastered to empty clears", []string{StatusMastered, ""}, -1},
		{"completed to dropped clears", []string{StatusCompleted, StatusDropped}, -1},
		{"playing to completed stamps the later moment", []string{StatusPlaying, StatusCompleted}, 1},
		{"completed, away and back stamps again", []string{StatusCompleted, StatusBacklog, StatusCompleted}, 2},
		{"mastered, away and back as completed stamps again", []string{StatusMastered, StatusPaused, StatusCompleted, StatusMastered}, 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
			id := addGames(t, s, 1)[0]
			var clock time.Time
			s.now = func() time.Time { return clock }
			var got Game
			for i, status := range tc.steps {
				clock = day(1 + i)
				var err error
				got, err = s.SetStatus(id, status)
				if err != nil {
					t.Fatalf("SetStatus %q: %v", status, err)
				}
			}
			if tc.wantStep < 0 {
				if got.CompletedAt != nil {
					t.Fatalf("CompletedAt = %v, want nil", got.CompletedAt)
				}
			} else if want := day(1 + tc.wantStep); got.CompletedAt == nil || !got.CompletedAt.Equal(want) {
				t.Fatalf("CompletedAt = %v, want %v", got.CompletedAt, want)
			}
			stored, err := s.Find(id)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(stored.CompletedAt, got.CompletedAt) {
				t.Fatalf("stored CompletedAt = %v, returned %v", stored.CompletedAt, got.CompletedAt)
			}
		})
	}
}

func TestSetStatusWithoutDateInHandsOverUnknownDate(t *testing.T) {
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	id := addGames(t, s, 1)[0]
	s.mu.Lock()
	g := s.findLocked(id)
	g.Status = StatusCompleted
	stamp := day(1)
	g.StatusAt = &stamp
	s.mu.Unlock()

	got, err := s.SetStatus(id, StatusMastered)
	if err != nil {
		t.Fatal(err)
	}
	if got.CompletedAt != nil {
		t.Fatalf("CompletedAt = %v, want nil: an unknown date must not be invented on completed -> mastered", got.CompletedAt)
	}
}

func TestSetCompletedAt(t *testing.T) {
	now := day(10)
	tests := []struct {
		name   string
		status string
		date   time.Time
		code   string
	}{
		{"completed accepts a past date", StatusCompleted, day(3), ""},
		{"mastered accepts a past date", StatusMastered, day(3), ""},
		{"date past the allowed skew", StatusCompleted, now.Add(25 * time.Hour), "library.completed_at_in_future"},
		{"the first allowed day", StatusCompleted, time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC), ""},
		{"day before 1980", StatusCompleted, time.Date(1979, 12, 31, 23, 59, 59, 0, time.UTC), "library.completed_at_too_old"},
		{"zero date", StatusCompleted, time.Time{}, "library.completed_at_required"},
		{"not started", "", day(3), "library.completed_at_needs_status"},
		{"playing", StatusPlaying, day(3), "library.completed_at_needs_status"},
		{"dropped", StatusDropped, day(3), "library.completed_at_needs_status"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "library.json")
			s := mustServiceAt(t, path)
			id := addGames(t, s, 1)[0]
			s.now = func() time.Time { return day(1) }
			if _, err := s.SetStatus(id, tc.status); err != nil {
				t.Fatal(err)
			}
			before, err := s.Find(id)
			if err != nil {
				t.Fatal(err)
			}
			s.now = func() time.Time { return now }

			err = s.SetCompletedAt(id, tc.date)
			after, findErr := s.Find(id)
			if findErr != nil {
				t.Fatal(findErr)
			}
			if tc.code != "" {
				if uierr.Code(err) != tc.code {
					t.Fatalf("err = %v, want code %q", err, tc.code)
				}
				if !reflect.DeepEqual(before, after) {
					t.Fatalf("a rejected call changed the game\nbefore %+v\nafter  %+v", before, after)
				}
				return
			}
			if err != nil {
				t.Fatalf("SetCompletedAt: %v", err)
			}
			if after.CompletedAt == nil || !after.CompletedAt.Equal(tc.date) {
				t.Fatalf("CompletedAt = %v, want %v", after.CompletedAt, tc.date)
			}
			if after.StatusAt == nil || !after.StatusAt.Equal(now) {
				t.Fatalf("StatusAt = %v, want %v: the edit has to win the sync merge", after.StatusAt, now)
			}
			if after.Status != tc.status {
				t.Fatalf("status = %q, want %q", after.Status, tc.status)
			}
			reloaded, err := mustServiceAt(t, path).Find(id)
			if err != nil {
				t.Fatal(err)
			}
			if reloaded.CompletedAt == nil || !reloaded.CompletedAt.Equal(tc.date) {
				t.Fatalf("reloaded CompletedAt = %v, want %v", reloaded.CompletedAt, tc.date)
			}
		})
	}
}

func TestSetCompletedAtUnknownGame(t *testing.T) {
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	if err := s.SetCompletedAt("missing", day(1)); !errors.Is(err, errNotFound) {
		t.Fatalf("err = %v, want errNotFound", err)
	}
}

func TestSetCompletedAtSameDateDoesNotRewrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)
	id := addGames(t, s, 1)[0]
	s.now = func() time.Time { return day(5) }
	if _, err := s.SetStatus(id, StatusCompleted); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s.now = func() time.Time { return day(6) }
	if err := s.SetCompletedAt(id, day(5)); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("setting the same date rewrote the file and bumped statusAt")
	}
}

func TestSetCompletedAtRollsBackMemoryWhenTheSaveFails(t *testing.T) {
	f := newSaveFixture(t)
	f.s.now = func() time.Time { return day(5) }
	if _, err := f.s.SetStatus(f.a.ID, StatusCompleted); err != nil {
		t.Fatal(err)
	}
	before := f.snapshot()
	f.breakSaving(t)

	f.s.now = func() time.Time { return day(9) }
	if err := f.s.SetCompletedAt(f.a.ID, day(2)); err == nil {
		t.Fatal("the save failed but SetCompletedAt reported success")
	}
	if after := f.snapshot(); !reflect.DeepEqual(before, after) {
		t.Fatalf("memory kept a date that never reached the disk\nbefore %+v\nafter  %+v", before, after)
	}

	f.repairSaving(t)
	if _, err := f.s.SetStatus(f.a.ID, StatusPlaying); err != nil {
		t.Fatalf("the next save failed: %v", err)
	}
	reloaded, err := mustServiceAt(t, f.path).Find(f.a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.CompletedAt != nil {
		t.Fatalf("the rolled back date came back after a later save: %v", reloaded.CompletedAt)
	}
}

func TestCompletionDateSurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)
	ids := addGames(t, s, 2)
	s.now = func() time.Time { return day(4) }
	if _, err := s.SetStatus(ids[0], StatusMastered); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetStatus(ids[1], StatusCompleted); err != nil {
		t.Fatal(err)
	}

	reloaded := mustServiceAt(t, path)
	for _, id := range ids {
		g, err := reloaded.Find(id)
		if err != nil {
			t.Fatal(err)
		}
		if g.CompletedAt == nil || !g.CompletedAt.Equal(day(4)) {
			t.Fatalf("game %s reloaded with CompletedAt = %v, want %v", id, g.CompletedAt, day(4))
		}
	}
	if g, err := reloaded.Find(ids[0]); err != nil || g.Status != StatusMastered {
		t.Fatalf("mastered reloaded as %q (err %v)", g.Status, err)
	}
}

func TestLoadDoesNotInventCompletionDate(t *testing.T) {
	stamp := day(2).Format(time.RFC3339)
	tests := []struct {
		name string
		raw  string
		want *time.Time
	}{
		{"completed without a date", `[{"id":"a","title":"A","status":"completed","statusAt":"` + stamp + `"}]`, nil},
		{"mastered without a date", `[{"id":"a","title":"A","status":"mastered","statusAt":"` + stamp + `"}]`, nil},
		{"completed with a date", `[{"id":"a","title":"A","status":"completed","statusAt":"` + stamp + `","completedAt":"` + day(1).Format(time.RFC3339) + `"}]`, func() *time.Time { d := day(1); return &d }()},
		{"date on a status that is not completed is dropped", `[{"id":"a","title":"A","status":"playing","statusAt":"` + stamp + `","completedAt":"` + day(1).Format(time.RFC3339) + `"}]`, nil},
		{"legacy completed flag keeps no date", `[{"id":"a","title":"A","completed":true,"completedAt":"` + day(1).Format(time.RFC3339) + `"}]`, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "library.json")
			if err := os.WriteFile(path, []byte(tc.raw), 0o644); err != nil {
				t.Fatal(err)
			}
			g, err := mustServiceAt(t, path).Find("a")
			if err != nil {
				t.Fatal(err)
			}
			switch {
			case tc.want == nil && g.CompletedAt != nil:
				t.Fatalf("CompletedAt = %v, want nil", g.CompletedAt)
			case tc.want != nil && (g.CompletedAt == nil || !g.CompletedAt.Equal(*tc.want)):
				t.Fatalf("CompletedAt = %v, want %v", g.CompletedAt, tc.want)
			}
		})
	}
}

func TestRemovedGameKeepsCompletionDateInHistory(t *testing.T) {
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	id := addGames(t, s, 1)[0]
	s.now = func() time.Time { return day(7) }
	if _, err := s.SetStatus(id, StatusMastered); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveGame(id); err != nil {
		t.Fatal(err)
	}
	for _, g := range s.GetHistoryGames() {
		if g.ID == id {
			if g.CompletedAt == nil || !g.CompletedAt.Equal(day(7)) {
				t.Fatalf("archived CompletedAt = %v, want %v", g.CompletedAt, day(7))
			}
			return
		}
	}
	t.Fatal("removed game missing from history")
}

func syncGameOf(t *testing.T, s *Service, canonical string) SyncGame {
	t.Helper()
	for _, item := range s.SyncSnapshot() {
		if item.CanonicalGameID == canonical {
			return item
		}
	}
	t.Fatalf("no sync item for %s", canonical)
	return SyncGame{}
}

func TestSyncSnapshotCarriesCompletionDate(t *testing.T) {
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	g, err := s.AddCatalogGame("canon-1", "One", "")
	if err != nil {
		t.Fatal(err)
	}
	s.now = func() time.Time { return day(3) }
	if _, err := s.SetStatus(g.ID, StatusMastered); err != nil {
		t.Fatal(err)
	}
	item := syncGameOf(t, s, "canon-1")
	if item.Status != StatusMastered || item.CompletedAt == nil || !item.CompletedAt.Equal(day(3)) {
		t.Fatalf("snapshot = %+v", item)
	}
	*item.CompletedAt = day(20)
	stored, err := s.Find(g.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !stored.CompletedAt.Equal(day(3)) {
		t.Fatal("mutating the snapshot mutated the library")
	}
}

func TestApplySyncCarriesCompletionDateWithTheStatus(t *testing.T) {
	older, newer := day(2), day(8)
	d1, d2 := day(1), day(7)
	tests := []struct {
		name       string
		local      string
		localAt    time.Time
		localDate  *time.Time
		item       SyncGame
		wantStatus string
		wantDate   *time.Time
		wantAt     time.Time
	}{
		{"newer completed brings its date", StatusPlaying, older, nil, SyncGame{Status: StatusCompleted, StatusAt: &newer, CompletedAt: &d2}, StatusCompleted, &d2, newer},
		{"newer mastered brings its date", StatusCompleted, older, &d1, SyncGame{Status: StatusMastered, StatusAt: &newer, CompletedAt: &d2}, StatusMastered, &d2, newer},
		{"older item changes nothing", StatusMastered, newer, &d2, SyncGame{Status: StatusCompleted, StatusAt: &older, CompletedAt: &d1}, StatusMastered, &d2, newer},
		{"equal stamp changes nothing", StatusMastered, newer, &d2, SyncGame{Status: StatusCompleted, StatusAt: &newer}, StatusMastered, &d2, newer},
		{"newer without a date replaces the local date", StatusMastered, older, &d1, SyncGame{Status: StatusCompleted, StatusAt: &newer}, StatusCompleted, nil, newer},
		{"newer non-completed status clears the date", StatusMastered, older, &d1, SyncGame{Status: StatusPlaying, StatusAt: &newer, CompletedAt: &d2}, StatusPlaying, nil, newer},
		{"zero date is dropped", StatusPlaying, older, nil, SyncGame{Status: StatusCompleted, StatusAt: &newer, CompletedAt: func() *time.Time { d := time.Time{}; return &d }()}, StatusCompleted, nil, newer},
		{"date far in the future is dropped", StatusPlaying, older, nil, SyncGame{Status: StatusCompleted, StatusAt: &newer, CompletedAt: func() *time.Time { d := time.Now().AddDate(5, 0, 0); return &d }()}, StatusCompleted, nil, newer},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
			g, err := s.AddCatalogGame("canon-1", "One", "")
			if err != nil {
				t.Fatal(err)
			}
			s.mu.Lock()
			stored := s.findLocked(g.ID)
			stored.Status = tc.local
			la := tc.localAt
			stored.StatusAt = &la
			stored.CompletedAt = tc.localDate
			s.mu.Unlock()

			tc.item.CanonicalGameID = "canon-1"
			if err := s.ApplySync([]SyncGame{tc.item}); err != nil {
				t.Fatalf("ApplySync: %v", err)
			}
			got, err := s.Find(g.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got.Status != tc.wantStatus || got.StatusAt == nil || !got.StatusAt.Equal(tc.wantAt) {
				t.Fatalf("status = %q at %v, want %q at %v", got.Status, got.StatusAt, tc.wantStatus, tc.wantAt)
			}
			switch {
			case tc.wantDate == nil && got.CompletedAt != nil:
				t.Fatalf("CompletedAt = %v, want nil", got.CompletedAt)
			case tc.wantDate != nil && (got.CompletedAt == nil || !got.CompletedAt.Equal(*tc.wantDate)):
				t.Fatalf("CompletedAt = %v, want %v", got.CompletedAt, tc.wantDate)
			}
		})
	}
}

func TestApplySyncRollsBackTheCompletionDateWhenTheSaveFails(t *testing.T) {
	f := newSaveFixture(t)
	f.s.mu.Lock()
	stored := f.s.findLocked(f.a.ID)
	old := day(1)
	stored.Status = StatusPlaying
	stored.StatusAt = &old
	f.s.mu.Unlock()
	before := f.snapshot()
	f.breakSaving(t)

	newer, date := day(8), day(7)
	err := f.s.ApplySync([]SyncGame{{CanonicalGameID: "canon-a", Status: StatusMastered, StatusAt: &newer, CompletedAt: &date}})
	if err == nil {
		t.Fatal("the save failed but ApplySync reported success")
	}
	if after := f.snapshot(); !reflect.DeepEqual(before, after) {
		t.Fatalf("memory kept a synced status that never reached the disk\nbefore %+v\nafter  %+v", before, after)
	}
}

func TestCompletionDateJSONKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)
	id := addGames(t, s, 1)[0]
	s.now = func() time.Time { return day(4) }
	if _, err := s.SetStatus(id, StatusCompleted); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"completedAt": "2026-09-04T12:00:00Z"`) {
		t.Fatalf("completedAt missing from the stored game: %s", data)
	}
	if _, err := s.SetStatus(id, StatusPlaying); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"completedAt"`) {
		t.Fatalf("completedAt survived leaving the completed statuses: %s", data)
	}
}

func TestSetCompletedAtKeepsLaterTodayAsNow(t *testing.T) {
	now := day(10)
	path := filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)
	id := addGames(t, s, 1)[0]
	s.now = func() time.Time { return day(1) }
	if _, err := s.SetStatus(id, StatusCompleted); err != nil {
		t.Fatal(err)
	}
	s.now = func() time.Time { return now }

	if err := s.SetCompletedAt(id, now.Add(23*time.Hour)); err != nil {
		t.Fatalf("SetCompletedAt: %v", err)
	}
	got, err := s.Find(id)
	if err != nil {
		t.Fatal(err)
	}
	if got.CompletedAt == nil || !got.CompletedAt.Equal(now) {
		t.Fatalf("CompletedAt = %v, want %v: the server clips a date past its clock, and a copy it stored differently would never look synced", got.CompletedAt, now)
	}
}
