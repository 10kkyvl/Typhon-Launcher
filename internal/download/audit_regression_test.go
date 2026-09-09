package download

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRegressionRepointOccupiedTargetDeletesSource(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "source")
	dst := filepath.Join(root, "target")
	os.MkdirAll(src, 0755)
	os.MkdirAll(dst, 0755)
	os.WriteFile(filepath.Join(src, "my-download.bin"), []byte("ONLY COPY"), 0644)
	os.WriteFile(filepath.Join(dst, "unrelated.txt"), []byte("unrelated"), 0644)
	if err := moveTreeIfPresent(context.Background(), src, dst); err == nil {
		t.Fatal("accepted different target")
	}
	if b, e := os.ReadFile(filepath.Join(src, "my-download.bin")); e != nil || string(b) != "ONLY COPY" {
		t.Fatal("source lost")
	}
}

func TestRegressionDeleteFlatDataLeavesPayload(t *testing.T) {
	dest := t.TempDir()
	payload := filepath.Join(dest, "game.exe")
	if err := os.WriteFile(payload, []byte("payload"), 0644); err != nil {
		t.Fatal(err)
	}
	m := newTestManager(t, 2)
	m.addTestItem("a", StatusCompleted)
	m.mu.Lock()
	d := m.findLocked("a")
	d.Destination = dest
	d.Flat = true
	d.InPlace = true
	m.mu.Unlock()
	if err := m.DeleteData("a"); err == nil {
		t.Fatal("in-place game data must not be deleted through download history")
	}
	m.wg.Wait()
	if _, err := os.Stat(payload); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Get("a"); err != nil {
		t.Fatal("record lost")
	}
	t.Log("Regression: delete-data removes record but flat payload remains on disk")
}

func TestDeleteFlatDownloadOnlyRemovesItsFiles(t *testing.T) {
	dest := t.TempDir()
	payload := filepath.Join(dest, "game.exe")
	other := filepath.Join(dest, "unrelated.txt")
	if err := os.WriteFile(payload, []byte("payload"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(other, []byte("keep"), 0644); err != nil {
		t.Fatal(err)
	}
	m := newTestManager(t, 2)
	m.addTestItem("a", StatusCompleted)
	m.mu.Lock()
	d := m.findLocked("a")
	d.Destination = dest
	d.Flat = true
	d.Files = []FileState{{Path: "game.exe", Selected: true}}
	m.mu.Unlock()
	if err := m.DeleteData("a"); err != nil {
		t.Fatal(err)
	}
	m.wg.Wait()
	if _, err := os.Stat(payload); !os.IsNotExist(err) {
		t.Fatal("payload not removed")
	}
	if _, err := os.Stat(other); err != nil {
		t.Fatal("unrelated file lost")
	}
}

func TestStopAndWaitJoinsAnEarlierCancel(t *testing.T) {
	m := newTestManager(t, 1)
	m.addTestItem("a", StatusDownloading)
	eng := &gatedEngine{fakeTorrent: &fakeTorrent{size: 100}, gate: make(chan struct{})}
	m.mu.Lock()
	m.findLocked("a").InPlace = true
	m.engines["a"] = eng
	m.mu.Unlock()
	if err := m.Cancel("a"); err != nil {
		close(eng.gate)
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() { result <- m.StopAndWait("a") }()
	select {
	case err := <-result:
		close(eng.gate)
		t.Fatalf("stop returned before writer closed: %v", err)
	case <-time.After(25 * time.Millisecond):
	}
	close(eng.gate)
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("stop did not finish")
	}
	if !eng.wasDropped() {
		t.Fatal("engine still alive")
	}
}
