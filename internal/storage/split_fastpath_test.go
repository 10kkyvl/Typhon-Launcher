package storage

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
)

// splitOriginal is a verbatim copy of split()'s implementation from before
// fastSplitEnvelope existed. Unlike splitByCopyingEnvelope (which only
// reproduces the envelope-decode half), this is the complete original
// function, malformed-document and legacy fallback included, so it is a
// faithful oracle for every input - valid or not - split() is required to
// keep matching exactly.
func splitOriginal(raw []byte) (json.RawMessage, int, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, 0, errors.New("empty document")
	}
	var envelope doc
	if err := json.Unmarshal(trimmed, &envelope); err != nil {
		if trimmed[0] != '[' && trimmed[0] != '{' {
			return nil, 0, fmt.Errorf("unexpected document root: %w", err)
		}
		if !json.Valid(trimmed) {
			return nil, 0, fmt.Errorf("malformed document: %w", err)
		}
		return json.RawMessage(trimmed), 0, nil
	}
	if envelope.Version == 0 || envelope.Data == nil {
		return json.RawMessage(trimmed), 0, nil
	}
	return envelope.Data, envelope.Version, nil
}

// TestSplitFastPathMatchesSlowPath locks fastSplitEnvelope's hand-rolled byte
// scan to the exact same (payload, version) pairs the original
// encoding/json-based decode (splitByCopyingEnvelope) produces, across shapes
// the scanner has to get right without ever calling encoding/json itself:
// reversed key order, non-array/object data, escaped and non-ASCII string
// content, and the "give up, let the slow path handle it" cases.
func TestSplitFastPathMatchesSlowPath(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"array data", `{"version":3,"data":[1,2,3]}`},
		{"object data", `{"version":7,"data":{"a":1,"b":2}}`},
		{"reversed key order", `{"data":[1,2,3],"version":9}`},
		{"null data", `{"version":2,"data":null}`},
		{"string data", `{"version":1,"data":"hello"}`},
		{"number data", `{"version":1,"data":42}`},
		{"bool data", `{"version":1,"data":true}`},
		{"escaped quote and brace in string", `{"version":1,"data":"he said \"hi\" and a brace { not real"}`},
		{"non-ascii content", `{"version":1,"data":["Первый","Второй"]}`},
		{"nested arrays and objects", `{"version":1,"data":[{"a":[1,2,{"b":3}]},[4,5]]}`},
		{"whitespace between tokens", "{\n  \"version\" : 1 ,\n  \"data\" : [1, 2]\n}\n"},
		{"pretty printed", "{\n  \"version\": 4,\n  \"data\": [\n    1,\n    2\n  ]\n}"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			trimmed := bytes.TrimSpace([]byte(tc.raw))
			wantPayload, wantVersion, wantErr := splitByCopyingEnvelope(trimmed)
			if wantErr != nil {
				t.Fatalf("reference split: %v", wantErr)
			}
			gotPayload, gotVersion, gotErr := split([]byte(tc.raw))
			if gotErr != nil {
				t.Fatalf("split: %v", gotErr)
			}
			if gotVersion != wantVersion {
				t.Fatalf("version = %d, want %d", gotVersion, wantVersion)
			}
			if !bytes.Equal(gotPayload, wantPayload) {
				t.Fatalf("payload = %q, want %q", gotPayload, wantPayload)
			}
		})
	}
}

