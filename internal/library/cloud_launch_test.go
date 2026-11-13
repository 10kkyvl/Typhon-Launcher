package library_test

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"typhon/internal/account"
	"typhon/internal/cloudsaves"
	"typhon/internal/library"
	"typhon/internal/savebackup"
	"typhon/internal/settings"
	"typhon/internal/uierr"
)

// cloudCopy is one archive the fake cloud holds.
type cloudCopy struct {
	save account.Save
	zip  []byte
}

type cloud struct {
	mu        sync.Mutex
	copies    map[string]cloudCopy
	lists     int
	downloads int
}

func (c *cloud) ListSaves(context.Context) (account.SaveList, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lists++
	list := account.SaveList{QuotaBytes: 1 << 30, MaxBytes: 100 << 20}
	for _, cp := range c.copies {
		list.Games = append(list.Games, cp.save)
		list.UsedBytes += cp.save.SizeBytes
	}
	return list, nil
}

func (c *cloud) UploadSave(_ context.Context, key string, body io.ReadSeeker, size int64, up account.SaveUpload) (account.Save, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return account.Save{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	saved := account.Save{GameID: key, SizeBytes: size, Digest: up.Digest, DeviceID: up.DeviceID, UpdatedAt: time.Now().UTC()}
	c.copies[key] = cloudCopy{save: saved, zip: data}
	return saved, nil
}

func (c *cloud) DownloadSave(_ context.Context, key string, dst io.Writer, _ int64) (account.SaveDownload, error) {
	c.mu.Lock()
	cp, ok := c.copies[key]
	c.downloads++
	c.mu.Unlock()
	if !ok {
		return account.SaveDownload{}, &account.Error{Code: account.CodeSaveNotFound, Status: 404}
	}
	if _, err := dst.Write(cp.zip); err != nil {
		return account.SaveDownload{}, err
	}
	return account.SaveDownload{Digest: cp.save.Digest, Size: int64(len(cp.zip))}, nil
}

func (c *cloud) DeleteSave(context.Context, string) error {
	return errors.New("fake cloud: delete is not expected here")
}

func (c *cloud) calls() (lists, downloads int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lists, c.downloads
}

func (c *cloud) put(t *testing.T, key, device string, files map[string]string) {
	t.Helper()
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	slices.Sort(names)
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	digest := sha256.New()
	for _, n := range names {
		w, err := zw.Create(n)
		if err != nil {
			t.Fatalf("zip %s: %v", n, err)
		}
		if _, err := w.Write([]byte(files[n])); err != nil {
			t.Fatalf("zip %s: %v", n, err)
		}
		sum := sha256.Sum256([]byte(files[n]))
		digest.Write([]byte(n + "\n" + hex.EncodeToString(sum[:]) + "\n"))
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.copies[key] = cloudCopy{
		save: account.Save{GameID: key, SizeBytes: int64(buf.Len()), Digest: hex.EncodeToString(digest.Sum(nil)), DeviceID: device, UpdatedAt: time.Now().UTC()},
		zip:  buf.Bytes(),
	}
}

func writeSaves(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func readSaves(dir string) (map[string]string, error) {
	var rels []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		rels = append(rels, rel)
		return nil
	})
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, rel := range rels {
		data, err := os.ReadFile(filepath.Join(dir, rel))
		if err != nil {
			return nil, err
		}
		out[filepath.ToSlash(rel)] = string(data)
	}
	return out, nil
}

func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(2 * time.Millisecond)
	defer tick.Stop()
	for !cond() {
		select {
		case <-tick.C:
		case <-deadline.C:
			t.Fatalf("timed out waiting for %s", what)
		}
	}
}

