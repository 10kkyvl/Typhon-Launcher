package feed

import "testing"

func TestParsePatchEntries(t *testing.T) {
	data := []byte(`{
		"name": "Feed",
		"downloads": [
			{"title": "Game 1.3", "uris": ["magnet:?xt=urn:btih:aaaa"], "sequence": 3},
			{
				"title": "Game Update 1.2 -> 1.3",
				"type": "patch",
				"game": "Game",
				"fromVersion": "1.2",
				"toVersion": "1.3",
				"uris": ["magnet:?xt=urn:btih:bbbb"]
			},
			{"title": "Broken patch", "type": "patch", "uris": ["magnet:?xt=urn:btih:cccc"]},
			{"title": "Odd type", "type": "bundle", "uris": ["magnet:?xt=urn:btih:dddd"]}
		]
	}`)

	parsed, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Entries) != 3 {
		t.Fatalf("entries = %d, want 3", len(parsed.Entries))
	}
	if parsed.Invalid != 1 {
		t.Fatalf("invalid = %d, want 1", parsed.Invalid)
	}

	release := parsed.Entries[0]
	if release.Type != TypeRelease || release.Sequence != 3 {
		t.Fatalf("release entry = %+v", release)
	}

	patch := parsed.Entries[1]
	if patch.Type != TypePatch {
		t.Fatalf("type = %q, want %q", patch.Type, TypePatch)
	}
	if patch.FromVersion != "1.2" || patch.ToVersion != "1.3" {
		t.Fatalf("patch versions = %q -> %q", patch.FromVersion, patch.ToVersion)
	}
	if patch.Game != "Game" {
		t.Fatalf("game hint = %q", patch.Game)
	}

	unknown := parsed.Entries[2]
	if unknown.Type != TypeRelease {
		t.Fatalf("unknown type = %q, want it treated as a release", unknown.Type)
	}
}

func TestParseKeepsOldFeedsWorking(t *testing.T) {
	data := []byte(`[{"title": "Game 1.0", "uris": ["magnet:?xt=urn:btih:aaaa"], "fileSize": "12 GB"}]`)
	parsed, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(parsed.Entries))
	}
	entry := parsed.Entries[0]
	if entry.Type != TypeRelease || entry.FromVersion != "" || entry.Sequence != 0 {
		t.Fatalf("entry = %+v", entry)
	}
	if entry.Size != 12_000_000_000 {
		t.Fatalf("size = %d", entry.Size)
	}
}
