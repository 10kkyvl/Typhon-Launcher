package download

import (
	"errors"
	"strings"
	"testing"
)

// TestNoContextFallbackReturnsNoClientError exercises bug #5's three sites
// directly. In normal operation m.client and m.ctx are only ever set
// together, in ServiceStartup, so every other test in this package (which
// goes through mustManagerAt's withTestContext) can never actually reach
// these branches — this test builds a manager the same way ServiceStartup
// would leave it if a future change ever set one without the other, and
// checks the service refuses instead of quietly defaulting to
// context.Background() (invariant 20).
func TestNoContextFallbackReturnsNoClientError(t *testing.T) {
	m, err := newManagerAt(t.TempDir(), nil)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	closePieceCompletionOnCleanup(t, m)

	// offlineClient пропускает тест, если torrent-клиент не поднялся, а
	// пропуск — это runtime.Goexit: под мьютексом он не дал бы выполниться
	// Unlock, и cleanup на этой же горутине встал бы на нём навсегда.
	// Мьютексы в Go не рекурсивные, так что клиент создаётся до захвата.
	cl := offlineClient(t)

	m.mu.Lock()
	m.client = cl
	// m.ctx is intentionally left nil: ServiceStartup was never called.
	m.mu.Unlock()

	magnet := "magnet:?xt=urn:btih:" + strings.Repeat("a", 40)
	if _, err := m.FetchMetadata(magnet); !errors.Is(err, errNoClient) {
		t.Fatalf("FetchMetadata with a nil ctx = %v, want errNoClient", err)
	}
	if _, _, err := m.engine(); !errors.Is(err, errNoClient) {
		t.Fatalf("engine() with a nil ctx = %v, want errNoClient", err)
	}
	if err := m.spawnSettleLocked("x", "hash", nil); !errors.Is(err, errNoClient) {
		t.Fatalf("spawnSettleLocked with a nil ctx = %v, want errNoClient", err)
	}
}
