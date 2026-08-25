package selfupdate

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestStatusMarshalJSONTimestamps(t *testing.T) {
	stamp := time.Date(2026, 8, 25, 3, 4, 5, 0, time.UTC)

	tests := []struct {
		name        string
		status      Status
		wantKeys    []string
		wantMissing []string
	}{
		{
			name:        "zero timestamps are omitted",
			status:      Status{State: StateIdle, CurrentVersion: "0.1.1"},
			wantKeys:    []string{`"currentVersion":"0.1.1"`},
			wantMissing: []string{"checkedAt", "publishedAt"},
		},
		{
			name:        "checked at is kept",
			status:      Status{State: StateIdle, CurrentVersion: "0.1.1", CheckedAt: stamp},
			wantKeys:    []string{`"checkedAt":"2026-08-25T03:04:05Z"`},
			wantMissing: []string{"publishedAt"},
		},
		{
			name: "published at is kept",
			status: Status{
				State:            StateAvailable,
				CurrentVersion:   "0.1.1",
				AvailableVersion: "0.1.2",
				PublishedAt:      stamp,
				CheckedAt:        stamp,
			},
			wantKeys: []string{`"publishedAt":"2026-08-25T03:04:05Z"`, `"checkedAt":"2026-08-25T03:04:05Z"`},
		},
		{
			name:        "error status without a check keeps no timestamp",
			status:      Status{State: StateFailed, CurrentVersion: "0.1.1", Error: "boom", ErrorCode: "network"},
			wantKeys:    []string{`"error":"boom"`},
			wantMissing: []string{"checkedAt", "publishedAt"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.status)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			got := string(data)
			for _, want := range tt.wantKeys {
				if !strings.Contains(got, want) {
					t.Fatalf("marshal(%+v) = %s, want it to contain %s", tt.status, got, want)
				}
			}
			for _, missing := range tt.wantMissing {
				if strings.Contains(got, missing) {
					t.Fatalf("marshal(%+v) = %s, want no %s", tt.status, got, missing)
				}
			}
		})
	}
}
