package install

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTrackInstallSizeVerifyingFollowsOption(t *testing.T) {
	log := `2026-09-10 Filename: C:\Games\_Redist\QuickSFV.exe` + "\n"
	for _, tc := range []struct {
		name   string
		verify bool
		want   Status
	}{
		{"verification skipped keeps installing", false, StatusInstalling},
		{"verification allowed reports verifying", true, StatusVerifying},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, _, _ := newTestService(t)
			old := installPollInterval
			installPollInterval = time.Millisecond
			t.Cleanup(func() { installPollInterval = old })

			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "data.bin"), make([]byte, 10), 0o600); err != nil {
				t.Fatal(err)
			}
			logPath := filepath.Join(t.TempDir(), "install.log")
			if err := os.WriteFile(logPath, []byte(log), 0o600); err != nil {
				t.Fatal(err)
			}
			s.mu.Lock()
			s.items = append(s.items, &Installation{ID: "track", Status: StatusInstalling})
			s.mu.Unlock()

			stop := s.trackInstallSize(context.Background(), "track", dir, 100, logPath, tc.verify)
			// updateProgress runs after the log check in the same tick, so a
			// recorded size proves the check already had its chance.
			waitFor(t, "install size to be recorded", func() bool {
				item, _ := s.snapshot("track")
				return item.BytesDone == 10
			})
			stop()
			item, _ := s.snapshot("track")
			if item.Status != tc.want {
				t.Fatalf("status = %s, want %s", item.Status, tc.want)
			}
		})
	}
}

func TestVerifierLogEvidence(t *testing.T) {
	for _, test := range []struct {
		log  string
		want bool
	}{
		{`2026-09-10 Filename: C:\Games\_Redist\QuickSFV.exe`, true},
		{`2026-09-10 Dest filename: C:\Games\_Redist\QuickSFV.exe`, false},
		{`2026-09-10 Filename: C:\Games\QuickSFV.exe.backup`, false},
		{`Installation process succeeded.`, false},
	} {
		if got := logHasVerifier(test.log); got != test.want {
			t.Errorf("%q = %v", test.log, got)
		}
	}
}

func TestInstallerProgressDoesNotClaimCompletion(t *testing.T) {
	s, _, _ := newTestService(t)
	s.mu.Lock()
	s.items = append(s.items, &Installation{ID: "phase", Status: StatusInstalling})
	s.mu.Unlock()
	s.updateProgress("phase", Progress{BytesDone: 100, BytesTotal: 100})
	item, _ := s.snapshot("phase")
	if item.Progress != 0.99 {
		t.Fatalf("installing progress=%v", item.Progress)
	}
	if err := s.setInstallerVerifying("phase"); err != nil {
		t.Fatal(err)
	}
	item, _ = s.snapshot("phase")
	if item.Status != StatusVerifying {
		t.Fatal(item.Status)
	}
	for _, status := range []Status{StatusCancelled, StatusWaitingForUser, StatusCompleted} {
		if err := s.setStatus("phase", status); err != nil {
			t.Fatal(err)
		}
		if err := s.setInstallerVerifying("phase"); err != nil {
			t.Fatal(err)
		}
		item, _ = s.snapshot("phase")
		if item.Status != status {
			t.Fatalf("monitor replaced %s with %s", status, item.Status)
		}
	}
}
