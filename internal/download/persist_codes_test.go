package download

import (
	"errors"
	"strings"
	"testing"

	"typhon/internal/uierr"
)

// A failed write of the downloads list has to reach the window as a code: the
// prose around it is Russian and the frontend translates codes only.
func TestFailedWritesCarryTheSaveCode(t *testing.T) {
	cases := []struct {
		name string
		act  func(t *testing.T, m *Manager) error
	}{
		{"pause", func(t *testing.T, m *Manager) error {
			m.addTestDownload("a")
			breakStore(t, m)
			return m.Pause("a")
		}},
		{"resume", func(t *testing.T, m *Manager) error {
			m.addTestItem("a", StatusPaused)
			attachFake(m, "a")
			breakStore(t, m)
			return m.Resume("a")
		}},
		{"force start", func(t *testing.T, m *Manager) error {
			m.addTestItem("a", StatusQueued)
			attachFake(m, "a")
			breakStore(t, m)
			return m.ForceStart("a")
		}},
		{"move down", func(t *testing.T, m *Manager) error {
			m.addTestItem("a", StatusQueued)
			m.addTestItem("b", StatusQueued)
			breakStore(t, m)
			return m.MoveDown("a")
		}},
		{"move up", func(t *testing.T, m *Manager) error {
			m.addTestItem("a", StatusQueued)
			m.addTestItem("b", StatusQueued)
			breakStore(t, m)
			return m.MoveUp("b")
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := newTestManager(t, 2)

			err := c.act(t, m)

			if err == nil {
				t.Fatal("the failed write was swallowed")
			}
			if got := uierr.Code(err); got != "download.save_failed" {
				t.Fatalf("code = %q (%v), want download.save_failed", got, err)
			}
		})
	}
}

// A write refused because the list was never read is not a failed write: the
// window has to say the list is unreadable, not that a save went wrong.
func TestRefusedWritesCarryTheUnreadListCode(t *testing.T) {
	cases := []struct {
		name string
		act  func(t *testing.T) (m *Manager, loadErr, err error)
	}{
		{"pause", func(t *testing.T) (*Manager, error, error) {
			m := newTestManager(t, 2)
			m.addTestDownload("a")
			loadErr := loseTheList(t, m)
			return m, loadErr, m.Pause("a")
		}},
		{"resume", func(t *testing.T) (*Manager, error, error) {
			m := newTestManager(t, 2)
			m.addTestItem("a", StatusPaused)
			attachFake(m, "a")
			loadErr := loseTheList(t, m)
			return m, loadErr, m.Resume("a")
		}},
		{"force start", func(t *testing.T) (*Manager, error, error) {
			m := newTestManager(t, 2)
			m.addTestItem("a", StatusQueued)
			attachFake(m, "a")
			loadErr := loseTheList(t, m)
			return m, loadErr, m.ForceStart("a")
		}},
		{"move", func(t *testing.T) (*Manager, error, error) {
			m := newTestManager(t, 2)
			m.addTestItem("a", StatusQueued)
			m.addTestItem("b", StatusQueued)
			loadErr := loseTheList(t, m)
			return m, loadErr, m.MoveTo("b", 0)
		}},
		{"start", func(t *testing.T) (*Manager, error, error) {
			m, hash := fetchedTorrent(t)
			loadErr := loseTheList(t, m)
			_, err := m.StartDownloadFrom(hash, t.TempDir(), nil, Origin{})
			return m, loadErr, err
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m, loadErr, err := c.act(t)

			if st := m.degradedStatus(); !st.Degraded || strings.Contains(st.Message, "typhon:") {
				t.Fatalf("degraded = %+v: the notice is shown as text and must not carry the code", st)
			}
			if got := uierr.Code(err); got != "download.state_unread" {
				t.Fatalf("code = %q (%v), want download.state_unread", got, err)
			}
			if !errors.Is(err, errStateUnread) {
				t.Fatalf("error %v does not say the list was not read", err)
			}
			if !errors.Is(err, loadErr) {
				t.Fatalf("error %v lost why the list was not read: %v", err, loadErr)
			}
		})
	}
}
