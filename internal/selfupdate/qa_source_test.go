package selfupdate

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

func qaLookup(env map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		v, ok := env[key]
		return v, ok
	}
}

func TestQASourceAbsentWhenNeitherVariableIsSet(t *testing.T) {
	for name, env := range map[string]map[string]string{
		"unset": nil,
		"blank": {qaManifestURLEnv: " ", qaReleasePubKeyEnv: "\n"},
	} {
		t.Run(name, func(t *testing.T) {
			src, ok, err := parseQASource(qaLookup(env))
			if err != nil || ok {
				t.Fatalf("parseQASource() = %+v, %v, %v; want no source and no error", src, ok, err)
			}
		})
	}
}

func TestQASourceTakesALoopbackURLAndAKey(t *testing.T) {
	_, pub := testKeyPair(t)
	encoded := base64.StdEncoding.EncodeToString(pub)
	for raw, want := range map[string]string{
		"http://127.0.0.1:8099":    "http://127.0.0.1:8099",
		"http://127.0.0.1:8099/":   "http://127.0.0.1:8099",
		"http://localhost:1":       "http://localhost:1",
		" http://localhost:65535 ": "http://localhost:65535",
	} {
		t.Run(raw, func(t *testing.T) {
			src, ok, err := parseQASource(qaLookup(map[string]string{qaManifestURLEnv: raw, qaReleasePubKeyEnv: encoded}))
			if err != nil || !ok {
				t.Fatalf("parseQASource() = %v, %v; want the source", ok, err)
			}
			if src.baseURL != want {
				t.Fatalf("baseURL = %q, want %q", src.baseURL, want)
			}
			if !bytes.Equal(src.key, pub) {
				t.Fatal("key differs from the one in the variable")
			}
		})
	}
}

// A qa build must not be pointable at a server elsewhere that signs with its
// own key: anything but plain http to 127.0.0.1 or localhost with a port is
// refused, https included.
func TestQASourceRefusesAnyURLButLoopbackHTTP(t *testing.T) {
	_, pub := testKeyPair(t)
	encoded := base64.StdEncoding.EncodeToString(pub)
	for _, raw := range []string{
		"https://127.0.0.1:8099",
		"https://typhon.example",
		"http://example.com:8099",
		"http://localhost.example.com:8099",
		"http://127.0.0.2:8099",
		"http://[::1]:8099",
		"http://0.0.0.0:8099",
		"http://LOCALHOST:8099",
		"http://127.0.0.1",
		"http://127.0.0.1:",
		"http://127.0.0.1:0",
		"http://127.0.0.1:08099",
		"http://127.0.0.1:65536",
		"http://127.0.0.1:port",
		"http://127.0.0.1:8099/launcher",
		"http://127.0.0.1:8099?next=https://example.com",
		"http://127.0.0.1:8099#x",
		"http://user@127.0.0.1:8099",
		"http://127.0.0.1:8099@example.com",
		"ftp://127.0.0.1:8099",
		"127.0.0.1:8099",
		"//127.0.0.1:8099",
	} {
		t.Run(raw, func(t *testing.T) {
			src, ok, err := parseQASource(qaLookup(map[string]string{qaManifestURLEnv: raw, qaReleasePubKeyEnv: encoded}))
			if !errors.Is(err, errQAUpdateSource) || ok {
				t.Fatalf("parseQASource(%q) = %+v, %v, %v; want errQAUpdateSource", raw, src, ok, err)
			}
			if !strings.Contains(err.Error(), qaManifestURLEnv) {
				t.Fatalf("error %q does not name %s", err, qaManifestURLEnv)
			}
		})
	}
}

