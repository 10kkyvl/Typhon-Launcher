package telemetrylog

import (
	"bytes"
	"fmt"
	"sync"
	"testing"
)

func TestLogRecordEvictsOldest(t *testing.T) {
	l := New(3)
	for i := 0; i < 5; i++ {
		l.Record(KindDiagnostics, fmt.Sprintf("/e/%d", i), []byte(fmt.Sprintf("payload-%d", i)))
	}

	entries := l.Entries()
	if len(entries) != 3 {
		t.Fatalf("got %d entries, want 3", len(entries))
	}

	want := []string{"/e/4", "/e/3", "/e/2"}
	for i, e := range entries {
		if e.Endpoint != want[i] {
			t.Fatalf("entry %d: got endpoint %q, want %q (full: %+v)", i, e.Endpoint, want[i], entries)
		}
	}
}

func TestLogEntriesPayloadRoundTrips(t *testing.T) {
	l := New(4)
	payload := []byte(`{"hello":"world"}`)
	l.Record(KindUsageStats, "/e", payload)

	entries := l.Entries()
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	if !bytes.Equal(entries[0].Payload, payload) {
		t.Fatalf("got payload %q, want %q", entries[0].Payload, payload)
	}
}

func TestLogRecordTruncatesLargePayload(t *testing.T) {
	l := New(1)
	big := bytes.Repeat([]byte("a"), maxPayloadBytes+1000)
	l.Record(KindDiagnostics, "/e", big)

	entries := l.Entries()
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	got := entries[0].Payload
	if len(got) != maxPayloadBytes+len(truncationMarker) {
		t.Fatalf("got payload length %d, want %d", len(got), maxPayloadBytes+len(truncationMarker))
	}
	if !bytes.HasSuffix(got, truncationMarker) {
		t.Fatalf("payload does not end with truncation marker: %q", got[len(got)-40:])
	}
	if !bytes.Equal(got[:maxPayloadBytes], big[:maxPayloadBytes]) {
		t.Fatal("truncated payload prefix does not match original")
	}
}

func TestLogRecordDoesNotAliasCallerSlice(t *testing.T) {
	l := New(2)
	payload := []byte("original")
	l.Record(KindDiagnostics, "/e", payload)

	for i := range payload {
		payload[i] = 'x'
	}

	entries := l.Entries()
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	if string(entries[0].Payload) != "original" {
		t.Fatalf("buffer aliased caller slice: got %q, want %q", entries[0].Payload, "original")
	}
}

func TestLogEntriesReturnsIndependentCopies(t *testing.T) {
	l := New(2)
	l.Record(KindDiagnostics, "/e", []byte("original"))

	entries := l.Entries()
	for i := range entries[0].Payload {
		entries[0].Payload[i] = 'x'
	}

	entries2 := l.Entries()
	if string(entries2[0].Payload) != "original" {
		t.Fatalf("mutating a returned entry corrupted the buffer: got %q", entries2[0].Payload)
	}
}

func TestLogNewNonPositiveCapacityFallsBack(t *testing.T) {
	tests := []struct {
		name     string
		capacity int
	}{
		{"zero", 0},
		{"negative", -5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := New(tt.capacity)
			for i := 0; i < defaultCapacity+10; i++ {
				l.Record(KindDiagnostics, "/e", []byte("x"))
			}
			if len(l.Entries()) != defaultCapacity {
				t.Fatalf("got %d entries, want %d", len(l.Entries()), defaultCapacity)
			}
		})
	}
}

func TestLogConcurrentRecordAndEntries(t *testing.T) {
	l := New(50)
	var wg sync.WaitGroup

	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				l.Record(KindDiagnostics, fmt.Sprintf("/w/%d", worker), []byte(fmt.Sprintf("payload-%d-%d", worker, i)))
			}
		}(w)
	}

	for r := 0; r < 4; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				entries := l.Entries()
				for _, e := range entries {
					_ = e.Payload
				}
			}
		}()
	}

	wg.Wait()

	entries := l.Entries()
	if len(entries) != 50 {
		t.Fatalf("got %d entries after concurrent access, want 50", len(entries))
	}
}

func TestPackageLevelRecordAndEntries(t *testing.T) {
	before := len(Entries())
	Record(KindUsageStats, "/pkg/e", []byte("pkg-payload"))
	after := Entries()

	if len(after) != before+1 {
		t.Fatalf("got %d entries after Record, want %d", len(after), before+1)
	}
	if after[0].Endpoint != "/pkg/e" || string(after[0].Payload) != "pkg-payload" {
		t.Fatalf("unexpected newest entry: %+v", after[0])
	}
}
