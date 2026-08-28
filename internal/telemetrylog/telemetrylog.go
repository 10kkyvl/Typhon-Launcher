// Package telemetrylog keeps an in-memory ring buffer of the telemetry
// payloads this process has sent, so the UI can show what actually went
// over the wire without depending on the backend to echo it back.
package telemetrylog

import (
	"sync"
	"time"
)

// Kind identifies which telemetry client produced an entry.
type Kind string

const (
	KindDiagnostics Kind = "diagnostics"
	KindUsageStats  Kind = "usagestats"
)

const (
	defaultCapacity = 100
	// maxPayloadBytes bounds a single stored payload. Diagnostics reports
	// carry full stack traces; up to defaultCapacity of them unbounded
	// would let the resident buffer grow into the hundreds of megabytes.
	maxPayloadBytes = 65536
)

// truncationMarker is appended to a payload cut at maxPayloadBytes so a
// reader can tell a clipped body from one that legitimately ends there.
var truncationMarker = []byte("...[truncated]")

// Entry is one recorded send attempt. Payload is the raw, un-formatted body
// as it was handed to Record, possibly clipped to maxPayloadBytes.
type Entry struct {
	Kind     Kind
	Endpoint string
	SentAt   time.Time
	Payload  []byte
}

// Log is a fixed-capacity ring buffer of Entry values. The zero value is
// not usable; construct with New.
type Log struct {
	mu       sync.Mutex
	capacity int
	entries  []Entry
	next     int
	size     int
}

// New creates a Log holding at most capacity entries. A non-positive
// capacity falls back to defaultCapacity.
func New(capacity int) *Log {
	if capacity <= 0 {
		capacity = defaultCapacity
	}
	return &Log{
		capacity: capacity,
		entries:  make([]Entry, capacity),
	}
}

// Record appends an entry, evicting the oldest one once the buffer is
// full. payload is copied; the caller's slice may be reused or mutated
// after Record returns.
func (l *Log) Record(kind Kind, endpoint string, payload []byte) {
	entry := Entry{
		Kind:     kind,
		Endpoint: endpoint,
		SentAt:   time.Now(),
		Payload:  clipPayload(payload),
	}

	l.mu.Lock()
	l.entries[l.next] = entry
	l.next = (l.next + 1) % l.capacity
	if l.size < l.capacity {
		l.size++
	}
	l.mu.Unlock()
}

func clipPayload(payload []byte) []byte {
	if len(payload) <= maxPayloadBytes {
		out := make([]byte, len(payload))
		copy(out, payload)
		return out
	}
	out := make([]byte, maxPayloadBytes+len(truncationMarker))
	copy(out, payload[:maxPayloadBytes])
	copy(out[maxPayloadBytes:], truncationMarker)
	return out
}

// Entries returns a snapshot of the buffered entries, newest first. Each
// Entry (including its Payload slice) is a copy, so the caller cannot
// mutate the buffer through the result.
func (l *Log) Entries() []Entry {
	l.mu.Lock()
	defer l.mu.Unlock()

	out := make([]Entry, l.size)
	for i := 0; i < l.size; i++ {
		idx := (l.next - 1 - i + l.capacity) % l.capacity
		src := l.entries[idx]
		payload := make([]byte, len(src.Payload))
		copy(payload, src.Payload)
		out[i] = Entry{
			Kind:     src.Kind,
			Endpoint: src.Endpoint,
			SentAt:   src.SentAt,
			Payload:  payload,
		}
	}
	return out
}

// std is the process-wide log of what this launcher instance has sent.
// There is exactly one such history per process, so a package-level
// singleton avoids threading an *Log through every telemetry client
// constructor for no behavioral benefit.
var std = New(defaultCapacity)

// Record appends to the process-wide log. See (*Log).Record.
func Record(kind Kind, endpoint string, payload []byte) {
	std.Record(kind, endpoint, payload)
}

// Entries returns a snapshot of the process-wide log. See (*Log).Entries.
func Entries() []Entry {
	return std.Entries()
}
