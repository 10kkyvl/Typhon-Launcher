package download

import (
	"errors"
	"testing"
	"time"
)

// TestCancelFetchMetadataReleasesReservationImmediately reproduces the real
// user report: closing the add-download window while FetchMetadata is still
// waiting for a magnet's metadata used to leave the torrent added and its
// infohash reserved for the full metadataTimeout (90s), because FetchMetadata
// only ever listened on the manager's whole-lifetime m.ctx. A second attempt
// for the same hash in that window hit errHashBusy with a message implying
// another download owns it, when really it is the user's own abandoned
// attempt. CancelFetchMetadata must make the first call return at once and
// release the reservation, so the very next call is not hash-busy.
func TestCancelFetchMetadataReleasesReservationImmediately(t *testing.T) {
	m := newTestManager(t, 1)
	m.client = offlineClient(t)

	const hash = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	source := "magnet:?xt=urn:btih:" + hash

	attempt := func() error {
		done := make(chan error, 1)
		go func() {
			_, err := m.FetchMetadata(source)
			done <- err
		}()

		waitUntil(t, "FetchMetadata to reserve the infohash", func() bool {
			m.mu.Lock()
			defer m.mu.Unlock()
			return m.reserved[hash]
		})

		m.CancelFetchMetadata(source)

		select {
		case err := <-done:
			return err
		case <-time.After(2 * time.Second):
			t.Fatal("FetchMetadata did not return after CancelFetchMetadata")
			return nil
		}
	}

	if err := attempt(); err == nil {
		t.Fatal("cancelled FetchMetadata returned no error")
	}

	m.mu.Lock()
	stillReserved := m.reserved[hash]
	m.mu.Unlock()
	if stillReserved {
		t.Fatal("infohash reservation was not released right after cancellation")
	}

	if err := attempt(); errors.Is(err, errHashBusy) {
		t.Fatal("retrying FetchMetadata right after cancellation still reports errHashBusy")
	}
}
