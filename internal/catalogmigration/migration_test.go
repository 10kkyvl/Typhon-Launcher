package catalogmigration

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"typhon/internal/catalog"
	"typhon/internal/storage"
)

func TestReportApplyResumePreservesPersonalData(t *testing.T) {
	dir := t.TempDir()
	games := []catalog.Game{{ID: "old", Title: "112 Operator", ExternalIDs: catalog.ExternalIDs{IGDB: "112"}}, {ID: "other", Title: "112 Operator [Archive]", ExternalIDs: catalog.ExternalIDs{IGDB: "112"}}, {ID: "private", Title: "Unknown installation"}}
	if err := storage.Save(filepath.Join(dir, "catalog.json"), 1, games); err != nil {
		t.Fatal(err)
	}
	personal := []byte(`{"version":1,"data":[{"id":"install-id","canonicalGameId":"other","favorite":true,"releaseId":"exact-release","sourceId":"source-a","distributionId":"line-a","installDir":"/only-copy","history":["played"]}]}`)
	path := filepath.Join(dir, "library.json")
	if err := os.WriteFile(path, personal, 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(dir, "catalog.json"))
	if err != nil {
		t.Fatal(err)
	}
	report, err := Plan(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Confident) != 1 || len(report.WithoutProviders) != 1 || report.References["library.json"]["other"] != 1 {
		t.Fatalf("report %+v", report)
	}
	if _, err = os.Stat(filepath.Join(dir, "catalog-redirects.json")); !os.IsNotExist(err) {
		t.Fatal("dry run wrote data")
	}
	for range 2 {
		if err = Apply(dir, report); err != nil {
			t.Fatal(err)
		}
	}
	after, err := os.ReadFile(filepath.Join(dir, "catalog.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("catalog was rewritten")
	}
	after, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(personal) != string(after) {
		t.Fatal("personal state/provenance changed")
	}
	svc, err := catalog.NewServiceAt(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !svc.SameGame("old", "other") {
		t.Fatal("old reference not resolved")
	}
	g, err := svc.GetGame("other")
	if err != nil || g.ID != "old" {
		t.Fatalf("redirect: %+v %v", g, err)
	}
	if _, err = svc.GetGame("private"); err != nil {
		t.Fatal("unknown installation lost")
	}
	backups, err := filepath.Glob(filepath.Join(dir, "catalog-migration-backups", "*"))
	if err != nil || len(backups) != 1 {
		t.Fatalf("backup: %v %v", backups, err)
	}
	if err = Restore(dir, backups[0]); err != nil {
		t.Fatal(err)
	}
	restored, err := catalog.NewServiceAt(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := restored.GetGame("other"); err != nil || got.ID != "other" {
		t.Fatal("rollback did not remove redirect")
	}
	after, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(personal) {
		t.Fatal("rollback changed personal data")
	}

}
func TestReportRejectsChangedDataAndTitleOnlyMerge(t *testing.T) {
	dir := t.TempDir()
	if err := storage.Save(filepath.Join(dir, "catalog.json"), 1, []catalog.Game{{ID: "a", Title: "Prey", ExternalIDs: catalog.ExternalIDs{IGDB: "1"}}, {ID: "b", Title: "Prey", ExternalIDs: catalog.ExternalIDs{IGDB: "2"}}}); err != nil {
		t.Fatal(err)
	}
	report, err := Plan(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Confident) != 0 || len(report.Review) != 1 {
		t.Fatal("homonyms merged")
	}
	report.Redirects["b"] = "a"
	if Apply(dir, report) == nil {
		t.Fatal("forged title merge accepted")
	}
	delete(report.Redirects, "b")
	if err = os.WriteFile(filepath.Join(dir, "catalog.json"), []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if Apply(dir, report) == nil {
		t.Fatal("stale plan applied")
	}
}

func TestReportJoinsConfirmedProviderLinksButRejectsConflictingClaims(t *testing.T) {
	for _, conflict := range []bool{false, true} {
		t.Run(map[bool]string{false: "confirmed", true: "conflict"}[conflict], func(t *testing.T) {
			dir := t.TempDir()
			games := []catalog.Game{{ID: "igdb", Title: "Official", ExternalIDs: catalog.ExternalIDs{IGDB: "10"}, ProviderLinks: map[string][]string{"steam": {"20", "21"}}}, {ID: "steam", Title: "Store title", ExternalIDs: catalog.ExternalIDs{Steam: "21"}}}
			if conflict {
				games[1].ExternalIDs.IGDB = "11"
			}
			if err := storage.Save(filepath.Join(dir, "catalog.json"), 1, games); err != nil {
				t.Fatal(err)
			}
			report, err := Plan(dir)
			if err != nil {
				t.Fatal(err)
			}
			if conflict && (len(report.Confident) != 0 || len(report.Review) == 0) {
				t.Fatalf("conflict %+v", report)
			}
			if !conflict && len(report.Confident) != 1 {
				t.Fatalf("confirmed links ignored %+v", report)
			}
		})
	}
}

func TestApplyResumesAfterBackupBeforeRedirectCommit(t *testing.T) {
	dir := t.TempDir()
	if err := storage.Save(filepath.Join(dir, "catalog.json"), 1, []catalog.Game{{ID: "a", Title: "Official", ExternalIDs: catalog.ExternalIDs{IGDB: "12"}}, {ID: "b", Title: "Old release title", ExternalIDs: catalog.ExternalIDs{IGDB: "12"}}}); err != nil {
		t.Fatal(err)
	}
	report, err := Plan(dir)
	if err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(dir, "catalog-migration-backups", digest([]byte(fmt.Sprint(report.CreatedAt.UnixNano()))))
	if err = os.MkdirAll(backup, 0700); err != nil {
		t.Fatal(err)
	}
	if err = storage.Save(filepath.Join(backup, "catalog-redirects.json"), 1, map[string]string{}); err != nil {
		t.Fatal(err)
	}
	// This is the durable state after backup, before the atomic sidecar commit.
	if err = Apply(dir, report); err != nil {
		t.Fatal(err)
	}
	var redirects map[string]string
	if err = storage.Load(filepath.Join(dir, "catalog-redirects.json"), 1, nil, &redirects); err != nil || redirects["b"] != "a" {
		t.Fatalf("resume %v %v", redirects, err)
	}
	if err = Restore(dir, backup); err != nil {
		t.Fatal(err)
	}
	redirects = nil
	if err = storage.Load(filepath.Join(dir, "catalog-redirects.json"), 1, nil, &redirects); err != nil || len(redirects) != 0 {
		t.Fatalf("backup overwritten on resume %v %v", redirects, err)
	}
}

func TestPlanIgnoresSymlinkToExternalJSON(t *testing.T) {
	dir := t.TempDir()
	if err := storage.Save(filepath.Join(dir, "catalog.json"), 1, []catalog.Game{}); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.json")
	if err := os.WriteFile(outside, []byte("not valid JSON"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "external.json")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	report, err := Plan(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, included := report.Hashes["external.json"]; included {
		t.Fatal("report included an external symlink")
	}
}
