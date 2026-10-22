package sources

import (
	"context"
	"log/slog"
	"time"
)

const (
	eventProgress = "sources:progress"

	stageFetch = "fetch"
	stageParse = "parse"
	stageMatch = "match"
	stageApply = "apply"
	stageSave  = "save"

	progressStep   = 500
	interruptEvery = 256
)

// refreshProgress is the sources:progress event payload. It stays unexported
// like degradedStatus: the wails bindings must not change for an event shape.
type refreshProgress struct {
	SourceID string `json:"sourceId"`
	Stage    string `json:"stage"`
	Done     int    `json:"done"`
	Total    int    `json:"total"`
}

type stageTime struct {
	name string
	took time.Duration
}

// refreshRun is the state of one source refresh that is shared by its stages.
// It belongs to the refresh goroutine and is not safe for concurrent use,
// except report, which the release matcher calls from its batch worker while
// that goroutine waits for it.
type refreshRun struct {
	id     string
	memo   *titleMemo
	emit   func(name string, data any)
	stages []stageTime
	last   map[string]int
}

func (s *Service) newRun(id string) *refreshRun {
	return &refreshRun{id: id, memo: s.newMemo(), emit: s.events, last: map[string]int{}}
}

func (r *refreshRun) progress(stage string, done, total int) {
	r.last[stage] = done
	r.emit(eventProgress, refreshProgress{SourceID: r.id, Stage: stage, Done: done, Total: total})
}

// finish closes a stage whose last batch report may not have reached total.
func (r *refreshRun) finish(stage string, total int) {
	if r.last[stage] != total {
		r.progress(stage, total, total)
	}
}

func (r *refreshRun) begin(stage string) func() {
	started := time.Now()
	return func() { r.stages = append(r.stages, stageTime{stage, time.Since(started)}) }
}

func (r *refreshRun) record(stage string, took time.Duration) {
	r.stages = append(r.stages, stageTime{stage, took})
}

func (r *refreshRun) log() {
	attrs := []any{"source_id", r.id}
	for _, st := range r.stages {
		attrs = append(attrs, st.name+"_ms", st.took.Milliseconds())
	}
	slog.Info("source refresh stages", attrs...)
}

func interrupted(ctx context.Context, i int) error {
	if i%interruptEvery == 0 {
		return ctx.Err()
	}
	return nil
}
