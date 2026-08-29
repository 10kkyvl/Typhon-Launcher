package feed

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestParseValidFeed(t *testing.T) {
	data := []byte(`{
		"name": "Example Source",
		"version": 1,
		"downloads": [
			{"title": "Some Game Deluxe Edition v1.5", "uris": ["magnet:?xt=urn:btih:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"], "uploadDate": "2026-08-20T10:00:00Z", "fileSize": 42949672960}
		]
	}`)

	f, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if f.Name != "Example Source" {
		t.Errorf("Name = %q", f.Name)
	}
	if f.Version != 1 {
		t.Errorf("Version = %d", f.Version)
	}
	if len(f.Entries) != 1 {
		t.Fatalf("Entries len = %d", len(f.Entries))
	}
	e := f.Entries[0]
	if e.Title != "Some Game Deluxe Edition v1.5" {
		t.Errorf("Title = %q", e.Title)
	}
	if e.Size != 42949672960 {
		t.Errorf("Size = %d", e.Size)
	}
	if e.UploadedAt == nil || !e.UploadedAt.Equal(time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)) {
		t.Errorf("UploadedAt = %v", e.UploadedAt)
	}
	if f.Fingerprint == "" {
		t.Error("Fingerprint is empty")
	}
	if f.Invalid != 0 {
		t.Errorf("Invalid = %d", f.Invalid)
	}
}

func TestParseURIVariants(t *testing.T) {
	cases := []string{
		`{"downloads":[{"title":"Game A","uris":["magnet:?xt=urn:btih:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"]}]}`,
		`{"downloads":[{"title":"Game A","uri":"magnet:?xt=urn:btih:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}]}`,
		`{"downloads":[{"title":"Game A","magnet":"magnet:?xt=urn:btih:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}]}`,
		`{"entries":[{"title":"Game A","uri":"magnet:?xt=urn:btih:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}]}`,
		`{"items":[{"title":"Game A","uri":"magnet:?xt=urn:btih:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}]}`,
		`[{"title":"Game A","uri":"magnet:?xt=urn:btih:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}]`,
	}
	for i, c := range cases {
		f, err := Parse([]byte(c))
		if err != nil {
			t.Fatalf("case %d: Parse error: %v", i, err)
		}
		if len(f.Entries) != 1 || len(f.Entries[0].URIs) != 1 {
			t.Fatalf("case %d: got %+v", i, f)
		}
	}
}

func TestParseFileSizeVariants(t *testing.T) {
	cases := []struct {
		size    string
		want    int64
		warn    bool
		unknown bool
	}{
		{`1024`, 1024, false, false},
		{`"1024"`, 1024, false, false},
		{`"42 GB"`, 42_000_000_000, false, false},
		{`"1.5 GiB"`, int64(1.5 * 1024 * 1024 * 1024), false, false},
		{`"700MB"`, 700_000_000, false, false},
		{`"1.5GB"`, 1_500_000_000, false, false},
		{`"garbage"`, 0, true, false},
		{`"lolwut"`, 0, true, false},
		{`"5.6/6.4 GB"`, 6_400_000_000, false, false},
		{`"11,6 GB"`, 11_600_000_000, false, false},
		{`"1,234 MB"`, 1_234_000_000, false, false},
		{`"N/A"`, 0, false, true},
		{`"n/a"`, 0, false, true},
		{`"?"`, 0, false, true},
		{`"unknown"`, 0, false, true},
		{`"неизвестно"`, 0, false, true},
		{`""`, 0, false, true},
		{`null`, 0, false, true},
		{`"~12 GB"`, 12_000_000_000, false, false},
		{`"from 11.6 GB"`, 11_600_000_000, false, false},
		{`"approx. 12 GB"`, 12_000_000_000, false, false},
		{`">12 GB"`, 12_000_000_000, false, false},
		{`"<12 GB"`, 12_000_000_000, false, false},
		{`"5 гб"`, 5_000_000_000, false, false},
	}
	for _, c := range cases {
		data := []byte(`{"downloads":[{"title":"Game A","uri":"magnet:?xt=urn:btih:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA","fileSize":` + c.size + `}]}`)
		f, err := Parse(data)
		if err != nil {
			t.Fatalf("size %s: Parse error: %v", c.size, err)
		}
		if len(f.Entries) != 1 {
			t.Fatalf("size %s: entries = %+v", c.size, f.Entries)
		}
		if f.Entries[0].Size != c.want {
			t.Errorf("size %s: got %d want %d", c.size, f.Entries[0].Size, c.want)
		}
		if f.Entries[0].SizeUnknown != c.unknown {
			t.Errorf("size %s: SizeUnknown = %v, want %v", c.size, f.Entries[0].SizeUnknown, c.unknown)
		}
		if c.warn && len(f.Warnings) == 0 {
			t.Errorf("size %s: expected warning", c.size)
		}
		if c.unknown && len(f.Warnings) != 0 {
			t.Errorf("size %s: expected no warning, got %v", c.size, f.Warnings)
		}
	}
}

