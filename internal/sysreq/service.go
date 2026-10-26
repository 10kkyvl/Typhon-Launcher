package sysreq

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"typhon/internal/metadata"
	"typhon/internal/platform"
)

var ErrEmptyGameID = errors.New("game id is empty")

// Source gives the Steam requirements of a catalog game. found is false when
// the game is known but the store block has not arrived.
type Source interface {
	SteamRequirements(gameID string) (reqs metadata.SteamRequirements, found bool, err error)
}

// Service compares a game's requirements with this machine. Nothing it reads
// about the hardware leaves the process: the only output is the Report
// returned to the window.
type Service struct {
	source  Source
	detect  func() (Hardware, bool)
	storage func(path string) (platform.StorageInfo, error)

	wg       sync.WaitGroup
	mu       sync.Mutex
	hardware Hardware
	cached   bool
}

func NewService(source Source) (*Service, error) {
	if source == nil {
		return nil, errors.New("requirements source is required")
	}
	return &Service{source: source, detect: detectHardware, storage: platform.GetStorageInfo}, nil
}

// shutdownWait bounds how long closing the app waits for a free-space query
// stuck on an unreachable network path.
const shutdownWait = 2 * time.Second

// Check takes the catalog game id, the one the game page opens metadata with,
// and the folder the game will be installed to (its volume decides the free
// space; empty means unknown). An installed game passes the storage check
// outright: its files already occupy the disk.
func (s *Service) Check(ctx context.Context, gameID, installPath string, installed bool) (Report, error) {
	if err := ctx.Err(); err != nil {
		return Report{}, err
	}
	gameID = strings.TrimSpace(gameID)
	if gameID == "" {
		return Report{}, ErrEmptyGameID
	}
	reqs, found, err := s.source.SteamRequirements(gameID)
	if err != nil {
		return Report{}, fmt.Errorf("read requirements of %s: %w", gameID, err)
	}
	if !found {
		return Report{Verdict: VerdictUnknown, Reason: ReasonNoRequirements, Checks: []Check{}}, nil
	}
	hw, err := s.hardwareFor(ctx, installPath, installed)
	if err != nil {
		return Report{}, err
	}
	return compare(hw, rowsOf(reqs.Minimum), rowsOf(reqs.Recommended)), nil
}

func rowsOf(in []metadata.Requirement) []Row {
	out := make([]Row, len(in))
	for i, r := range in {
		out[i] = Row{Label: r.Label, Value: r.Value}
	}
	return out
}

func (s *Service) hardwareFor(ctx context.Context, installPath string, installed bool) (Hardware, error) {
	hw := s.cachedHardware()
	if installed {
		hw.Installed = true
		return hw, nil
	}
	if strings.TrimSpace(installPath) == "" {
		return hw, nil
	}
	info, err := s.freeSpace(ctx, installPath)
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return Hardware{}, err
	case err != nil:
		slog.Warn("free space for requirements check", "path", installPath, "error", err)
		return hw, nil
	}
	hw.FreeBytes, hw.FreeKnown = info.FreeBytes, true
	return hw, nil
}

// cachedHardware probes outside the lock: a slow registry read must not
// block other callers, and a duplicate probe on a cold start is harmless.
func (s *Service) cachedHardware() Hardware {
	s.mu.Lock()
	if s.cached {
		hw := s.hardware
		s.mu.Unlock()
		return hw
	}
	s.mu.Unlock()

	hw, complete := s.detect()
	if complete {
		s.mu.Lock()
		s.hardware, s.cached = hw, true
		s.mu.Unlock()
	}
	return hw
}

type storageResult struct {
	info platform.StorageInfo
	err  error
}

// freeSpace abandons the query when ctx ends: a dead network share can hold
// the syscall for minutes, and the goroutine is left to finish on its own.
func (s *Service) freeSpace(ctx context.Context, path string) (platform.StorageInfo, error) {
	out := make(chan storageResult, 1)
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		info, err := s.storage(path)
		out <- storageResult{info, err}
	}()
	select {
	case r := <-out:
		return r.info, r.err
	case <-ctx.Done():
		return platform.StorageInfo{}, ctx.Err()
	}
}

func (s *Service) ServiceShutdown() error {
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(shutdownWait):
		slog.Warn("free space query still running at shutdown")
	}
	return nil
}

// detectHardware reports complete=false when a probe failed for a reason
// other than "this platform cannot tell", so the next call tries again.
func detectHardware() (Hardware, bool) {
	complete := true
	var hw Hardware
	info, err := platform.GetSystemInfo()
	if err != nil {
		slog.Warn("system info for requirements check", "error", err)
		complete = false
	}
	hw.Arch = info.Arch
	hw.RAMBytes = info.RAMBytes
	if !info.OSUnknown {
		hw.OSName = info.OS
		hw.WinRank = windowsRank(info.OS)
	}
	if !info.CPUUnknown {
		hw.CPUName = info.CPU
	}
	if !info.PhysicalCoresUnknown {
		hw.PhysicalCores = info.PhysicalCores
	}
	gpu, err := platform.DetectGPU()
	switch {
	case err == nil:
		hw.GPUName = gpu.Name
		if gpu.VRAMKnown {
			hw.VRAMBytes = gpu.VRAMBytes
		}
	case errors.Is(err, platform.ErrNoGPU), errors.Is(err, platform.ErrGPUUnsupported):
	default:
		slog.Warn("gpu detection for requirements check", "error", err)
		complete = false
	}
	return hw, complete
}
