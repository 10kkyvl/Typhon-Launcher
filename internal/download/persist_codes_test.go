package download

import (
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
