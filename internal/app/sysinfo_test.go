package app

import (
	"errors"
	"testing"

	"typhon/internal/platform"
)

func TestGetSystemInfo(t *testing.T) {
	boom := errors.New("memory status unavailable")
	good := platform.SystemInfo{OS: "Windows 10", CPU: "Test CPU", Cores: 8, RAMBytes: 16 << 30}
	tests := []struct {
		name    string
		read    func() (platform.SystemInfo, error)
		want    platform.SystemInfo
		wantErr error
	}{
		{"ok", func() (platform.SystemInfo, error) { return good, nil }, good, nil},
		{
			"unknown flags pass through",
			func() (platform.SystemInfo, error) { return platform.SystemInfo{OS: "linux", CPUUnknown: true}, nil },
			platform.SystemInfo{OS: "linux", CPUUnknown: true}, nil,
		},
		{
			"error is returned and partial data dropped",
			func() (platform.SystemInfo, error) { return good, boom },
			platform.SystemInfo{}, boom,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := &Service{systemInfo: tc.read}
			got, err := s.GetSystemInfo()
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}
