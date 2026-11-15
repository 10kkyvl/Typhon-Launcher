package download

import (
	"encoding/json"
	"io/fs"
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
			Platform: "ps2", Title: "t", Cover: "c",
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
			"id", "ignoreSchedule", "inPlace", "infoHash", "name", "origin", "peers", "progress", "seeders", "seeding", "source",
			"stalled", "stalledSince", "status", "total", "type", "uploadSpeed",
		}},
		{"Origin", fullDownload().Origin, []string{
			"autoInstall", "cover", "distributionId", "elevateAhead", "gameId", "libraryId", "platform", "purpose", "releaseId",
			"releaseUploadedAt", "sourceId", "title", "updatePlanId", "version",
		}},
		{"FileState", FileState{}, []string{"bytesDone", "path", "selected", "size"}},
		{"ProgressUpdate", func() ProgressUpdate {
			at := time.Now()
			return ProgressUpdate{StalledSince: &at}
		}(), []string{
			"downloadSpeed", "downloaded", "etaSeconds", "id", "peers", "progress", "seeders", "stalled", "stalledSince", "status", "uploadSpeed",
		}},
		{"RemovedEvent", RemovedEvent{}, []string{"id"}},
		{"ReorderedEvent", ReorderedEvent{}, []string{"ids"}},
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
		{"DownloadReordered", store["DownloadReordered"], ReorderedEvent{}, true},
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

// unregisteredEvents names the package's events main.go does not register,
// with the reason; the frontend still has to listen to each of them.
var unregisteredEvents = map[string]string{
	"download:degraded":       "its payload type degradedStatus is unexported",
	"downloads:play-throttle": "the frontend types its payload by hand",
}

var registeredEventPattern = regexp.MustCompile(`RegisterEvent\[.+?\]\(\s*(?:"([^"]+)"|download\.(\w+))\s*\)`)

// packageEvents maps every event constant of the package, in any of its
// files, to its name.
func packageEvents(t *testing.T) map[string]string {
	t.Helper()
	top, ok := parseDownloadTree(t)["."]
	if !ok {
		t.Fatal("no file of the package parsed")
	}
	events := map[string]string{}
	for name, value := range stringConsts(top.files) {
		if strings.HasPrefix(name, "event") && strings.HasPrefix(value, "download") {
			events[name] = value
		}
	}
	return events
}

// frontendSources is the text of every non-test script and component of
// the frontend, where a listener names the event it subscribes to.
func frontendSources(t *testing.T) string {
	t.Helper()
	var all strings.Builder
	src := os.DirFS(filepath.Join("..", "..", "frontend", "src"))
	err := fs.WalkDir(src, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		name := d.Name()
		script := strings.HasSuffix(name, ".ts") || strings.HasSuffix(name, ".svelte")
		if !script || strings.Contains(name, ".test.") || strings.Contains(name, ".spec.") {
			return nil
		}
		body, err := fs.ReadFile(src, path)
		if err != nil {
			return err
		}
		all.Write(body)
		return nil
	})
	if err != nil {
		t.Fatalf("frontend not read: %v", err)
	}
	return all.String()
}

func TestEventNamesMatchWhatTheFrontendListensTo(t *testing.T) {
	events := packageEvents(t)
	if len(events) < 10 {
		t.Fatalf("found %d event constants in the package, want at least 10: %v", len(events), events)
	}
	mainGo, err := os.ReadFile(filepath.Join("..", "..", "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	registered := map[string]bool{}
	for _, m := range registeredEventPattern.FindAllStringSubmatch(string(mainGo), -1) {
		switch {
		case m[1] != "":
			registered[m[1]] = true
		case events[m[2]] != "":
			registered[events[m[2]]] = true
		}
	}
	listened := frontendSources(t)

	names := map[string]bool{}
	for constant, name := range events {
		names[name] = true
		_, exempt := unregisteredEvents[name]
		if !registered[name] && !exempt {
			t.Errorf("%s = %q is not registered in main.go: register it with its payload type, or name it in unregisteredEvents with the reason", constant, name)
		}
		if registered[name] && exempt {
			t.Errorf("%q is registered in main.go: drop it from unregisteredEvents", name)
		}
		if !strings.Contains(listened, "'"+name+"'") {
			t.Errorf("%s = %q: nothing in the frontend listens to it", constant, name)
		}
	}
	for name := range unregisteredEvents {
		if !names[name] {
			t.Errorf("unregisteredEvents names %q, which is no event of the package", name)
		}
	}
	for name := range registered {
		if (strings.HasPrefix(name, "download:") || strings.HasPrefix(name, "downloads:")) && !names[name] {
			t.Errorf("main.go registers %q, which is no event constant of the package", name)
		}
	}
}

// The frontend sorts its list by this event.
func TestReorderEventWireFormat(t *testing.T) {
	m := newTestManager(t, 1)
	m.addTestItem("a", StatusQueued)
	m.addTestItem("b", StatusQueued)
	log := recordEmits(t)
	if err := m.MoveTo("b", 0); err != nil {
		t.Fatal(err)
	}
	reorders := log.reorders()
	if len(reorders) != 1 {
		t.Fatalf("reorder events = %+v", reorders)
	}
	body, err := json.Marshal(reorders[0])
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(body), `{"ids":["b","a"]}`; got != want {
		t.Fatalf("payload = %s, want %s", got, want)
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