func TestQASourceRefusesABadKey(t *testing.T) {
	for name, value := range map[string]string{
		"not base64": "not*base64",
		"too short":  base64.StdEncoding.EncodeToString(make([]byte, ed25519.PublicKeySize-1)),
		"too long":   base64.StdEncoding.EncodeToString(make([]byte, ed25519.PublicKeySize+1)),
	} {
		t.Run(name, func(t *testing.T) {
			_, ok, err := parseQASource(qaLookup(map[string]string{qaManifestURLEnv: "http://127.0.0.1:8099", qaReleasePubKeyEnv: value}))
			if !errors.Is(err, ErrBadPublicKey) || !errors.Is(err, errQAUpdateSource) || ok {
				t.Fatalf("parseQASource() = %v, %v; want ErrBadPublicKey and errQAUpdateSource", ok, err)
			}
			if !strings.Contains(err.Error(), qaReleasePubKeyEnv) {
				t.Fatalf("error %q does not name %s", err, qaReleasePubKeyEnv)
			}
		})
	}
}

// One variable without the other is a half-configured source: neither the
// test key against the real server nor the real key against the test server.
func TestQASourceNeedsBothVariables(t *testing.T) {
	_, pub := testKeyPair(t)
	for _, tc := range []struct {
		name    string
		env     map[string]string
		missing string
	}{
		{name: "url only", env: map[string]string{qaManifestURLEnv: "http://127.0.0.1:8099"}, missing: qaReleasePubKeyEnv},
		{name: "key only", env: map[string]string{qaReleasePubKeyEnv: base64.StdEncoding.EncodeToString(pub)}, missing: qaManifestURLEnv},
		{name: "blank key", env: map[string]string{qaManifestURLEnv: "http://127.0.0.1:8099", qaReleasePubKeyEnv: " "}, missing: qaReleasePubKeyEnv},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, ok, err := parseQASource(qaLookup(tc.env))
			if !errors.Is(err, errQAUpdateSource) || ok {
				t.Fatalf("parseQASource() = %v, %v; want errQAUpdateSource", ok, err)
			}
			if !strings.Contains(err.Error(), tc.missing+" is missing") {
				t.Fatalf("error %q does not say %s is missing", err, tc.missing)
			}
		})
	}
}

// The test source publishes no installer, so a qa launcher it feeds updates
// the portable way; and its update record, cache and worker live in its
// config dir, which outside portable mode is the one the installed launcher
// reads: there the source is refused with words that say how to isolate it.
func TestQAUpdateModeFollowsTheTestSource(t *testing.T) {
	_, pub := testKeyPair(t)
	active := map[string]string{qaManifestURLEnv: "http://127.0.0.1:8099", qaReleasePubKeyEnv: base64.StdEncoding.EncodeToString(pub)}
	for _, tc := range []struct {
		name     string
		env      map[string]string
		portable bool
		want     bool
		wantErr  string
	}{
		{name: "no source, portable", portable: true, want: true},
		{name: "no source, installed", portable: false, want: false},
		{name: "blank source, installed", env: map[string]string{qaManifestURLEnv: " ", qaReleasePubKeyEnv: " "}, portable: false, want: false},
		{name: "source, portable", env: active, portable: true, want: true},
		{name: "source, installed", env: active, portable: false, wantErr: "TYPHON_PORTABLE_DIR"},
		{name: "url only, portable", env: map[string]string{qaManifestURLEnv: "http://127.0.0.1:8099"}, portable: true, wantErr: qaReleasePubKeyEnv + " is missing"},
		{name: "url only, installed", env: map[string]string{qaManifestURLEnv: "http://127.0.0.1:8099"}, portable: false, wantErr: qaReleasePubKeyEnv + " is missing"},
		{name: "remote url, portable", env: map[string]string{qaManifestURLEnv: "https://typhon.example", qaReleasePubKeyEnv: active[qaReleasePubKeyEnv]}, portable: true, wantErr: qaManifestURLEnv},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := qaUpdatesPortable(tc.portable, qaLookup(tc.env))
			if tc.wantErr != "" {
				if !errors.Is(err, errQAUpdateSource) || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("qaUpdatesPortable(%v) = %v, %v; want errQAUpdateSource naming %q", tc.portable, got, err, tc.wantErr)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("qaUpdatesPortable(%v) = %v, %v; want %v", tc.portable, got, err, tc.want)
			}
		})
	}
}