func TestParseKeepsOldSizesWorking(t *testing.T) {
	cases := []struct {
		name   string
		fields string
		want   int64
	}{
		{"plain integer", `,"fileSize":1234`, 1234},
		{"quoted integer", `,"fileSize":"1234"`, 1234},
		{"decimal GB", `,"fileSize":"1.5 GB"`, 1_500_000_000},
		{"no-space unit", `,"fileSize":"1.5GB"`, 1_500_000_000},
		{"float json number", `,"fileSize":12345.0`, 12345},
		{"json null", `,"fileSize":null`, 0},
		{"field absent", ``, 0},
	}
	for _, c := range cases {
		data := []byte(`{"downloads":[{"title":"Game A","uri":"magnet:?xt=urn:btih:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"` + c.fields + `}]}`)
		f, err := Parse(data)
		if err != nil {
			t.Fatalf("%s: Parse error: %v", c.name, err)
		}
		if len(f.Entries) != 1 {
			t.Fatalf("%s: entries = %+v", c.name, f.Entries)
		}
		if f.Entries[0].Size != c.want {
			t.Errorf("%s: size = %d, want %d", c.name, f.Entries[0].Size, c.want)
		}
		if len(f.Warnings) != 0 {
			t.Errorf("%s: warnings = %v, want none", c.name, f.Warnings)
		}
	}
}

func TestParseUploadDateVariants(t *testing.T) {
	cases := []struct {
		date    string
		wantNil bool
	}{
		{`"2026-08-20T10:00:00Z"`, false},
		{`"2026-08-20"`, false},
		{`1755680400`, false},
		{`1755680400000`, false},
		{`"not-a-date"`, true},
	}
	for _, c := range cases {
		data := []byte(`{"downloads":[{"title":"Game A","uri":"magnet:?xt=urn:btih:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA","uploadDate":` + c.date + `}]}`)
		f, err := Parse(data)
		if err != nil {
			t.Fatalf("date %s: Parse error: %v", c.date, err)
		}
		if len(f.Entries) != 1 {
			t.Fatalf("date %s: entries = %+v", c.date, f.Entries)
		}
		got := f.Entries[0].UploadedAt
		if c.wantNil && got != nil {
			t.Errorf("date %s: expected nil, got %v", c.date, got)
		}
		if !c.wantNil && got == nil {
			t.Errorf("date %s: expected non-nil", c.date)
		}
	}
}

func TestParseInvalidEntries(t *testing.T) {
	data := []byte(`{"downloads":[
		{"title":"", "uri":"magnet:?xt=urn:btih:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"},
		{"title":"No URI game"},
		{"title":"Non magnet game", "uri":"http://example.com/file.torrent"},
		{"title":"Valid Game", "uri":"magnet:?xt=urn:btih:BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB"},
		{"title":"Valid Game", "uri":"magnet:?xt=urn:btih:BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB"}
	]}`)
	f, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(f.Entries) != 1 {
		t.Fatalf("expected 1 valid entry, got %d: %+v", len(f.Entries), f.Entries)
	}
	if f.Entries[0].Title != "Valid Game" {
		t.Errorf("unexpected surviving entry: %+v", f.Entries[0])
	}
	if f.Invalid != 3 {
		t.Errorf("Invalid = %d, want 3", f.Invalid)
	}
	if len(f.Warnings) == 0 {
		t.Error("expected warnings")
	}
}

