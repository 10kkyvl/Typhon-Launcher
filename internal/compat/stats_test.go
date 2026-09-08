package compat

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newTestStats(t *testing.T) *Stats {
	t.Helper()
	return NewStatsAt(filepath.Join(t.TempDir(), "compat-stats.json"))
}

func TestUnknownGameHasNoSharedNumber(t *testing.T) {
	s := newTestStats(t)
	if _, ok := s.Game("232567"); ok {
		t.Fatal("нашлась строка для игры, про которую никто не отчитывался")
	}
}

func TestSnapshotSeparatesRepackersFromTheWholeGame(t *testing.T) {
	s := newTestStats(t)
	s.Apply(Snapshot{Games: []Shared{
		{GameID: "232567", Repacker: AnyRepacker, Works: 9, Total: 10},
		{GameID: "232567", Repacker: "fitgirl", Works: 8, Total: 8},
		{GameID: "232567", Repacker: "dodi", Works: 1, Total: 2},
	}})

	whole, ok := s.Game("232567")
	if !ok || whole.Works != 9 || whole.Total != 10 {
		t.Fatalf("игра целиком = %+v, ok = %v", whole, ok)
	}
	dodi, ok := s.Lookup("232567", "dodi")
	if !ok || dodi.Ratio() != 0.5 {
		t.Fatalf("dodi = %+v, ratio = %v", dodi, dodi.Ratio())
	}
}

// Строка без наблюдений — не «ноль процентов», а отсутствие ответа. Пустить её
// в кэш значит показать в каталоге долю, посчитанную ни по чему.
func TestRowsWithoutObservationsAreDropped(t *testing.T) {
	s := newTestStats(t)
	s.Apply(Snapshot{Games: []Shared{
		{GameID: "232567", Repacker: AnyRepacker, Works: 0, Total: 0},
		{GameID: "", Repacker: AnyRepacker, Works: 3, Total: 3},
	}})
	if s.Len() != 0 {
		t.Fatalf("в кэше %d строк, want 0", s.Len())
	}
}

func TestSnapshotSurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "compat-stats.json")
	first := NewStatsAt(path)
	first.Apply(Snapshot{
		ETag:  `W/"abc"`,
		Games: []Shared{{GameID: "232567", Repacker: AnyRepacker, Works: 9, Total: 10}},
	})

	second := NewStatsAt(path)
	got, ok := second.Game("232567")
	if !ok || got.Total != 10 {
		t.Fatalf("после перезапуска = %+v, ok = %v", got, ok)
	}
	if second.ETag() != `W/"abc"` {
		t.Fatalf("ETag = %q, снимок будет качаться заново каждый старт", second.ETag())
	}
}

// Битый файл не должен мешать лаунчеру: без агрегата каталог просто молчит.
func TestCorruptSnapshotStartsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "compat-stats.json")
	if err := os.WriteFile(path, []byte("{ не json"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	s := NewStatsAt(path)
	if s.Len() != 0 {
		t.Fatalf("в кэше %d строк, want 0", s.Len())
	}
}

// Новый снимок заменяет прошлый целиком, а не доливается в него: игра, по
// которой отчёты перестали приходить, обязана исчезнуть, а не остаться навсегда.
func TestSnapshotReplacesRatherThanMerges(t *testing.T) {
	s := newTestStats(t)
	s.Apply(Snapshot{Games: []Shared{{GameID: "1", Repacker: AnyRepacker, Works: 1, Total: 5}}})
	s.Apply(Snapshot{Games: []Shared{{GameID: "2", Repacker: AnyRepacker, Works: 5, Total: 5}}})

	if _, ok := s.Game("1"); ok {
		t.Fatal("старая строка пережила новый снимок")
	}
	if _, ok := s.Game("2"); !ok {
		t.Fatal("новая строка не применилась")
	}
}

func TestPersistedSnapshotIsReadableJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "compat-stats.json")
	NewStatsAt(path).Apply(Snapshot{
		UpdatedAt: time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC),
		Games:     []Shared{{GameID: "1", Repacker: AnyRepacker, Works: 5, Total: 5}},
	})
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var back Snapshot
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("снимок на диске не читается: %v\n%s", err, raw)
	}
	if len(back.Games) != 1 {
		t.Fatalf("игр в снимке %d, want 1", len(back.Games))
	}
}
