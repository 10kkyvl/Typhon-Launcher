package main

import (
	"errors"
	"fmt"
	"testing"

	"typhon/internal/compat"
	"typhon/internal/platform"
	"typhon/internal/wine"
)

func TestCompatEnvFrom(t *testing.T) {
	info := platform.SystemInfo{OS: "macOS 26.6.2", CPU: "Apple M4 Pro"}
	infoErr := errors.New("sysctl hw.memsize: operation not permitted")
	detectErr := errors.New("stat Info.plist: input/output error")

	tests := []struct {
		name       string
		systemInfo func() (platform.SystemInfo, error)
		detect     func() (wine.Runtime, error)
		want       compat.Env
		wantErr    error
	}{
		{
			name:       "everything known",
			systemInfo: func() (platform.SystemInfo, error) { return info, nil },
			detect:     func() (wine.Runtime, error) { return wine.Runtime{Version: "26.3"}, nil },
			want:       compat.Env{OSVersion: "macOS 26.6.2", CrossOver: "26.3", Chip: "Apple M4 Pro"},
		},
		{
			name:       "CrossOver is not installed",
			systemInfo: func() (platform.SystemInfo, error) { return info, nil },
			detect: func() (wine.Runtime, error) {
				return wine.Runtime{}, fmt.Errorf("%w: искали в []", wine.ErrNotInstalled)
			},
			want: compat.Env{OSVersion: "macOS 26.6.2", Chip: "Apple M4 Pro"},
		},
		{
			name:       "system info failed",
			systemInfo: func() (platform.SystemInfo, error) { return info, infoErr },
			detect:     func() (wine.Runtime, error) { return wine.Runtime{Version: "26.3"}, nil },
			wantErr:    infoErr,
		},
		{
			name:       "CrossOver lookup failed",
			systemInfo: func() (platform.SystemInfo, error) { return info, nil },
			detect:     func() (wine.Runtime, error) { return wine.Runtime{}, detectErr },
			wantErr:    detectErr,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := compatEnvFrom(tt.systemInfo, tt.detect)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("error = %v, want it to wrap %v", err, tt.wantErr)
				}
				if got != (compat.Env{}) {
					t.Fatalf("env = %+v, want zero on error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("env = %+v, want %+v", got, tt.want)
			}
		})
	}
}
