package history

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func mustService(t *testing.T) *Service {
	t.Helper()
	s, err := NewServiceAt(filepath.Join(t.TempDir(), "history.json"))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	return s
}

func TestRecordRollsBackOnPersistFailure(t *testing.T) {
	root := t.TempDir()
	blocker := filepath.Join(root, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := mustService(t)
	if err := s.Record(Record{Kind: KindInstalled, Title: "First"}); err != nil {
		t.Fatalf("seed record: %v", err)
	}
	before := s.List(Filter{})

	s.mu.Lock()
	s.path = filepath.Join(blocker, "history.json")
	s.mu.Unlock()

	if err := s.Record(Record{Kind: KindInstalled, Title: "Second"}); err == nil {
		t.Fatal("persist failure must be returned to the caller")
	}
	after := s.List(Filter{})
	if len(after) != len(before) || after[0].Title != before[0].Title {
		t.Fatalf("records = %+v, want rollback to %+v", after, before)
	}
	if st := s.StatusOf(); !st.Degraded || st.Message == "" {
		t.Fatalf("status = %+v, want degraded with a message", st)
	}
}

func TestRecordRejectsBadInput(t *testing.T) {
	cases := []struct {
		name string
		rec  Record
		want error
	}{
		{"bad kind", Record{Kind: Kind("bogus"), Title: "Game"}, ErrBadKind},
		{"empty title", Record{Kind: KindInstalled, Title: "   "}, ErrEmptyTitle},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := mustService(t)
			if err := s.Record(tc.rec); !errors.Is(err, tc.want) {
				t.Fatalf("Record() error = %v, want %v", err, tc.want)
			}
			if got := s.List(Filter{}); len(got) != 0 {
				t.Fatalf("records = %+v, want none", got)
			}
		})
	}
}

func TestRetentionByCount(t *testing.T) {
	s := mustService(t)
	base := time.Now().Add(-time.Hour)
	total := maxRecords + 10
	for i := 0; i < total; i++ {
		rec := Record{
			Kind:  KindInstalled,
			Title: fmt.Sprintf("Game %d", i),
			At:    base.Add(time.Duration(i) * time.Second),
		}
		if err := s.Record(rec); err != nil {
			t.Fatalf("record %d: %v", i, err)
		}
	}
	got := s.List(Filter{})
	if len(got) != maxRecords {
		t.Fatalf("len = %d, want %d", len(got), maxRecords)
	}
	if want := fmt.Sprintf("Game %d", total-1); got[0].Title != want {
		t.Fatalf("newest = %q, want %q", got[0].Title, want)
	}
	if want := fmt.Sprintf("Game %d", total-maxRecords); got[len(got)-1].Title != want {
		t.Fatalf("oldest kept = %q, want %q", got[len(got)-1].Title, want)
	}
}

func TestRetentionByAge(t *testing.T) {
	s := mustService(t)
	old := Record{Kind: KindInstalled, Title: "Old", At: time.Now().Add(-(maxAge + time.Hour))}
	if err := s.Record(old); err != nil {
		t.Fatalf("seed old record: %v", err)
	}
	if err := s.Record(Record{Kind: KindInstalled, Title: "Fresh"}); err != nil {
		t.Fatalf("record fresh: %v", err)
	}
	got := s.List(Filter{})
	if len(got) != 1 || got[0].Title != "Fresh" {
		t.Fatalf("records = %+v, want only Fresh", got)
	}
}

func TestBytesKnownSurvivesRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	s, err := NewServiceAt(path)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	if err := s.Record(Record{Kind: KindRemoved, Title: "Known Zero", Bytes: 0, BytesKnown: true}); err != nil {
		t.Fatalf("record known: %v", err)
	}
	if err := s.Record(Record{Kind: KindRemoved, Title: "Unknown", Bytes: 0, BytesKnown: false}); err != nil {
		t.Fatalf("record unknown: %v", err)
	}

	reloaded, err := NewServiceAt(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	byTitle := map[string]Record{}
	for _, r := range reloaded.List(Filter{}) {
		byTitle[r.Title] = r
	}
	if !byTitle["Known Zero"].BytesKnown {
		t.Fatalf("known-zero record lost BytesKnown: %+v", byTitle["Known Zero"])
	}
	if byTitle["Unknown"].BytesKnown {
		t.Fatalf("unknown record gained BytesKnown: %+v", byTitle["Unknown"])
	}
}

func TestListFilters(t *testing.T) {
	s := mustService(t)
	records := []Record{
		{Kind: KindInstalled, Title: "Cyberpunk 2077"},
		{Kind: KindUpdated, Title: "Doom Eternal", Detail: "1.2 → 1.3"},
		{Kind: KindRemoved, Title: "Hades", Detail: "освобождено 14 ГБ"},
	}
	for _, r := range records {
		if err := s.Record(r); err != nil {
			t.Fatalf("record %q: %v", r.Title, err)
		}
	}

	cases := []struct {
		name string
		f    Filter
		want []string
	}{
		{"by kind", Filter{Kinds: []Kind{KindUpdated}}, []string{"Doom Eternal"}},
		{"by query title", Filter{Query: "cyber"}, []string{"Cyberpunk 2077"}},
		{"by query cyrillic case", Filter{Query: "ОСВОБОЖДЕНО"}, []string{"Hades"}},
		{"kind and query combined", Filter{Kinds: []Kind{KindRemoved, KindUpdated}, Query: "eternal"}, []string{"Doom Eternal"}},
		{"limit", Filter{Limit: 1}, []string{"Hades"}},
		{"no match", Filter{Query: "no such game"}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := s.List(tc.f)
			if len(got) != len(tc.want) {
				t.Fatalf("List() = %+v, want titles %v", got, tc.want)
			}
			for i, r := range got {
				if r.Title != tc.want[i] {
					t.Fatalf("List()[%d] = %q, want %q", i, r.Title, tc.want[i])
				}
			}
		})
	}
}

func TestConcurrentRecordAndList(t *testing.T) {
	s := mustService(t)
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs <- s.Record(Record{Kind: KindInstalled, Title: fmt.Sprintf("Concurrent %d", i)})
		}(i)
	}
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.List(Filter{})
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("record: %v", err)
		}
	}
}
