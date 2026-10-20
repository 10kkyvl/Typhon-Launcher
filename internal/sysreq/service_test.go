package sysreq

import (
	"context"
	"errors"
	"testing"

	"typhon/internal/metadata"
	"typhon/internal/platform"
)

type fakeSource struct {
	reqs  metadata.SteamRequirements
	found bool
	err   error
	asked string
}

func (f *fakeSource) SteamRequirements(id string) (metadata.SteamRequirements, bool, error) {
	f.asked = id
	return f.reqs, f.found, f.err
}

func testService(src Source, detects *int) *Service {
	return &Service{
		source: src,
		detect: func() (Hardware, bool) {
			*detects++
			hw := rig()
			hw.FreeBytes, hw.FreeKnown = 0, false
			return hw, true
		},
		storage: func(path string) (platform.StorageInfo, error) {
			return platform.StorageInfo{Path: path, FreeBytes: 30 * gb}, nil
		},
	}
}

func TestNewServiceRejectsNilSource(t *testing.T) {
	if _, err := NewService(nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestCheckEmptyGameID(t *testing.T) {
	ctx := context.Background()
	var n int
	s := testService(&fakeSource{}, &n)
	for _, id := range []string{"", "   "} {
		if _, err := s.Check(ctx, id, "C:/Games"); !errors.Is(err, ErrEmptyGameID) {
			t.Fatalf("Check(%q) err = %v", id, err)
		}
	}
}

func TestCheckSourceError(t *testing.T) {
	boom := errors.New("no such game")
	ctx := context.Background()
	var n int
	s := testService(&fakeSource{err: boom}, &n)
	if _, err := s.Check(ctx, "g1", "C:/Games"); !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
}

func TestCheckWithoutRequirementsIsUnknownNotError(t *testing.T) {
	ctx := context.Background()
	var n int
	s := testService(&fakeSource{found: false}, &n)
	r, err := s.Check(ctx, "g1", "C:/Games")
	if err != nil {
		t.Fatal(err)
	}
	if r.Verdict != VerdictUnknown || r.Reason != ReasonNoRequirements || r.Checks == nil {
		t.Fatalf("report = %+v", r)
	}
	if n != 0 {
		t.Fatalf("hardware probed %d times for a game without requirements", n)
	}
}

func TestCheckUsesInstallVolumeAndCachesHardware(t *testing.T) {
	ctx := context.Background()
	var n int
	src := &fakeSource{found: true, reqs: metadata.SteamRequirements{
		Minimum: []metadata.Requirement{{Label: "Storage", Value: "50 GB available space"}, {Label: "Memory", Value: "8 GB RAM"}},
	}}
	s := testService(src, &n)
	r, err := s.Check(ctx, " g1 ", "D:/Games")
	if err != nil {
		t.Fatal(err)
	}
	if src.asked != "g1" {
		t.Fatalf("asked %q", src.asked)
	}
	if r.Verdict != VerdictNotEnough || statusOf(t, r, KindStorage) != StatusBelowMinimum {
		t.Fatalf("report = %+v", r)
	}
	if _, err := s.Check(ctx, "g1", "D:/Games"); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("hardware probed %d times, want 1", n)
	}
}

func TestCheckStorageProblemsMakeSpaceUnknown(t *testing.T) {
	src := &fakeSource{found: true, reqs: metadata.SteamRequirements{
		Minimum: []metadata.Requirement{{Label: "Storage", Value: "50 GB"}},
	}}
	for name, path := range map[string]string{"empty path": "", "failing volume": "Z:/x"} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			var n int
			s := testService(src, &n)
			s.storage = func(string) (platform.StorageInfo, error) { return platform.StorageInfo{}, platform.ErrEmptyPath }
			r, err := s.Check(ctx, "g1", path)
			if err != nil {
				t.Fatal(err)
			}
			if got := statusOf(t, r, KindStorage); got != StatusUnknown {
				t.Fatalf("status = %s", got)
			}
		})
	}
}

func TestIncompleteDetectionIsRetried(t *testing.T) {
	ctx := context.Background()
	var n int
	src := &fakeSource{found: true, reqs: metadata.SteamRequirements{
		Minimum: []metadata.Requirement{{Label: "Memory", Value: "8 GB RAM"}},
	}}
	s := testService(src, &n)
	s.detect = func() (Hardware, bool) {
		n++
		return Hardware{}, false
	}
	for range 2 {
		r, err := s.Check(ctx, "g1", "")
		if err != nil {
			t.Fatal(err)
		}
		if r.Verdict != VerdictUnknown {
			t.Fatalf("verdict = %s", r.Verdict)
		}
	}
	if n != 2 {
		t.Fatalf("probed %d times, want 2", n)
	}
}

func storageSource() *fakeSource {
	return &fakeSource{found: true, reqs: metadata.SteamRequirements{
		Minimum: []metadata.Requirement{{Label: "Storage", Value: "50 GB"}},
	}}
}

func TestCheckAbandonsHungFreeSpaceQueryOnCancel(t *testing.T) {
	var n int
	s := testService(storageSource(), &n)
	started := make(chan struct{})
	release := make(chan struct{})
	s.storage = func(string) (platform.StorageInfo, error) {
		close(started)
		<-release
		return platform.StorageInfo{FreeBytes: 100 * gb}, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := s.Check(ctx, "g1", `\dead-host\share`)
		result <- err
	}()
	<-started
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	close(release)
	if err := s.ServiceShutdown(); err != nil {
		t.Fatal(err)
	}
}

func TestCheckCanceledBeforeStart(t *testing.T) {
	var n int
	s := testService(storageSource(), &n)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Check(ctx, "g1", "D:/Games"); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
}

func TestHardwareProbeRunsOutsideTheLock(t *testing.T) {
	var n int
	s := testService(storageSource(), &n)
	var lockedDuringProbe bool
	s.detect = func() (Hardware, bool) {
		if !s.mu.TryLock() {
			lockedDuringProbe = true
		} else {
			s.mu.Unlock()
		}
		return rig(), true
	}
	if _, err := s.Check(context.Background(), "g1", ""); err != nil {
		t.Fatal(err)
	}
	if lockedDuringProbe {
		t.Fatal("detect ran while the service mutex was held")
	}
	if !s.cached {
		t.Fatal("complete probe was not cached")
	}
}
