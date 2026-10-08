package download

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
)

func jsonKeys(t *testing.T, v any) []string {
	t.Helper()
	body, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		t.Fatal(err)
	}
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

func fullDownload() Download {
	at := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	flag := true
	return Download{
		Files: []FileState{{}}, Flat: true, InPlace: true, CompletedAt: &at, StalledSince: &at,
		Origin: Origin{
			ReleaseID: "r", SourceID: "s", DistributionID: "d", ReleaseUploadedAt: &at, GameID: "g", Version: "v",
			Purpose: PurposeUpdate, UpdatePlanID: "p", LibraryID: "l", AutoInstall: &flag, ElevateAhead: true,
		},
	}
}

func TestPayloadsKeepTheirJSONKeys(t *testing.T) {
	cases := []struct {
		name  string
		value any
		want  []string
	}{
		{"Download", fullDownload(), []string{
			"addedAt", "completedAt", "destination", "downloadSpeed", "downloaded", "error", "etaSeconds", "files", "flat",
			"id", "inPlace", "infoHash", "name", "origin", "peers", "progress", "seeders", "seeding", "source",
			"stalled", "stalledSince", "status", "total", "type", "uploadSpeed",
		}},
		{"Origin", fullDownload().Origin, []string{
			"autoInstall", "distributionId", "elevateAhead", "gameId", "libraryId", "purpose", "releaseId",
			"releaseUploadedAt", "sourceId", "updatePlanId", "version",
		}},
		{"FileState", FileState{}, []string{"bytesDone", "path", "selected", "size"}},
		{"ProgressUpdate", func() ProgressUpdate {
			at := time.Now()
			return ProgressUpdate{StalledSince: &at}
		}(), []string{
			"downloadSpeed", "downloaded", "etaSeconds", "id", "peers", "progress", "seeders", "stalled", "stalledSince", "status", "uploadSpeed",
		}},
		{"RemovedEvent", RemovedEvent{}, []string{"id"}},
		{"TorrentInfo", TorrentInfo{}, []string{"files", "infoHash", "name", "totalBytes"}},
		{"NetworkState", NetworkState{}, []string{"address", "code", "mode", "reason", "state", "warning"}},
		{"NetInterface", NetInterface{}, []string{"addresses", "description", "name", "up", "vpnLike"}},
		{"degradedStatus", degradedStatus{}, []string{"degraded", "message"}},
		{"record", func() record {
			at := time.Now()
			return record{Flat: true, InPlace: true, CompletedAt: &at}
		}(), []string{
			"addedAt", "completedAt", "destination", "downloaded", "error", "flat", "id", "inPlace", "infoHash",
			"name", "origin", "seeding", "selected", "source", "status", "total", "type",
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := jsonKeys(t, c.value); !slices.Equal(got, c.want) {
				t.Fatalf("keys = %v\nwant   %v\nthe interface and the stored downloads.json read these names", got, c.want)
			}
		})
	}
}

func TestStatusAndKindLiteralsAreStable(t *testing.T) {
	got := map[string]string{
		"queued": string(StatusQueued), "metadata": string(StatusMetadata), "downloading": string(StatusDownloading),
		"paused": string(StatusPaused), "verifying": string(StatusVerifying), "completed": string(StatusCompleted),
		"failed": string(StatusFailed), "torrent": string(TypeTorrent), "update": string(PurposeUpdate),
		"repair": string(PurposeRepair), "ok": NetworkOK, "down": NetworkDown,
	}
	for want, have := range got {
		if want != have {
			t.Errorf("literal %q became %q", want, have)
		}
	}
	if PurposeRelease != "" {
		t.Errorf("the purpose of a plain download = %q, must stay empty so that old records keep reading as releases", PurposeRelease)
	}
}