func TestParseTooLongTitle(t *testing.T) {
	longTitle := make([]byte, MaxTitleLen+10)
	for i := range longTitle {
		longTitle[i] = 'a'
	}
	raw := map[string]any{
		"downloads": []map[string]any{
			{"title": string(longTitle), "uri": "magnet:?xt=urn:btih:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"},
		},
	}
	data, _ := json.Marshal(raw)
	f, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(f.Entries) != 0 || f.Invalid != 1 {
		t.Errorf("expected entry rejected, got entries=%d invalid=%d", len(f.Entries), f.Invalid)
	}
}

func TestParseVersionErrors(t *testing.T) {
	_, err := Parse([]byte(`{"version":2,"downloads":[{"title":"Game A","uri":"magnet:?xt=urn:btih:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}]}`))
	if !errors.Is(err, ErrUnsupportedVersion) {
		t.Errorf("version 2: got %v, want ErrUnsupportedVersion", err)
	}

	_, err = Parse([]byte(`{"version":0,"downloads":[{"title":"Game A","uri":"magnet:?xt=urn:btih:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}]}`))
	if !errors.Is(err, ErrUnsupportedVersion) {
		t.Errorf("version 0: got %v, want ErrUnsupportedVersion", err)
	}
}

func TestParseInvalidJSON(t *testing.T) {
	_, err := Parse([]byte(`not json at all`))
	if !errors.Is(err, ErrInvalidJSON) {
		t.Errorf("got %v, want ErrInvalidJSON", err)
	}

	_, err = Parse(nil)
	if !errors.Is(err, ErrInvalidJSON) {
		t.Errorf("empty input: got %v, want ErrInvalidJSON", err)
	}
}

func TestParseEmptyFeed(t *testing.T) {
	_, err := Parse([]byte(`{"name":"x","downloads":[]}`))
	if !errors.Is(err, ErrEmptyFeed) {
		t.Errorf("got %v, want ErrEmptyFeed", err)
	}

	_, err = Parse([]byte(`{"name":"x"}`))
	if !errors.Is(err, ErrEmptyFeed) {
		t.Errorf("no section: got %v, want ErrEmptyFeed", err)
	}
}

