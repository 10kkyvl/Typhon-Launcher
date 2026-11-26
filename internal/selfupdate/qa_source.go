package selfupdate

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// The qa Windows build (url_qa.go, key_qa.go) can take its launcher updates
// from a local test source such as tools/devrelease. The parsing is untagged
// so that it is tested on every platform; no other build calls it.
const (
	qaManifestURLEnv   = "TYPHON_QA_MANIFEST_URL"
	qaReleasePubKeyEnv = "TYPHON_QA_RELEASE_PUBKEY"
)

var errQAUpdateSource = errors.New("selfupdate: qa update source")

type qaSource struct {
	baseURL string
	key     ed25519.PublicKey
}

// parseQASource reads both variables or neither. The test key with the real
// server, or the real key with the test server, is a half-configured source
// and an error, never a fallback.
func parseQASource(lookup func(string) (string, bool)) (qaSource, bool, error) {
	rawURL := qaVariable(lookup, qaManifestURLEnv)
	rawKey := qaVariable(lookup, qaReleasePubKeyEnv)
	switch {
	case rawURL == "" && rawKey == "":
		return qaSource{}, false, nil
	case rawKey == "":
		return qaSource{}, false, fmt.Errorf("%w: %s is set but %s is missing; set both or neither", errQAUpdateSource, qaManifestURLEnv, qaReleasePubKeyEnv)
	case rawURL == "":
		return qaSource{}, false, fmt.Errorf("%w: %s is set but %s is missing; set both or neither", errQAUpdateSource, qaReleasePubKeyEnv, qaManifestURLEnv)
	}
	base, err := qaLoopbackBaseURL(rawURL)
	if err != nil {
		return qaSource{}, false, err
	}
	key, err := qaPublicKey(rawKey)
	if err != nil {
		return qaSource{}, false, err
	}
	return qaSource{baseURL: base, key: key}, true, nil
}

func qaVariable(lookup func(string) (string, bool), name string) string {
	v, ok := lookup(name)
	if !ok {
		return ""
	}
	return strings.TrimSpace(v)
}

// qaLoopbackBaseURL accepts exactly http://127.0.0.1:<port> or
// http://localhost:<port>, so a mistyped or planted variable cannot name a
// server on another machine. A local listener can still redirect elsewhere;
// what it serves is trusted only through the key and the signed hashes.
func qaLoopbackBaseURL(raw string) (string, error) {
	refuse := fmt.Errorf("%w: %s = %q, want http://127.0.0.1:<port> or http://localhost:<port>", errQAUpdateSource, qaManifestURLEnv, raw)
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("%w (%w)", refuse, err)
	}
	if u.Scheme != "http" || u.Opaque != "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return "", refuse
	}
	if u.Path != "" && u.Path != "/" {
		return "", refuse
	}
	host, port := u.Hostname(), u.Port()
	if host != "127.0.0.1" && host != "localhost" {
		return "", refuse
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 || strconv.Itoa(n) != port || u.Host != host+":"+port {
		return "", refuse
	}
	return "http://" + u.Host, nil
}

func qaPublicKey(encoded string) (ed25519.PublicKey, error) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("%w: %w: %s: %w", errQAUpdateSource, ErrBadPublicKey, qaReleasePubKeyEnv, err)
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("%w: %w: %s holds %d bytes, want %d", errQAUpdateSource, ErrBadPublicKey, qaReleasePubKeyEnv, len(raw), ed25519.PublicKeySize)
	}
	return ed25519.PublicKey(raw), nil
}
