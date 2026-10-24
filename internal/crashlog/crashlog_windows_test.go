//go:build windows

package crashlog

import (
	"testing"
	"time"
)

func TestReadEmptyWindowQueriesJournal(t *testing.T) {
	at := time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)
	faults, err := read(t.Context(), Query{Exe: "none.exe", From: at, To: at.Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	if len(faults) != 0 {
		t.Fatalf("unexpected faults in an empty window: %+v", faults)
	}
}

func TestReadRecentWindowParses(t *testing.T) {
	now := time.Now()
	if _, err := read(t.Context(), Query{Exe: "none.exe", From: now.Add(-30 * 24 * time.Hour), To: now}); err != nil {
		t.Fatal(err)
	}
}