// TestSplitFastPathDefersToSlowPathOnAmbiguity checks that shapes the fast
// scanner refuses to touch still resolve exactly like the reference
// (copying) decode, by falling through to the original code path unchanged.
func TestSplitFastPathDefersToSlowPathOnAmbiguity(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"extra unknown key", `{"version":1,"data":[1],"extra":true}`},
		{"duplicate version key", `{"version":1,"version":2,"data":[1]}`},
		{"duplicate data key", `{"version":1,"data":[1],"data":[2]}`},
		{"version zero explicit", `{"version":0,"data":[1,2,3]}`},
		{"version missing", `{"data":[1,2,3]}`},
		{"data missing", `{"version":1}`},
		{"empty object", `{}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			trimmed := bytes.TrimSpace([]byte(tc.raw))
			wantPayload, wantVersion, wantErr := splitByCopyingEnvelope(trimmed)
			gotPayload, gotVersion, gotErr := split([]byte(tc.raw))
			if (wantErr == nil) != (gotErr == nil) {
				t.Fatalf("split err = %v, reference err = %v", gotErr, wantErr)
			}
			if wantErr != nil {
				return
			}
			if gotVersion != wantVersion {
				t.Fatalf("version = %d, want %d", gotVersion, wantVersion)
			}
			if !bytes.Equal(gotPayload, wantPayload) {
				t.Fatalf("payload = %q, want %q", gotPayload, wantPayload)
			}
		})
	}
}

// TestSplitTreatsFieldTypeMismatchAsLegacyDocument pins a pre-existing,
// slow-path-only behavior that fastSplitEnvelope must keep deferring to
// unchanged: when "version" doesn't decode as an int, the original
// json.Unmarshal(trimmed, &envelope) call fails, and split() falls back to
// treating the *entire* document as a version-0 legacy payload (it never
// surfaces the type-mismatch error) as long as the document is otherwise
// valid JSON. splitByCopyingEnvelope, unlike split(), does not implement
// that fallback, so this case is asserted directly against the documented
// behavior instead of against that helper.
func TestSplitTreatsFieldTypeMismatchAsLegacyDocument(t *testing.T) {
	raw := []byte(`{"version":"oops","data":[1,2,3]}`)
	if _, _, ok := fastSplitEnvelope(bytes.TrimSpace(raw)); ok {
		t.Fatal("fastSplitEnvelope should defer on a non-integer version")
	}
	payload, version, err := split(raw)
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	if version != 0 {
		t.Fatalf("version = %d, want 0", version)
	}
	if string(payload) != string(raw) {
		t.Fatalf("payload = %q, want the whole document %q", payload, raw)
	}
}

// TestSplitMatchesOriginalOnInvalidJSON is a differential test: it compares
// split() against splitOriginal (the exact pre-optimization implementation)
// on inputs that are NOT valid JSON but still happen to have balanced
// brackets and correctly-toggling string quotes - the one thing
// fastSplitEnvelope's byte scanner checks on its own. The scanner has no
// notion of JSON grammar beyond that (e.g. it never verifies that a value is
// followed by "," or "}"), so before this test it could return ok=true with
// a truncated/garbage payload for a document splitOriginal would reject
// outright as malformed. That divergence matters because the returned
// version selects which migration chain runs (invariant I.3): the same
// broken file must fail identically regardless of which parser touches it
// first, not run migrations from a different starting point depending on
// where exactly the corruption sits.
func TestSplitMatchesOriginalOnInvalidJSON(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"adjacent string literals, no comma", `{"version":3,"data":{"a":""data""}}`},
		{"adjacent string literals, reversed key order", `{"data":{"a":""data""},"version":3}`},
		{"adjacent string literals, extra whitespace", `{"version": 3, "data": {"a": ""data""}}`},
		{"trailing comma in array", `{"version":1,"data":[1,2,]}`},
		{"trailing comma in object", `{"version":1,"data":{"a":1,}}`},
		{"mismatched bracket types", `{"version":1,"data":[1,2,3}}`},
		{"mismatched bracket types other way", `{"version":1,"data":{"a":1]}`},
		{"trailing garbage after valid envelope", `{"version":1,"data":[1,2,3]}garbage`},
		{"two top-level values", `{"version":1,"data":[1]}{"extra":true}`},
		{"unquoted key inside data", `{"version":1,"data":{a:1}}`},
		{"single quotes instead of double", `{"version":1,"data":{'a':1}}`},
		{"raw control character in string", "{\"version\":1,\"data\":\"line1\nline2\"}"},
		{"missing colon inside data", `{"version":1,"data":{"a" 1}}`},
		{"colon where comma belongs", `{"version":1,"data":[1:2]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wantPayload, wantVersion, wantErr := splitOriginal([]byte(tc.raw))
			gotPayload, gotVersion, gotErr := split([]byte(tc.raw))
			if (wantErr == nil) != (gotErr == nil) {
				t.Fatalf("split err = %v, want err presence = %v (original: %v)", gotErr, wantErr != nil, wantErr)
			}
			if wantErr != nil {
				return
			}
			if gotVersion != wantVersion {
				t.Fatalf("version = %d, want %d", gotVersion, wantVersion)
			}
			if !bytes.Equal(gotPayload, wantPayload) {
				t.Fatalf("payload = %q, want %q", gotPayload, wantPayload)
			}
		})
	}
}

// TestFastSplitEnvelopeRejectsTruncatedInput checks that a truncated "data"
// value makes the scanner give up (ok=false) rather than read out of bounds
// or return a partial slice, so split() falls back to the exact original
// malformed-document handling.
func TestFastSplitEnvelopeRejectsTruncatedInput(t *testing.T) {
	cases := []string{
		`{"version":1,"data":[`,
		`{"version":1,"data":"unterminated`,
		`{"version":1,"data":{"a":1`,
		`{"version":1`,
		`{"version":1,`,
	}
	for _, raw := range cases {
		t.Run(raw, func(t *testing.T) {
			if _, _, ok := fastSplitEnvelope([]byte(raw)); ok {
				t.Fatalf("fastSplitEnvelope(%q) returned ok=true for truncated input", raw)
			}
		})
	}
}
