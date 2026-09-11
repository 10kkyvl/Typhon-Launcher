package updates

import (
	"context"
	"testing"
	"time"
	"typhon/internal/download"
	"typhon/internal/sources"
)

func TestLegacyUpdateTaskReusesOnlyTheSamePayload(t *testing.T) {
	uploaded := time.Unix(200, 0)
	oldDate := time.Unix(100, 0)
	r := release("r2", "2.0", 100)
	r.InfoHash = "abcdef"
	r.UploadedAt = &uploaded
	plan := UpdatePlan{ID: "plan", GameID: "g", SourceID: "src", DistributionID: "main", TargetReleaseID: "r2", TargetVersion: "2.0", TargetReleaseUploadedAt: &uploaded}
	for _, tc := range []struct {
		name, hash, distribution string
		date                     *time.Time
		want                     bool
	}{
		{name: "legacy same payload", hash: "ABCDEF", want: true},
		{name: "replaced revision", hash: "different", want: false},
		{name: "unknown payload", want: false},
		{name: "conflicting distribution", hash: "abcdef", distribution: "other", want: false},
		{name: "conflicting recorded date", hash: "abcdef", date: &oldDate, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, err := newServiceAt(t.TempDir(), nil)
			if err != nil {
				t.Fatal(err)
			}
			downloads := newFakeDownloads()
			s.downloads = downloads
			s.releases = &fakeReleases{list: []sources.Release{r}}
			downloads.tasks["existing"] = &download.Download{ID: "existing", InfoHash: tc.hash, Destination: "downloads", Status: download.StatusDownloading, Origin: download.Origin{LibraryID: "g", Purpose: download.PurposeUpdate, ReleaseID: "r2", SourceID: "src", Version: "2.0", DistributionID: tc.distribution, ReleaseUploadedAt: tc.date}}
			_, found := s.existingTask(plan, r, "downloads", false, false)
			if found != tc.want {
				t.Fatalf("reuse=%v want=%v", found, tc.want)
			}
			if tc.want {
				task, err := s.downloadRelease(context.Background(), plan, "r2", "downloads", false, false)
				if err != nil || task.ID != "existing" || len(downloads.requests) != 0 {
					t.Fatalf("created duplicate task: %+v err=%v requests=%+v", task, err, downloads.requests)
				}
			}
		})
	}
}
