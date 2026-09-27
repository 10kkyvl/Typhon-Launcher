package install

import "testing"

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
