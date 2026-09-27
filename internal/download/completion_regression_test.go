package download

import (
	"testing"
)

func TestRejectInconsistentCompletionBitset(t *testing.T) {
	for _, data := range []string{
		`{"version":1,"torrents":{"0000000000000000000000000000000000000000":{"pieces":"","count":1}}}`,
		`{"version":1,"torrents":{"0000000000000000000000000000000000000000":{"pieces":"AA==","count":-1}}}`,
	} {
		if _, err := decodeCompletionFile([]byte(data)); err == nil {
			t.Fatalf("accepted malformed completion: %s", data)
		}
	}
}
