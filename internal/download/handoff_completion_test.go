package download

import (
	"context"
	"errors"
	"testing"
	"time"

	"typhon/internal/handoffgate"
	"typhon/internal/settings"

	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/storage"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// The successor opens the record of finished pieces in its constructor,
// while the old launcher still downloads. What the old one flushes on its way
// out must reach the client the successor builds when its gate opens;
// otherwise the successor's next flush writes its stale copy over it and the
// finished pieces are fetched again.
func TestSuccessorClientSeesPiecesTheOldLauncherFinished(t *testing.T) {
	dir := t.TempDir()
	pk := metainfo.PieceKey{InfoHash: metainfo.Hash{7, 7, 7}, Index: 3}

	old := mustManagerAt(t, dir)
	succ, err := newManagerAt(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	closePieceCompletionOnCleanup(t, succ)

	if err := old.pieceCompletion.Set(pk, true); err != nil {
		t.Fatal(err)
	}
	if err := old.closePieceCompletion(); err != nil {
		t.Fatal(err)
	}

	built := make(chan storage.PieceCompletion, 8)
	succ.buildClient = func(_ context.Context, _ settings.Settings, _ string, completion storage.PieceCompletion, _ netPlan) (*client, error) {
		built <- completion
		return nil, errors.New("no torrent client in this test")
	}
	handoffgate.Arm()
	t.Cleanup(handoffgate.Open)
	if err := succ.ServiceStartup(t.Context(), application.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	handoffgate.Open()
	if err := handoffgate.Settled(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := succ.ServiceShutdown(); err != nil {
			t.Errorf("ServiceShutdown: %v", err)
		}
	})

	var completion storage.PieceCompletion
	select {
	case completion = <-built:
	case <-time.After(5 * time.Second):
		t.Fatal("the successor never built its torrent client")
	}
	got, err := completion.Get(pk)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Ok || !got.Complete {
		t.Fatalf("piece the old launcher finished reads %+v in the successor, want complete", got)
	}
}
