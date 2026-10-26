package theme

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"typhon/internal/uierr"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type SourceFailure struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type sourceWatch struct {
	mu   sync.Mutex
	seen map[string]sourceSeen
}

type sourceSeen struct {
	source string
	state  string
	// retry marks a state whose reload failed for a reason outside the file's
	// content (themes.json held by another process, the file locked by the
	// editor still writing it); it is retried silently on the next polls.
	retry bool
}

type sourceRef struct {
	id        string
	name      string
	source    string
	updatedAt time.Time
}

func (s *Service) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ctx != nil {
		return errors.New("theme: already started")
	}
	own, cancel := context.WithCancel(ctx)
	s.ctx, s.cancel = own, cancel
	s.wg.Add(1)
	go s.watchSources(own, s.pollEvery)
	return nil
}

func (s *Service) ServiceShutdown() error {
	s.mu.Lock()
	cancel := s.cancel
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	s.wg.Wait()
	return nil
}

func (s *Service) watchSources(ctx context.Context, every time.Duration) {
	defer s.wg.Done()
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.pollOnce(ctx)
		}
	}
}

func (s *Service) sourceRefs() []sourceRef {
	s.mu.Lock()
	defer s.mu.Unlock()
	var refs []sourceRef
	for _, t := range s.themes {
		if t.Source != "" {
			refs = append(refs, sourceRef{id: t.ID, name: t.Name, source: t.Source, updatedAt: t.UpdatedAt})
		}
	}
	return refs
}

func (s *Service) pollOnce(ctx context.Context) {
	s.watch.mu.Lock()
	defer s.watch.mu.Unlock()
	refs := s.sourceRefs()
	live := make(map[string]bool, len(refs))
	for _, ref := range refs {
		if ctx.Err() != nil {
			return
		}
		live[ref.id] = true
		s.pollSource(ref)
	}
	for id := range s.watch.seen {
		if !live[id] {
			delete(s.watch.seen, id)
		}
	}
}

func fileState(info os.FileInfo, err error) string {
	if err != nil {
		return "error:" + err.Error()
	}
	return fmt.Sprintf("%d:%d", info.Size(), info.ModTime().UnixNano())
}

// pollSource reloads a watched file once per distinct (size, modtime) state
// and reports a failure once per state. A content error waits for the file to
// change; any other failure is retried quietly until it clears.
func (s *Service) pollSource(ref sourceRef) {
	info, statErr := os.Stat(ref.source)
	state := fileState(info, statErr)
	prev, known := s.watch.seen[ref.id]
	known = known && prev.source == ref.source
	if known && prev.state == state && !prev.retry {
		return
	}
	retrying := known && prev.state == state && prev.retry
	s.watch.seen[ref.id] = sourceSeen{source: ref.source, state: state}
	if statErr != nil {
		// A file already gone when the launcher starts is not news: reporting it
		// would raise the same toast on every start for a file the user deleted.
		if !known {
			slog.Info("theme source is missing", "id", ref.id, "source", ref.source, "error", statErr)
			return
		}
		s.reportSource(ref, fmt.Errorf("файл темы %s: %w", ref.source, statErr))
		return
	}
	if !known && !info.ModTime().After(ref.updatedAt) {
		return
	}
	err := s.reloadSource(ref)
	if err == nil {
		return
	}
	if uierr.Code(err) == "" {
		s.watch.seen[ref.id] = sourceSeen{source: ref.source, state: state, retry: true}
	}
	if !retrying {
		s.reportSource(ref, err)
	}
}

func (s *Service) reloadSource(ref sourceRef) error {
	data, err := readThemeFile(ref.source)
	if err != nil {
		return err
	}
	t, err := decodeTheme(data, "")
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.findLocked(ref.id)
	if !ok || current.Source != ref.source {
		return nil
	}
	t.ID = ref.id
	t.Source = ref.source
	if _, err := s.upsertLocked(t); err != nil {
		return err
	}
	return nil
}

func (s *Service) reportSource(ref sourceRef, err error) {
	slog.Warn("theme source reload failed", "id", ref.id, "source", ref.source, "error", err)
	s.events("theme:source-failed", SourceFailure{
		ID:      ref.id,
		Name:    ref.name,
		Code:    uierr.Code(err),
		Message: err.Error(),
	})
}
