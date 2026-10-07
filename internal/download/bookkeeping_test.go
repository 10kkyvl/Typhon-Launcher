package download

import (
	"testing"
)

func (m *Manager) tracesOf(id string) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []string
	if _, ok := m.engines[id]; ok {
		out = append(out, "engines")
	}
	if _, ok := m.rates[id]; ok {
		out = append(out, "rates")
	}
	if m.verified[id] {
		out = append(out, "verified")
	}
	if _, ok := m.jobs[id]; ok {
		out = append(out, "jobs")
	}
	if _, ok := m.teardowns[id]; ok {
		out = append(out, "teardowns")
	}
	return out
}

func TestEveryEndOfADownloadForgetsItsBookkeeping(t *testing.T) {
	cases := []struct {
		name string
		end  func(t *testing.T, m *Manager, d Download) error
	}{
		{"cancel while downloading", func(_ *testing.T, m *Manager, d Download) error { return m.Cancel(d.ID) }},
		{"remove while downloading", func(_ *testing.T, m *Manager, d Download) error { return m.Remove(d.ID) }},
		{"remove after a pause", func(t *testing.T, m *Manager, d Download) error {
			if err := m.Pause(d.ID); err != nil {
				t.Fatal(err)
			}
			return m.Remove(d.ID)
		}},
		{"delete the data after completion", func(t *testing.T, m *Manager, d Download) error {
			m.mu.Lock()
			m.completeLocked(m.findLocked(d.ID))
			m.mu.Unlock()
			return m.DeleteData(d.ID)
		}},
		{"stop and wait", func(_ *testing.T, m *Manager, d Download) error { return m.StopAndWait(d.ID) }},
		{"cancel after a failure", func(t *testing.T, m *Manager, d Download) error {
			m.markFailed(d.ID, "boom", nil)
			return m.Cancel(d.ID)
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mi, _ := threeFileTorrent(t)
			m := managerWithClient(t, 1)
			if _, err := m.FetchMetadata(writeTorrentFile(t, mi)); err != nil {
				t.Fatal(err)
			}
			d, err := m.StartDownload(mi.HashInfoBytes().HexString(), t.TempDir(), nil)
			if err != nil {
				t.Fatal(err)
			}
			if got := m.tracesOf(d.ID); len(got) != 3 {
				t.Fatalf("a running download is tracked in %v, want engines, rates and verified", got)
			}

			if err := c.end(t, m, d); err != nil {
				t.Fatal(err)
			}
			m.wg.Wait()

			if got := m.tracesOf(d.ID); len(got) != 0 {
				t.Fatalf("a finished-with download is still remembered in %v: the maps would grow with every download", got)
			}
			if pending, reserved := m.bookkeeping(); pending != 0 || reserved != 0 {
				t.Fatalf("pending = %d, reserved = %d", pending, reserved)
			}
			if got := len(m.client.cl.Torrents()); got != 0 {
				t.Fatalf("torrents left in the client: %d", got)
			}
		})
	}
}
