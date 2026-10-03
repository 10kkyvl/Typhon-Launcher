package installguard

import "testing"

func TestMusicState(t *testing.T) {
	for _, s := range []string{"&Music", "Play music", "  Фоновая   музыка  "} {
		if checked, ok := MusicState(s); !ok || checked {
			t.Errorf("%q: %v %v", s, checked, ok)
		}
	}
	for _, s := range []string{"Mute music", "Отключить музыку"} {
		if checked, ok := MusicState(s); !ok || !checked {
			t.Errorf("%q: %v %v", s, checked, ok)
		}
	}
	for _, s := range []string{"Install soundtrack", "Music files", "Music pack", "Sound effects", "Install", ""} {
		if _, ok := MusicState(s); ok {
			t.Errorf("must not toggle component %q", s)
		}
	}
}

func TestNextVerifierAction(t *testing.T) {
	for _, tc := range []struct {
		name   string
		verify bool
		closed bool
		waited int
		want   verifierAction
	}{
		{"allowed, fresh window", true, false, 0, verifierObserve},
		{"allowed never closes or kills", true, true, verifierKillAfter * 4, verifierObserve},
		{"skipped, close at once", false, false, 0, verifierClose},
		{"skipped, close ignores waited", false, false, verifierKillAfter * 4, verifierClose},
		{"skipped, just closed", false, true, 0, verifierWait},
		{"skipped, last grace pass", false, true, verifierKillAfter - 1, verifierWait},
		{"skipped, grace over", false, true, verifierKillAfter, verifierTerminate},
		{"skipped, long overdue", false, true, verifierKillAfter * 4, verifierTerminate},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := nextVerifierAction(tc.verify, tc.closed, tc.waited); got != tc.want {
				t.Fatalf("nextVerifierAction(%v, %v, %d) = %d, want %d", tc.verify, tc.closed, tc.waited, got, tc.want)
			}
		})
	}
}

func TestOptionalSiteAction(t *testing.T) {
	for _, label := range []string{"Visit FitGirl website", "Open repacker web site", "Посетить сайт Игруха", "Перейти на сайт", "Apply redirection to official FitGirl site", "Настроить перенаправление на сайт"} {
		if !OptionalSiteAction(label) {
			t.Errorf("missed %q", label)
		}
	}
	for _, label := range []string{"Verify game files", "Install game", "Accept website license", "Music files", "Open game"} {
		if OptionalSiteAction(label) {
			t.Errorf("matched %q", label)
		}
	}
}
