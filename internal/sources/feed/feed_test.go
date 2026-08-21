package feed

import (
	"encoding/json"
	"errors"
	"fmt"
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
		size string
		want int64
		warn bool
	}{
		{`1024`, 1024, false},
		{`"1024"`, 1024, false},
		{`"42 GB"`, 42_000_000_000, false},
		{`"1.5 GiB"`, int64(1.5 * 1024 * 1024 * 1024), false},
		{`"700MB"`, 700_000_000, false},
		{`"garbage"`, 0, true},
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
		if c.warn && len(f.Warnings) == 0 {
			t.Errorf("size %s: expected warning", c.size)
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
