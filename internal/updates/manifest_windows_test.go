//go:build windows

package updates

import (
	"context"
	"path/filepath"
	"syscall"
	"testing"
)

func lockExclusive(t *testing.T, path string) {
	t.Helper()
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := syscall.CreateFile(name, syscall.GENERIC_READ, 0, nil,
		syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatalf("lock %s: %v", path, err)
	}
	t.Cleanup(func() {
		if err := syscall.CloseHandle(handle); err != nil {
			t.Errorf("close handle: %v", err)
		}
	})
}

// A running game, an installer or an antivirus holds files open. Those files
// are unread, not damaged, and reporting them as damage is what makes a healthy
// installation look destroyed right after playing it.
func TestVerifyManifestSeparatesBusyFilesFromDamage(t *testing.T) {
	root := sampleTree(t)
	manifest, err := BuildManifest(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	lockExclusive(t, filepath.Join(root, "game.exe"))

	result, err := VerifyManifest(context.Background(), root, manifest, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := result.Count(IssueCorrupted, IssueSize, IssueMissing); got != 0 {
		t.Fatalf("damage reported for a busy file: %d issues %+v", got, result.Issues)
	}
	if got := result.Count(IssueUnreadable); got != 1 {
		t.Fatalf("unreadable files = %d, want 1: %+v", got, result.Issues)
	}
}