func TestParseAllEntriesInvalidNoError(t *testing.T) {
	f, err := Parse([]byte(`{"downloads":[{"title":"x"},{"title":"y"}]}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.Entries != nil {
		t.Errorf("expected nil Entries, got %+v", f.Entries)
	}
	if f.Invalid != 2 {
		t.Errorf("Invalid = %d, want 2", f.Invalid)
	}
}

func TestParseMaxURIsPerEntryTruncation(t *testing.T) {
	uris := make([]string, MaxURIsPerEntry+5)
	for i := range uris {
		uris[i] = "magnet:?xt=urn:btih:" + hashOfIndex(i)
	}
	raw := map[string]any{
		"downloads": []map[string]any{
			{"title": "Many URIs", "uris": uris},
		},
	}
	data, _ := json.Marshal(raw)
	f, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(f.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(f.Entries))
	}
	if len(f.Entries[0].URIs) != MaxURIsPerEntry {
		t.Errorf("URIs len = %d, want %d", len(f.Entries[0].URIs), MaxURIsPerEntry)
	}
	if len(f.Warnings) == 0 {
		t.Error("expected truncation warning")
	}
}

func hashOfIndex(i int) string {
	return fmt.Sprintf("%040x", i)
}

func TestParseMaxEntriesTruncation(t *testing.T) {
	downloads := make([]map[string]any, MaxEntries+5)
	for i := range downloads {
		downloads[i] = map[string]any{
			"title": "Game " + hashOfIndex(i),
			"uri":   "magnet:?xt=urn:btih:" + hashOfIndex(i),
		}
	}
	raw := map[string]any{"downloads": downloads}
	data, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	f, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(f.Entries) != MaxEntries {
		t.Errorf("Entries len = %d, want %d", len(f.Entries), MaxEntries)
	}
	found := false
	for _, w := range f.Warnings {
		if w != "" {
			found = true
		}
	}
	if !found {
		t.Error("expected at least one warning")
	}
}

func TestParseDuplicates(t *testing.T) {
	data := []byte(`{"downloads":[
		{"title":"  Same   Game  ", "uri":"magnet:?xt=urn:btih:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"},
		{"title":"Same Game", "uri":"magnet:?xt=urn:btih:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}
	]}`)
	f, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(f.Entries) != 1 {
		t.Errorf("expected 1 entry after dedup, got %d", len(f.Entries))
	}
	if len(f.Warnings) == 0 {
		t.Error("expected duplicate warning")
	}
}

func TestParseHTTPOnlyEntryGetsDistinctWarning(t *testing.T) {
	data := []byte(`{"downloads":[
		{"title":"HTTP Only Game", "uris":["http://example.com/a.zip","https://example.com/b.zip"]},
		{"title":"Valid Game", "uri":"magnet:?xt=urn:btih:BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB"}
	]}`)
	f, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(f.Entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(f.Entries))
	}
	if f.Invalid != 1 {
		t.Fatalf("Invalid = %d, want 1", f.Invalid)
	}
	found := false
	for _, w := range f.Warnings {
		if strings.Contains(w, "доступны только по прямой ссылке") {
			found = true
		}
		if strings.Contains(w, "нет валидного URI") {
			t.Errorf("http-only entry should not be reported as having no URI, got warning %q", w)
		}
	}
	if !found {
		t.Errorf("expected http-only warning, got %v", f.Warnings)
	}
}

func TestParseMixedNonMagnetSchemesStayNoURI(t *testing.T) {
	data := []byte(`{"downloads":[
		{"title":"Weird Scheme Game", "uris":["ed2k://foo","ftp://example.com/a.zip"]}
	]}`)
	f, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(f.Entries) != 0 || f.Invalid != 1 {
		t.Fatalf("entries=%d invalid=%d, want 0 entries and 1 invalid", len(f.Entries), f.Invalid)
	}
	found := false
	for _, w := range f.Warnings {
		if strings.Contains(w, "нет валидного URI") {
			found = true
		}
		if strings.Contains(w, "доступны только по прямой ссылке") {
			t.Errorf("non-http non-magnet entry should not be reported as http-only, got warning %q", w)
		}
	}
	if !found {
		t.Errorf("expected no-URI warning, got %v", f.Warnings)
	}
}

func TestFingerprintOrderIndependent(t *testing.T) {
	f1 := Feed{
		Name:    "n",
		Version: 1,
		Entries: []Entry{
			{Title: "A", URIs: []string{"magnet:?xt=urn:btih:1"}, Size: 10},
			{Title: "B", URIs: []string{"magnet:?xt=urn:btih:2"}, Size: 20},
		},
	}
	f2 := Feed{
		Name:    "n",
		Version: 1,
		Entries: []Entry{
			{Title: "B", URIs: []string{"magnet:?xt=urn:btih:2"}, Size: 20},
			{Title: "A", URIs: []string{"magnet:?xt=urn:btih:1"}, Size: 10},
		},
	}
	if Fingerprint(f1) != Fingerprint(f2) {
		t.Error("fingerprint should be order independent")
	}

	f3 := Feed{
		Name:    "n",
		Version: 1,
		Entries: []Entry{
			{Title: "A", URIs: []string{"magnet:?xt=urn:btih:1"}, Size: 99},
			{Title: "B", URIs: []string{"magnet:?xt=urn:btih:2"}, Size: 20},
		},
	}
	if Fingerprint(f1) == Fingerprint(f3) {
		t.Error("fingerprint should change when size changes")
	}
}
