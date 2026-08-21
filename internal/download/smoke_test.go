//go:build smoke

package download

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/anacrolix/torrent/metainfo"
	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	sintelURL = "https://webtorrent.io/torrents/sintel.torrent"
	debianURL = "https://cdimage.debian.org/debian-cd/current/amd64/bt-cd/debian-13.6.0-amd64-netinst.iso.torrent"
)

func fetchTorrentFile(t *testing.T, url, dest string) string {
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("download torrent file: %v", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read torrent file: %v", err)
	}
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return dest
}

func startManager(t *testing.T, cfgDir string) *Manager {
	m := mustManagerAt(t, cfgDir)
	if err := m.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatalf("startup: %v", err)
	}
	return m
}

func logState(t *testing.T, d Download) {
	t.Logf("status=%s progress=%.1f%% done=%d/%d speed=%d up=%d seeders=%d peers=%d eta=%d err=%q",
		d.Status, d.Progress*100, d.Downloaded, d.Total, d.DownloadSpeed, d.UploadSpeed, d.Seeders, d.Peers, d.ETASeconds, d.Error)
}

func waitUntilSmoke(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Errorf("timed out waiting for %s", what)
}

func waitFor(t *testing.T, m *Manager, id string, timeout time.Duration, cond func(Download) bool, what string) Download {
	deadline := time.Now().Add(timeout)
	var last Download
	for time.Now().Before(deadline) {
		d, err := m.Get(id)
		if err != nil {
			t.Fatalf("get %s: %v", id, err)
		}
		last = d
		if cond(d) {
			logState(t, d)
			return d
		}
		if d.Status == StatusFailed {
			t.Fatalf("download failed while waiting for %s: %s", what, d.Error)
		}
		time.Sleep(2 * time.Second)
		logState(t, d)
	}
	t.Fatalf("timeout waiting for %s; last state: %+v", what, last)
	return last
}

func TestSmokeSelectedFilesCompletion(t *testing.T) {
	cfg := t.TempDir()
	dest := t.TempDir()
	m := startManager(t, cfg)
	defer func() {
		m.ServiceShutdown()
		time.Sleep(2 * time.Second)
	}()

	tf := fetchTorrentFile(t, sintelURL, filepath.Join(t.TempDir(), "sintel.torrent"))
	info, err := m.FetchMetadata(tf)
	if err != nil {
		t.Fatalf("fetch metadata: %v", err)
	}
	t.Logf("torrent %q hash=%s total=%d files=%d", info.Name, info.InfoHash, info.TotalBytes, len(info.Files))
	for i, f := range info.Files {
		t.Logf("  [%d] %s (%d bytes)", i, f.Path, f.Size)
	}

	var selected []int
	var selectedBytes int64
	for i, f := range info.Files {
		if f.Size < 5<<20 {
			selected = append(selected, i)
			selectedBytes += f.Size
		}
	}
	if len(selected) == 0 || len(selected) == len(info.Files) {
		t.Fatalf("expected a mix of small and large files, got %d/%d selected", len(selected), len(info.Files))
	}
	t.Logf("selecting %d small files, %d bytes total", len(selected), selectedBytes)

	d, err := m.StartDownload(info.InfoHash, dest, selected)
	if err != nil {
		t.Fatalf("start download: %v", err)
	}
	if d.Total != selectedBytes {
		t.Errorf("Total=%d want %d (selected bytes)", d.Total, selectedBytes)
	}

	done := waitFor(t, m, d.ID, 5*time.Minute, func(d Download) bool { return d.Status == StatusCompleted }, "completion")
	if done.CompletedAt == nil {
		t.Error("CompletedAt is nil after completion")
	}
	if done.Seeding {
		t.Error("Seeding should be false by default")
	}

	root := filepath.Join(dest, done.Name)
	for _, i := range selected {
		p := filepath.Join(root, filepath.FromSlash(info.Files[i].Path))
		st, err := os.Stat(p)
		if err != nil {
			t.Errorf("selected file missing: %s: %v", p, err)
			continue
		}
		if st.Size() != info.Files[i].Size {
			t.Errorf("size mismatch %s: %d want %d", p, st.Size(), info.Files[i].Size)
		}
	}
}

