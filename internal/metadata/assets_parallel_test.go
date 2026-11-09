package metadata

import (
	"context"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"typhon/internal/catalog"
)

// slowImageServer answers every image after delay; a path containing "hang"
// waits for block instead. Each finished answer is announced on the returned
// channel, which is buffered for every request a test makes.
func slowImageServer(t *testing.T, delay time.Duration, block <-chan struct{}) (*httptest.Server, <-chan struct{}) {
	t.Helper()
	body := pngBytes(t, 640, 360)
	served := make(chan struct{}, 64)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if block != nil && strings.Contains(r.URL.Path, "hang") {
			select {
			case <-block:
			case <-r.Context().Done():
				return
			}
		}
		timer := time.NewTimer(delay)
		select {
		case <-timer.C:
		case <-r.Context().Done():
			timer.Stop()
			return
		}
		w.Header().Set("Content-Type", "image/png")
		if _, err := w.Write(body); err != nil {
			t.Errorf("write image: %v", err)
		}
		served <- struct{}{}
	}))
	t.Cleanup(srv.Close)
	return srv, served
}

// TestBuildAssetsDownloadsImagesTogether fails against one-by-one downloads: a
// cover and eight screenshots at 150 ms each then take 1.35 s before the game
// page receives anything.
func TestBuildAssetsDownloadsImagesTogether(t *testing.T) {
	const delay = 150 * time.Millisecond
	srv, _ := slowImageServer(t, delay, nil)
	svc, cat, _ := newTestService(t, &fakeProvider{})
	game := addGame(t, cat, catalog.Game{Title: "Prey"})

	meta := GameMetadata{Title: "Prey", Cover: &ImageRef{URL: srv.URL + "/cover.png"}}
	for i := range maxScreenshots + 2 {
		meta.Screenshots = append(meta.Screenshots, ImageRef{URL: fmt.Sprintf("%s/shot-%d.png", srv.URL, i)})
	}

	start := time.Now()
	batch, err := svc.buildAssets(context.Background(), game, meta, modeFull)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("buildAssets: %v", err)
	}
	if sequential := delay * time.Duration(1+maxScreenshots); elapsed >= sequential/2 {
		t.Fatalf("assets took %v; one by one would take %v", elapsed, sequential)
	}
	if batch.cover == "" || batch.assets[0].Type != AssetCover || batch.assets[0].SourceURL != meta.Cover.URL {
		t.Fatalf("cover missing or misplaced: %+v", batch.assets)
	}
	shots := batch.assets[1:]
	if len(shots) != maxScreenshots {
		t.Fatalf("%d screenshots, want %d", len(shots), maxScreenshots)
	}
	for i, shot := range shots {
		if shot.Type != AssetScreenshot || shot.SourceURL != meta.Screenshots[i].URL {
			t.Fatalf("screenshot %d = %s, want %s: order must follow the provider", i, shot.SourceURL, meta.Screenshots[i].URL)
		}
	}
	if len(batch.created) != 1+maxScreenshots {
		t.Fatalf("created %d assets, want %d", len(batch.created), 1+maxScreenshots)
	}
}

func TestBuildAssetsKeepsGoingWhenSomeImagesFail(t *testing.T) {
	ok, _ := slowImageServer(t, 0, nil)
	bad := imageServer(t)
	svc, cat, _ := newTestService(t, &fakeProvider{})
	game := addGame(t, cat, catalog.Game{Title: "Prey"})
	meta := GameMetadata{
		Title: "Prey",
		Cover: &ImageRef{URL: bad.URL + "/missing.png"},
		Screenshots: []ImageRef{
			{URL: ok.URL + "/a.png"}, {URL: bad.URL + "/broken.png"}, {URL: ""}, {URL: ok.URL + "/b.png"},
		},
	}
	batch, err := svc.buildAssets(context.Background(), game, meta, modeFull)
	if err != nil {
		t.Fatalf("buildAssets: %v", err)
	}
	if batch.cover != "" {
		t.Fatalf("a failed cover was kept: %+v", batch.assets)
	}
	if len(batch.assets) != 2 || batch.assets[0].SourceURL != ok.URL+"/a.png" || batch.assets[1].SourceURL != ok.URL+"/b.png" {
		t.Fatalf("assets = %+v, want a.png then b.png", batch.assets)
	}
}

// A cancelled refresh must not leave the images that finished before the
// cancel on disk: nothing records them, so nothing would remove them later.
func TestBuildAssetsCancelledRemovesFinishedDownloads(t *testing.T) {
	block := make(chan struct{})
	srv, served := slowImageServer(t, 0, block)
	t.Cleanup(func() { close(block) })
	svc, cat, _ := newTestService(t, &fakeProvider{})
	game := addGame(t, cat, catalog.Game{Title: "Prey"})
	meta := GameMetadata{
		Title: "Prey",
		Cover: &ImageRef{URL: srv.URL + "/cover.png"},
		Screenshots: []ImageRef{
			{URL: srv.URL + "/a.png"}, {URL: srv.URL + "/hang.png"}, {URL: srv.URL + "/b.png"},
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		for range 3 {
			<-served
		}
		cancel()
	}()
	if _, err := svc.buildAssets(ctx, game, meta, modeFull); err == nil {
		t.Fatal("a cancelled build succeeded")
	}
	var left []string
	if err := filepath.WalkDir(svc.store.mediaRoot(), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			left = append(left, path)
		}
		return nil
	}); err != nil && !strings.Contains(err.Error(), "no such file") {
		t.Fatal(err)
	}
	if len(left) != 0 {
		t.Fatalf("files left after cancel: %v", left)
	}
}
