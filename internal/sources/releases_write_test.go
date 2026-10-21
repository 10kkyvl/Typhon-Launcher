package sources

import (
	"bytes"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"testing"
	"time"

	"typhon/internal/catalog"
)

var longAgo = time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)

func ageFile(t *testing.T, path string) {
	t.Helper()
	if err := os.Chtimes(path, longAgo, longAgo); err != nil {
		t.Fatalf("age %s: %v", path, err)
	}
}

func modTime(t *testing.T, path string) time.Time {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return info.ModTime()
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

func matchedFeed(t *testing.T, size int64) (string, *cachingTestRemote) {
	t.Helper()
	body := feedBody(t, "Source",
		feedEntry{DistributionID: "a", Title: "Alpha Game v1.0", URIs: []string{magnetOf("aa")}, FileSize: size},
		feedEntry{DistributionID: "b", Title: "Beta Game v1.0", URIs: []string{magnetOf("bb")}, FileSize: 20},
		feedEntry{DistributionID: "c", Title: "Unknown Thing v1.0", URIs: []string{magnetOf("cc")}, FileSize: 30},
	)
	remote := &cachingTestRemote{games: map[string]catalog.Game{
		"Alpha Game": {ID: "server-alpha", Title: "Alpha Game", ExternalIDs: catalog.ExternalIDs{IGDB: "1"}},
		"Beta Game":  {ID: "server-beta", Title: "Beta Game", ExternalIDs: catalog.ExternalIDs{IGDB: "2"}},
	}}
	return body, remote
}

func requireMemoryMatchesDisk(t *testing.T, s *Service, id string) {
	t.Helper()
	disk, err := s.store.loadReleases(id)
	if err != nil {
		t.Fatalf("load releases: %v", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	memory := s.releases[id]
	if len(disk) != len(memory) {
		t.Fatalf("disk holds %d releases, memory %d", len(disk), len(memory))
	}
	for i := range memory {
		if !sameRelease(disk[i], memory[i]) {
			t.Fatalf("release %d differs between memory and disk:\nmemory %+v\ndisk   %+v", i, *memory[i], *disk[i])
		}
	}
}

func TestRefreshWithoutChangesLeavesReleasesFileAlone(t *testing.T) {
	cases := []struct {
		name  string
		nudge func(server *feedServer, body string)
	}{
		{"feed not modified", func(*feedServer, string) {}},
		{"feed sent again with the same content", func(server *feedServer, body string) { server.set(body, `"v2"`) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, cat, _ := testService(t)
			body, remote := matchedFeed(t, 10)
			cat.SetRemoteCatalog(remote)
			server := newFeedServer(t, body)
			src := addSource(t, s, server.url())
			path := s.store.releasesPath(src.ID)
			before := readFile(t, path)
			ageFile(t, path)

			tc.nudge(server, body)
			summary, err := s.RefreshSource(src.ID)
			if err != nil {
				t.Fatalf("refresh: %v", err)
			}
			if summary.Added+summary.Updated+summary.Removed+summary.Restored != 0 {
				t.Fatalf("summary = %+v, want no changes", summary)
			}

			if got := modTime(t, path); !got.Equal(longAgo) {
				t.Fatalf("releases file rewritten: mtime %v, want untouched %v", got, longAgo)
			}
			if !bytes.Equal(before, readFile(t, path)) {
				t.Fatal("releases file content changed")
			}
			requireMemoryMatchesDisk(t, s, src.ID)
		})
	}
}

func TestRefreshWithOneChangedEntryRewritesReleasesFile(t *testing.T) {
	s, cat, _ := testService(t)
	body, remote := matchedFeed(t, 10)
	cat.SetRemoteCatalog(remote)
	server := newFeedServer(t, body)
	src := addSource(t, s, server.url())
	path := s.store.releasesPath(src.ID)
	before := readFile(t, path)
	ageFile(t, path)

	changed, _ := matchedFeed(t, 777)
	server.set(changed, `"v2"`)
	summary, err := s.RefreshSource(src.ID)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if summary.Updated != 1 {
		t.Fatalf("summary = %+v, want exactly one updated entry", summary)
	}

	if got := modTime(t, path); got.Equal(longAgo) {
		t.Fatal("releases file not rewritten after an entry changed")
	}
	after := readFile(t, path)
	if bytes.Equal(before, after) || !bytes.Contains(after, []byte(`"size": 777`)) {
		t.Fatalf("new size missing from the releases file:\n%s", after)
	}
	requireMemoryMatchesDisk(t, s, src.ID)
}

func TestRefreshWithNewServerAnswerRewritesReleasesFile(t *testing.T) {
	s, cat, _ := testService(t)
	body, remote := matchedFeed(t, 10)
	cat.SetRemoteCatalog(remote)
	server := newFeedServer(t, body)
	src := addSource(t, s, server.url())
	path := s.store.releasesPath(src.ID)

	s.mu.Lock()
	for _, r := range s.releases[src.ID] {
		r.RemoteMatch.At = time.Now().Add(-remoteMatchKnownTTL - remoteMatchKnownJitterMax - time.Hour)
	}
	s.mu.Unlock()
	ageFile(t, path)
	if _, err := s.RefreshSource(src.ID); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if got := modTime(t, path); got.Equal(longAgo) {
		t.Fatal("a renewed server answer carries a new timestamp and must be saved")
	}
	requireMemoryMatchesDisk(t, s, src.ID)
}

func TestRefreshWritesSourcesFileEvenWhenReleasesAreUnchanged(t *testing.T) {
	s, cat, _ := testService(t)
	body, remote := matchedFeed(t, 10)
	cat.SetRemoteCatalog(remote)
	server := newFeedServer(t, body)
	src := addSource(t, s, server.url())
	releases, sources := s.store.releasesPath(src.ID), s.store.sourcesPath()
	ageFile(t, releases)
	ageFile(t, sources)

	later := time.Now().Add(time.Hour).Truncate(time.Second)
	s.now = func() time.Time { return later }
	if _, err := s.RefreshSource(src.ID); err != nil {
		t.Fatalf("refresh: %v", err)
	}

	if !modTime(t, releases).Equal(longAgo) {
		t.Fatal("releases file rewritten")
	}
	if modTime(t, sources).Equal(longAgo) {
		t.Fatal("sources file must record the refresh time")
	}
	got, err := s.GetSource(src.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.LastUpdatedAt == nil || !got.LastUpdatedAt.Equal(later) {
		t.Fatalf("LastUpdatedAt = %v, want %v", got.LastUpdatedAt, later)
	}
	stored, err := s.store.loadSources()
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 || stored[0].LastUpdatedAt == nil || !stored[0].LastUpdatedAt.Equal(later) {
		t.Fatalf("stored sources = %+v, want the refresh time on disk", stored)
	}
}

func TestReparsedFieldsReachTheDiskWhenTheFeedIsNotModified(t *testing.T) {
	s, cat, _ := testService(t)
	body, remote := matchedFeed(t, 10)
	cat.SetRemoteCatalog(remote)
	server := newFeedServer(t, body)
	src := addSource(t, s, server.url())
	path := s.store.releasesPath(src.ID)
	ageFile(t, path)

	s.mu.Lock()
	s.releases[src.ID][0].Edition = "left by an older parser"
	s.mu.Unlock()
	if _, err := s.RefreshSource(src.ID); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if modTime(t, path).Equal(longAgo) {
		t.Fatal("a field fixed by the reparse never reached the file")
	}
	disk, err := s.store.loadReleases(src.ID)
	if err != nil {
		t.Fatal(err)
	}
	if disk[0].Edition != "" {
		t.Fatalf("disk edition = %q, want the reparsed value", disk[0].Edition)
	}
	requireMemoryMatchesDisk(t, s, src.ID)
}

func TestFailedReleasesSaveKeepsMemoryAtWhatIsOnDisk(t *testing.T) {
	s, cat, _ := testService(t)
	body, remote := matchedFeed(t, 10)
	cat.SetRemoteCatalog(remote)
	server := newFeedServer(t, body)
	src := addSource(t, s, server.url())
	path := s.store.releasesPath(src.ID)

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	changed, _ := matchedFeed(t, 999)
	server.set(changed, `"v2"`)
	if _, err := s.RefreshSource(src.ID); err == nil {
		t.Fatal("a failed save must be reported")
	}
	s.mu.Lock()
	size := s.releases[src.ID][0].Size
	s.mu.Unlock()
	if size != 10 {
		t.Fatalf("memory holds size %d that never reached the disk, want 10", size)
	}
}

func fillValue(v reflect.Value, variant uint8) {
	switch v.Kind() {
	case reflect.String:
		v.SetString("s" + strconv.Itoa(int(variant)))
	case reflect.Int, reflect.Int64:
		v.SetInt(int64(variant))
	case reflect.Uint64:
		v.SetUint(uint64(variant))
	case reflect.Float64:
		v.SetFloat(float64(variant))
	case reflect.Bool:
		v.SetBool(variant == 1)
	case reflect.Slice:
		v.Set(reflect.ValueOf([]string{"v" + strconv.Itoa(int(variant))}))
	case reflect.Pointer:
		p := reflect.New(v.Type().Elem())
		fillValue(p.Elem(), variant)
		v.Set(p)
	case reflect.Struct:
		if v.Type() == reflect.TypeOf(time.Time{}) {
			v.Set(reflect.ValueOf(time.Unix(int64(variant)*1000, 0)))
			return
		}
		for i := range v.NumField() {
			fillValue(v.Field(i), variant)
		}
	default:
		panic(fmt.Sprintf("fillValue: unsupported kind %s", v.Kind()))
	}
}

// A new Release field that sameRelease forgets would never be saved by a
// refresh that changes nothing else.
func TestSameReleaseComparesEveryPersistedField(t *testing.T) {
	typ := reflect.TypeOf(Release{})
	for i := range typ.NumField() {
		name := typ.Field(i).Name
		t.Run(name, func(t *testing.T) {
			var base, other Release
			fillValue(reflect.ValueOf(&base).Elem(), 1)
			fillValue(reflect.ValueOf(&other).Elem(), 1)
			fillValue(reflect.ValueOf(&other).Elem().Field(i), 2)
			same := sameRelease(&base, &other)
			if name == "LastSeenAt" {
				if !same {
					t.Fatal("LastSeenAt moves on every pass and must not count as a change")
				}
				return
			}
			if same {
				t.Fatalf("a different %s went unnoticed", name)
			}
		})
	}
}

func TestSameRemoteMatchComparesEveryField(t *testing.T) {
	typ := reflect.TypeOf(RemoteMatch{})
	for i := range typ.NumField() {
		name := typ.Field(i).Name
		t.Run(name, func(t *testing.T) {
			var base, other RemoteMatch
			fillValue(reflect.ValueOf(&base).Elem(), 1)
			fillValue(reflect.ValueOf(&other).Elem(), 1)
			fillValue(reflect.ValueOf(&other).Elem().Field(i), 2)
			if sameRemoteMatch(&base, &other) {
				t.Fatalf("a different %s went unnoticed", name)
			}
		})
	}
}
