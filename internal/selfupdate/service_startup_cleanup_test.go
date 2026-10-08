package selfupdate

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// A cache entry that cannot be removed is a warning; a startup that was
// cancelled is not, and the sweep stopping on the cancelled context must not be
// mistaken for the first.
func TestServiceStartupReportsACancelledContext(t *testing.T) {
	dir := t.TempDir()
	store := mustStore(t, dir)
	if err := store.Save(stored{AvailableVersion: "2.0.0", CheckedAt: time.Now()}); err != nil {
		t.Fatalf("seed store: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	s := &Service{dir: dir, notes: mustNotesStore(t, dir), store: store, client: mustQuietClient(t), currentVersion: "1.0.0"}
	err := s.ServiceStartup(ctx, application.ServiceOptions{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ServiceStartup() error = %v, want context.Canceled", err)
	}
	if err := s.ServiceShutdown(); err != nil {
		t.Fatalf("ServiceShutdown: %v", err)
	}
}
