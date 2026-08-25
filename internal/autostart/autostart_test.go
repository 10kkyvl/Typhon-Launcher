package autostart

import (
	"errors"
	"sync"
	"testing"
)

type fakeManager struct {
	mu sync.Mutex

	enabled      bool
	enableErr    error
	disableErr   error
	isEnabledErr error

	enableCalls    int
	disableCalls   int
	isEnabledCalls int
}

func (f *fakeManager) Enable() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.enableCalls++
	if f.enableErr != nil {
		return f.enableErr
	}
	f.enabled = true
	return nil
}

func (f *fakeManager) Disable() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.disableCalls++
	if f.disableErr != nil {
		return f.disableErr
	}
	f.enabled = false
	return nil
}

func (f *fakeManager) IsEnabled() (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.isEnabledCalls++
	if f.isEnabledErr != nil {
		return false, f.isEnabledErr
	}
	return f.enabled, nil
}

func (f *fakeManager) counts() (enable, disable int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.enableCalls, f.disableCalls
}

func TestNewServiceRejectsNilManager(t *testing.T) {
	if _, err := NewService(nil); err == nil {
		t.Fatal("NewService(nil) must return an error")
	}

	var typedNil *fakeManager
	if _, err := NewService(typedNil); err == nil {
		t.Fatal("NewService with a typed nil manager must return an error")
	}
}

func TestApplyIdempotent(t *testing.T) {
	cases := []struct {
		name           string
		initialEnabled bool
		want           bool
		wantEnable     int
		wantDisable    int
	}{
		{"already enabled, request enable", true, true, 0, 0},
		{"disabled, request enable", false, true, 1, 0},
		{"enabled, request disable", true, false, 0, 1},
		{"already disabled, request disable", false, false, 0, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mgr := &fakeManager{enabled: c.initialEnabled}
			svc, err := NewService(mgr)
			if err != nil {
				t.Fatalf("NewService: %v", err)
			}
			if err := svc.Apply(c.want); err != nil {
				t.Fatalf("Apply: %v", err)
			}
			enable, disable := mgr.counts()
			if enable != c.wantEnable || disable != c.wantDisable {
				t.Fatalf("enable calls = %d, disable calls = %d, want %d/%d", enable, disable, c.wantEnable, c.wantDisable)
			}
		})
	}
}

func TestApplyIsEnabledErrorDoesNotCallEnableOrDisable(t *testing.T) {
	wantErr := errors.New("registry unavailable")
	mgr := &fakeManager{isEnabledErr: wantErr}
	svc, err := NewService(mgr)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	if err := svc.Apply(true); err == nil {
		t.Fatal("Apply must return an error when IsEnabled fails")
	}
	enable, disable := mgr.counts()
	if enable != 0 || disable != 0 {
		t.Fatalf("enable calls = %d, disable calls = %d, want 0/0", enable, disable)
	}
}

func TestApplyEnableErrorIsWrapped(t *testing.T) {
	wantErr := errors.New("access denied")
	mgr := &fakeManager{enabled: false, enableErr: wantErr}
	svc, err := NewService(mgr)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	err = svc.Apply(true)
	if err == nil {
		t.Fatal("Apply must return an error when Enable fails")
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("Apply error = %v, want wrapped %v", err, wantErr)
	}
}

func TestApplyConcurrent(t *testing.T) {
	mgr := &fakeManager{enabled: false}
	svc, err := NewService(mgr)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		want := i%2 == 0
		go func() {
			defer wg.Done()
			if err := svc.Apply(want); err != nil {
				t.Errorf("Apply(%v) error: %v", want, err)
			}
		}()
	}
	wg.Wait()
}