// Scenario case 2 with the real services: the saves here are unchanged since
// the last sync and another device put a newer copy in the cloud. Play puts
// the cloud copy in place before the process starts, through the library's
// real launch, savebackup's real restore and the cloud saves hook.
func TestPlayStartsTheGameOnTheNewerCloudCopy(t *testing.T) {
	ctx := t.Context()
	lib, err := library.NewServiceAt(filepath.Join(t.TempDir(), "library.json"))
	if err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(t.TempDir(), "game.exe")
	if err := os.WriteFile(exe, []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}
	game, err := lib.AddGame(ctx, exe, "Game")
	if err != nil {
		t.Fatal(err)
	}
	library.SetCanonicalID(lib, game.ID, "cloud-1")
	saves := filepath.Join(t.TempDir(), "Saves")
	here := map[string]string{"slot1.sav": "one", "profile/slot2.sav": "two"}
	newer := map[string]string{"slot1.sav": "from the other device", "profile/slot2.sav": "two"}
	writeSaves(t, saves, here)
	if game, err = lib.SetSavesDir(game.ID, saves); err != nil {
		t.Fatal(err)
	}

	cfg := settings.Defaults()
	cfg.CloudSaves = true
	config := func() settings.Settings { return cfg }
	backups, err := savebackup.NewServiceAt(t.TempDir(), config, lib)
	if err != nil {
		t.Fatal(err)
	}
	if err := backups.ServiceStartup(ctx, application.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := backups.ServiceShutdown(); err != nil {
			t.Errorf("savebackup shutdown: %v", err)
		}
	})
	fake := &cloud{copies: map[string]cloudCopy{}}
	owner := func(string) (string, error) { return "account-1", nil }
	stored := func(string) string { return "account-1" }
	cloudSaves, err := cloudsaves.NewService(t.TempDir(), config, func() (string, error) { return "token", nil }, owner, stored, fake, lib, backups)
	if err != nil {
		t.Fatal(err)
	}
	if err := cloudSaves.ServiceStartup(ctx, application.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := cloudSaves.ServiceShutdown(); err != nil {
			t.Errorf("cloud saves shutdown: %v", err)
		}
	})

	// The first sync uploads the saves here, so they count as unchanged since
	// it. The startup pass and SyncNow both list; after both, neither will
	// see the newer copy put in next.
	if err := cloudSaves.SyncNow(game.ID); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	waitUntil(t, "the startup pass and the first sync", func() bool {
		lists, _ := fake.calls()
		return lists >= 2
	})
	fake.put(t, "cloud-1", "other-device", newer)
	_, downloadsBefore := fake.calls()

	var atStart []map[string]string
	library.StubLaunch(lib, ctx, func() error {
		files, err := readSaves(game.SavesDir)
		if err != nil {
			return err
		}
		atStart = append(atStart, files)
		return errors.New("stop before a real process")
	})
	lib.AddLaunchHook(cloudSaves)
	lib.AddLaunchHook(backups)

	if err := lib.PlayGame(game.ID); uierr.Code(err) != "library.launch_failed" {
		t.Fatalf("PlayGame = %v, want the stubbed start failure", err)
	}

	if len(atStart) != 1 {
		t.Fatalf("the process was started %d times, want once", len(atStart))
	}
	for name, want := range newer {
		if atStart[0][name] != want {
			t.Fatalf("saves when the process started = %v, want the cloud copy %v", atStart[0], newer)
		}
	}
	if _, downloads := fake.calls(); downloads-downloadsBefore != 1 {
		t.Fatalf("%d downloads, want 1", downloads-downloadsBefore)
	}
	for _, g := range cloudSaves.Status().Games {
		if g.GameID == game.ID && (g.State != cloudsaves.StateSynced || g.Conflict != nil) {
			t.Fatalf("cloud status = %+v, want synced without a conflict", g)
		}
	}
	if err := cloudSaves.SyncNow(game.ID); err != nil {
		t.Fatalf("sync after the launch: %v", err)
	}
	for _, g := range cloudSaves.Status().Games {
		if g.GameID == game.ID && (g.State != cloudsaves.StateSynced || g.Conflict != nil) {
			t.Fatalf("cloud status after a sync = %+v, want synced without a conflict", g)
		}
	}
}
