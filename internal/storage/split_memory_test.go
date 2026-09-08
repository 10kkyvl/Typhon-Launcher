package storage

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"testing"
)

// splitByCopyingEnvelope is a fixed reference reproducing split()'s
// implementation from before this optimization: it decodes the whole
// document into doc{Data json.RawMessage}, and json.RawMessage's
// UnmarshalJSON always copies its input, so the "data" field's bytes end up
// allocated twice: once inside the file's raw bytes, once in the fresh
// envelope.Data buffer. It exists only as a stable baseline for
// TestSplitDoesNotCopyEnvelopeData below, independent of whatever split()
// becomes.
func splitByCopyingEnvelope(trimmed []byte) (json.RawMessage, int, error) {
	var envelope doc
	if err := json.Unmarshal(trimmed, &envelope); err != nil {
		return nil, 0, err
	}
	if envelope.Version == 0 || envelope.Data == nil {
		return json.RawMessage(trimmed), 0, nil
	}
	return envelope.Data, envelope.Version, nil
}

// TestSplitDoesNotCopyEnvelopeData guards against split() allocating a
// second full-size copy of the "data" envelope field while the file's raw
// bytes are still resident. The audit measured os.ReadFile+json.Unmarshal
// alone reaching ~198 MB HeapAlloc for 100k releases; part of that is this
// exact pattern, independent of GC timing: decoding into
// doc{Data json.RawMessage} always copies the "data" value, so it is held
// twice (once inside raw, once in the copy) before json.Unmarshal(payload,
// out) even starts building the result tree.
//
// The test measures heap growth (GC disabled during measurement, the same
// technique already used in internal/sources/load_memory_test.go) for the
// real split() against a reference reproducing the old copying behavior, on
// an identical, large, already-versioned fixture. A non-copying split must
// grow the heap by much less than the copying reference; testing.AllocsPerRun
// would not show this because both approaches make a similar number of
// allocations — only the byte total differs.
func TestSplitDoesNotCopyEnvelopeData(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "items.json")
	const n = 200000
	items := make([]item, 0, n)
	for i := 0; i < n; i++ {
		items = append(items, item{
			ID:    fmt.Sprintf("id-%d", i),
			Title: fmt.Sprintf("Заголовок предмета номер %d, чуть длиннее ради размера файла", i),
		})
	}
	if err := Save(path, 1, items); err != nil {
		t.Fatalf("save fixture: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if len(raw) < 5<<20 {
		t.Fatalf("fixture too small to measure reliably: %d bytes", len(raw))
	}

	restore := debug.SetGCPercent(-1)
	defer debug.SetGCPercent(restore)

	measure := func(f func()) uint64 {
		runtime.GC()
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		f()
		runtime.ReadMemStats(&after)
		return after.HeapAlloc - before.HeapAlloc
	}

	var refPayload json.RawMessage
	refGrowth := measure(func() {
		payload, version, err := splitByCopyingEnvelope(raw)
		if err != nil {
			t.Fatalf("reference split: %v", err)
		}
		if version != 1 {
			t.Fatalf("reference split version = %d, want 1", version)
		}
		refPayload = payload
	})

	var realPayload json.RawMessage
	realGrowth := measure(func() {
		payload, version, err := split(raw)
		if err != nil {
			t.Fatalf("split: %v", err)
		}
		if version != 1 {
			t.Fatalf("split version = %d, want 1", version)
		}
		realPayload = payload
	})

	if !bytes.Equal(refPayload, realPayload) {
		t.Fatalf("payload mismatch between reference (%d bytes) and real (%d bytes) split", len(refPayload), len(realPayload))
	}

	t.Logf("reference (copying) split grew the heap by %d bytes; split() grew it by %d bytes (%.1f%%) for a %d byte file",
		refGrowth, realGrowth, 100*float64(realGrowth)/float64(refGrowth), len(raw))

	if float64(realGrowth) > float64(refGrowth)*0.5 {
		t.Fatalf("split() heap growth (%d bytes) is too close to the reference copying implementation (%d bytes) "+
			"for a %d byte file: it must return the data payload without copying it again", realGrowth, refGrowth, len(raw))
	}
}
