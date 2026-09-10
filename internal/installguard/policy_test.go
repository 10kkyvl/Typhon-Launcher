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