var (
	tsInterfacePattern = regexp.MustCompile(`(?s)export interface (\w+) \{(.*?)\n\}`)
	tsFieldPattern     = regexp.MustCompile(`(?m)^\s+(\w+)\??:`)
	tsUnionPattern     = regexp.MustCompile(`(?s)export type DownloadStatus =(.*?);`)
	tsLiteralPattern   = regexp.MustCompile(`'([a-z]+)'`)
)

func tsInterfaces(t *testing.T, rel string) map[string][]string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("..", "..", "frontend", "src", "lib", rel))
	if err != nil {
		t.Fatalf("frontend file: %v", err)
	}
	out := map[string][]string{}
	for _, m := range tsInterfacePattern.FindAllStringSubmatch(string(body), -1) {
		var fields []string
		for _, f := range tsFieldPattern.FindAllStringSubmatch(m[2], -1) {
			fields = append(fields, f[1])
		}
		slices.Sort(fields)
		out[m[1]] = fields
	}
	return out
}

func subset(have, of []string) []string {
	var missing []string
	for _, k := range have {
		if !slices.Contains(of, k) {
			missing = append(missing, k)
		}
	}
	return missing
}

func TestInterfaceTypesAreCoveredByWhatGoSends(t *testing.T) {
	services := tsInterfaces(t, filepath.Join("services", "downloads.ts"))
	network := tsInterfaces(t, filepath.Join("services", "network.ts"))
	store := tsInterfaces(t, filepath.Join("stores", "downloads.ts"))

	checks := []struct {
		name  string
		ts    []string
		value any
		exact bool
	}{
		{"Download", services["Download"], fullDownload(), false},
		{"DownloadOrigin", services["DownloadOrigin"], fullDownload().Origin, false},
		{"FileState", services["FileState"], FileState{}, true},
		{"TorrentInfo", services["TorrentInfo"], TorrentInfo{}, true},
		{"NetworkState", network["NetworkState"], NetworkState{}, true},
		{"NetInterface", network["NetInterface"], NetInterface{}, true},
		{"DownloadProgress", store["DownloadProgress"], func() ProgressUpdate {
			at := time.Now()
			return ProgressUpdate{StalledSince: &at}
		}(), true},
	}
	for _, c := range checks {
		t.Run(c.name, func(t *testing.T) {
			if len(c.ts) == 0 {
				t.Fatalf("interface %s not found in the frontend", c.name)
			}
			goKeys := jsonKeys(t, c.value)
			if missing := subset(c.ts, goKeys); len(missing) > 0 {
				t.Fatalf("the interface reads %v, which Go does not send", missing)
			}
			if c.exact {
				if extra := subset(goKeys, c.ts); len(extra) > 0 {
					t.Fatalf("Go sends %v, which the interface does not declare", extra)
				}
			}
		})
	}

	t.Run("DownloadStatus", func(t *testing.T) {
		body, err := os.ReadFile(filepath.Join("..", "..", "frontend", "src", "lib", "services", "downloads.ts"))
		if err != nil {
			t.Fatal(err)
		}
		block := tsUnionPattern.FindStringSubmatch(string(body))
		if block == nil {
			t.Fatal("DownloadStatus not found in the frontend")
		}
		var ts []string
		for _, m := range tsLiteralPattern.FindAllStringSubmatch(block[1], -1) {
			ts = append(ts, m[1])
		}
		slices.Sort(ts)
		var goStatuses []string
		for _, s := range allStatuses {
			goStatuses = append(goStatuses, string(s))
		}
		slices.Sort(goStatuses)
		if !slices.Equal(ts, goStatuses) {
			t.Fatalf("frontend statuses %v, Go statuses %v", ts, goStatuses)
		}
	})
}