func TestSmokeMagnetPauseResumeRestoreCancel(t *testing.T) {
	cfg := t.TempDir()
	dest := t.TempDir()
	m := startManager(t, cfg)

	tf := fetchTorrentFile(t, debianURL, filepath.Join(t.TempDir(), "debian.torrent"))
	mi, err := metainfo.LoadFromFile(tf)
	if err != nil {
		t.Fatal(err)
	}
	mag, err := mi.MagnetV2()
	if err != nil {
		t.Fatal(err)
	}
	magnet := mag.String()
	t.Logf("magnet: %s", magnet)

	info, err := m.FetchMetadata(magnet)
	if err != nil {
		t.Fatalf("fetch metadata from magnet: %v", err)
	}
	t.Logf("metadata via magnet ok: %q files=%d", info.Name, len(info.Files))

	all := make([]int, len(info.Files))
	for i := range all {
		all[i] = i
	}
	d, err := m.StartDownload(info.InfoHash, dest, all)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	id := d.ID

	waitFor(t, m, id, 5*time.Minute, func(d Download) bool { return d.Downloaded > 3<<20 }, "3MB of progress")

	if err := m.Pause(id); err != nil {
		t.Fatalf("pause: %v", err)
	}
	p, _ := m.Get(id)
	if p.Status != StatusPaused {
		t.Fatalf("status after pause = %s", p.Status)
	}
	pausedBytes := p.Downloaded
	time.Sleep(5 * time.Second)
	p2, _ := m.Get(id)
	t.Logf("paused: bytes %d -> %d over 5s", pausedBytes, p2.Downloaded)

	if err := m.Resume(id); err != nil {
		t.Fatalf("resume: %v", err)
	}
	waitFor(t, m, id, 3*time.Minute, func(d Download) bool { return d.Downloaded > pausedBytes+1<<20 }, "progress after resume")

	if err := m.ServiceShutdown(); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	t.Log("manager stopped; restarting to test restore+recheck")

	m2 := startManager(t, cfg)
	list := m2.List()
	if len(list) != 1 {
		t.Fatalf("restored %d downloads, want 1", len(list))
	}
	r := list[0]
	t.Logf("restored: status=%s downloaded=%d", r.Status, r.Downloaded)

	got := waitFor(t, m2, r.ID, 5*time.Minute, func(d Download) bool {
		return (d.Status == StatusDownloading || d.Status == StatusCompleted) && d.Downloaded >= pausedBytes
	}, "recheck and resume after restart")
	t.Logf("after restore: %d bytes (before shutdown: %d)", got.Downloaded, pausedBytes)

	waitFor(t, m2, r.ID, 3*time.Minute, func(d Download) bool {
		return d.Status == StatusCompleted || d.Downloaded > got.Downloaded+1<<20
	}, "progress after restore")

	final, _ := m2.Get(r.ID)
	if final.Status == StatusCompleted {
		t.Log("download completed before cancel phase; removing record instead")
		if err := m2.Remove(r.ID); err != nil {
			t.Fatalf("remove: %v", err)
		}
		if len(m2.List()) != 0 {
			t.Errorf("list not empty after remove")
		}
		if _, err := os.Stat(filepath.Join(dest, final.Name)); err != nil {
			t.Errorf("completed file missing after remove: %v", err)
		}
	} else {
		if err := m2.Cancel(r.ID); err != nil {
			t.Fatalf("cancel: %v", err)
		}
		if len(m2.List()) != 0 {
			t.Errorf("list not empty after cancel")
		}
		waitUntilSmoke(t, "partial files deleted", func() bool {
			entries, _ := os.ReadDir(dest)
			return len(entries) == 0
		})
	}
	if err := m2.ServiceShutdown(); err != nil {
		t.Fatalf("second shutdown: %v", err)
	}
	time.Sleep(2 * time.Second)
	fmt.Println("smoke ok")
}
