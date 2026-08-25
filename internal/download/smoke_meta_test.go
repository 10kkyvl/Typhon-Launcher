//go:build smoke

package download

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"typhon/internal/settings"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const debianMagnet = "magnet:?xt=urn:btih:481b6e3617be4c88f96cb25e47c9d8272130071e&dn=debian-13.6.0-amd64-netinst.iso&tr=http%3A%2F%2Fbttracker.debian.org%3A6969%2Fannounce"

func TestSmokeClientMetadataOnly(t *testing.T) {
	cl, err := newClient(settings.Defaults(), filepath.Join(t.TempDir(), "meta"))
	if err != nil {
		t.Fatal(err)
	}
	defer cl.close()

	lt, err := cl.addMagnet(debianMagnet, cl.metaDir)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.After(100 * time.Second)
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-lt.t.GotInfo():
			t.Logf("GOT INFO %s", lt.t.Name())
			return
		case <-tick.C:
			st := lt.t.Stats()
			t.Logf("peers total=%d active=%d halfopen=%d pending=%d", st.TotalPeers, st.ActivePeers, st.HalfOpenPeers, st.PendingPeers)
		case <-deadline:
			t.Fatal("timeout")
		}
	}
}

func TestSmokeMagnetMetadataOnly(t *testing.T) {
	m := mustManagerAt(t, t.TempDir())
	if err := m.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	defer m.ServiceShutdown()

	info, err := m.FetchMetadata(debianMagnet)
	if err != nil {
		t.Fatalf("fetch metadata: %v", err)
	}
	t.Logf("got %q files=%d", info.Name, len(info.Files))
	m.DiscardMetadata(info.InfoHash)
}