func TestEventNamesMatchWhatTheFrontendListensTo(t *testing.T) {
	events := map[string]string{
		"download:added": eventAdded, "download:updated": eventUpdated, "download:progress": eventProgress,
		"download:completed": eventCompleted, "download:failed": eventFailed, "download:removed": eventRemoved,
		"download:degraded": eventDegraded, "download:network": eventNetwork,
	}
	var listened strings.Builder
	for _, rel := range []string{"downloads.ts", "degraded.ts", "network.ts"} {
		body, err := os.ReadFile(filepath.Join("..", "..", "frontend", "src", "lib", "stores", rel))
		if err != nil {
			t.Fatal(err)
		}
		listened.Write(body)
	}
	for literal, constant := range events {
		if constant != literal {
			t.Errorf("event constant is %q, want %q", constant, literal)
		}
		if !strings.Contains(listened.String(), "'"+literal+"'") {
			t.Errorf("no store of the frontend listens to %q", literal)
		}
	}
}

func TestDownloadsFileOfEarlierVersionsStillLoads(t *testing.T) {
	legacy := `[
  {
    "id": "old1",
    "name": "Old Game",
    "source": "magnet:?xt=urn:btih:aaaa",
    "infoHash": "aaaa",
    "destination": "D:\\Games",
    "status": "completed",
    "selected": [0, 2],
    "downloaded": 500,
    "total": 500,
    "seeding": true,
    "addedAt": "2026-01-02T03:04:05Z",
    "completedAt": "2026-01-02T04:00:00Z",
    "error": ""
  },
  {
    "id": "new1",
    "name": "Newer Game",
    "type": "torrent",
    "source": "magnet:?xt=urn:btih:bbbb",
    "infoHash": "bbbb",
    "destination": "D:\\Games",
    "status": "downloading",
    "selected": null,
    "downloaded": 10,
    "total": 100,
    "seeding": false,
    "flat": true,
    "inPlace": true,
    "origin": {"releaseId": "r1", "purpose": "repair", "autoInstall": false, "fromTheFuture": 1},
    "addedAt": "2026-02-03T03:04:05Z",
    "completedAt": null,
    "error": "",
    "aFieldOfAFutureVersion": {"x": 1}
  }
]`
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "downloads.json"), []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	m := mustManagerAt(t, dir)
	m.mu.Lock()
	err := m.loadLocked()
	m.mu.Unlock()
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	old := mustGet(t, m, "old1")
	if old.Type != TypeTorrent {
		t.Errorf("type of a record written before types existed = %q, want %q", old.Type, TypeTorrent)
	}
	if old.Status != StatusCompleted || !old.Seeding || old.Name != "Old Game" || old.Downloaded != 500 || old.Total != 500 || old.Progress != 1 {
		t.Errorf("old record = %+v", old)
	}
	if old.Flat || old.InPlace || !reflect.DeepEqual(old.Origin, Origin{}) {
		t.Errorf("flags and origin of an old record = %v %v %+v, want zero values", old.Flat, old.InPlace, old.Origin)
	}
	if old.CompletedAt == nil || !old.CompletedAt.Equal(time.Date(2026, 1, 2, 4, 0, 0, 0, time.UTC)) {
		t.Errorf("completedAt = %v", old.CompletedAt)
	}

	newer := mustGet(t, m, "new1")
	if newer.Status != StatusQueued || !newer.Flat || !newer.InPlace || newer.Origin.Purpose != PurposeRepair {
		t.Errorf("newer record = %+v", newer)
	}
	if newer.Origin.AutoInstall == nil || *newer.Origin.AutoInstall {
		t.Errorf("an explicit auto-install = false must survive: %v", newer.Origin.AutoInstall)
	}
	if newer.CompletedAt != nil {
		t.Errorf("completedAt = %v, want nil", newer.CompletedAt)
	}

	m.mu.Lock()
	err = m.persistLocked()
	m.mu.Unlock()
	if err != nil {
		t.Fatalf("persist after load: %v", err)
	}
	records := persistedRecords(t, m)
	if len(records) != 2 || records[0].ID != "old1" || records[1].ID != "new1" {
		t.Fatalf("records after a rewrite = %+v", records)
	}
}
