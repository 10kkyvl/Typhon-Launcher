package extlib

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestScannersSkippedAppIDContract(t *testing.T) {
	root := t.TempDir()
	apps := filepath.Join(root, "steamapps")
	if err := os.MkdirAll(filepath.Join(apps, "common", "Half"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(apps, "libraryfolders.vdf"), `"libraryfolders" { "0" { "path" "`+vdfPath(root)+`" } "1" { "path" "`+vdfPath(filepath.Join(root, "gone"))+`" } }`)
	writeFile(t, filepath.Join(apps, "appmanifest_2.acf"), acf("2", "Half", "Half", "1026"))
	writeFile(t, filepath.Join(apps, "appmanifest_5.acf"), acf("5", "No Folder", "Missing", "4"))
	writeFile(t, filepath.Join(apps, "appmanifest_7.acf"), `"AppState" {`)
	if err := os.Mkdir(filepath.Join(apps, "appmanifest_9.acf"), 0o755); err != nil {
		t.Fatal(err)
	}
	steam := scanSteam(context.Background(), root)

	manifests := filepath.Join(t.TempDir(), "Manifests")
	good := t.TempDir()
	writeFile(t, filepath.Join(manifests, "a.item"), epicItemJSON(good, "Unfinished Ns", "ns%3A", "item1", "AppA", "", false, ""))
	writeFile(t, filepath.Join(manifests, "b.item"), epicItemJSON(filepath.Join(good, "missing"), "Gone", "ns2", "item2", "AppB", "", false, ""))
	writeFile(t, filepath.Join(manifests, "c.item"), epicItemJSON(good, "Bad Name", "ns3", "item3", "bad name!", "", false, ""))
	writeFile(t, filepath.Join(manifests, "d.item"), `{"DisplayName": `)
	if err := os.Mkdir(filepath.Join(manifests, "e.item"), 0o755); err != nil {
		t.Fatal(err)
	}
	epic := scanEpic(context.Background(), manifests)

	reg := &fakeRegistry{keys: map[string]map[string]string{
		"k1":  {"gamename": "Gameid Differs", "path": filepath.Join(good, "missing"), "gameid": "555"},
		"77":  {"gamename": "Key As Id", "path": filepath.Join(good, "missing")},
		"k3":  {"gamename": "Bad Gameid", "path": good, "gameid": "abc"},
		"123": {"gamename": "No Exe", "path": good, "gameid": "123"},
	}}
	gog := scanGOG(context.Background(), reg)

	tests := []struct {
		name string
		scan ProviderScan
		want map[string]string
	}{
		{"steam", steam, map[string]string{"Half": "2", "No Folder": "5", "appmanifest_7.acf": "", "appmanifest_9.acf": "", filepath.Join(root, "gone"): ""}},
		{"epic", epic, map[string]string{"Unfinished Ns": "AppA", "Gone": "AppB", "Bad Name": "", "d.item": "", "e.item": ""}},
		{"gog", gog, map[string]string{"Gameid Differs": "555", "Key As Id": "77", "Bad Gameid": "", "No Exe": "123"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.scan.Err != nil {
				t.Fatalf("scan error: %v", tc.scan.Err)
			}
			got := map[string]string{}
			for _, sk := range tc.scan.Skipped {
				got[sk.Title] = sk.AppID
			}
			for title, appID := range tc.want {
				v, ok := got[title]
				if !ok {
					t.Errorf("%q is not in Skipped: %v", title, got)
					continue
				}
				if v != appID {
					t.Errorf("%q: Skipped.AppID = %q, want %q", title, v, appID)
				}
			}
		})
	}
}
