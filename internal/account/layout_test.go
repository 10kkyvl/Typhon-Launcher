package account

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func decodeSettings(t *testing.T, data string) ProfileSettings {
	t.Helper()
	var settings ProfileSettings
	if err := json.Unmarshal([]byte(data), &settings); err != nil {
		t.Fatalf("decode %s: %v", data, err)
	}
	return settings
}

func settingsKeys(t *testing.T, settings ProfileSettings) map[string]json.RawMessage {
	t.Helper()
	encoded, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &keys); err != nil {
		t.Fatal(err)
	}
	return keys
}

func ptr(s string) *string { return &s }

func deref(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}

func equalPtr(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func TestProfileSettingsLayoutIsTriState(t *testing.T) {
	blocks := `{"version":1,"blocks":[{"id":"b1","type":"about","width":"full","config":{}}]}`
	tests := []struct {
		name      string
		input     string
		wantSet   bool
		wantValue bool
		wantBody  string
	}{
		{"absent", `{"visibility":"public"}`, false, false, ""},
		{"null", `{"visibility":"public","layout":null}`, true, false, "null"},
		{"empty layout", `{"layout":{"version":1,"blocks":[]}}`, true, true, `{"version":1,"blocks":[]}`},
		{"blocks", `{"layout":` + blocks + `}`, true, true, blocks},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			settings := decodeSettings(t, tc.input)
			if settings.Layout.Set != tc.wantSet {
				t.Fatalf("Set = %v, want %v", settings.Layout.Set, tc.wantSet)
			}
			if (settings.Layout.Value != nil) != tc.wantValue {
				t.Fatalf("Value = %+v, want value %v", settings.Layout.Value, tc.wantValue)
			}
			got, present := settingsKeys(t, settings)["layout"]
			if present != tc.wantSet {
				t.Fatalf("layout present in body = %v, want %v (%s)", present, tc.wantSet, got)
			}
			if tc.wantSet && string(got) != tc.wantBody {
				t.Fatalf("layout body = %s, want %s", got, tc.wantBody)
			}
		})
	}
}

func TestProfileSettingsLayoutRejectsMalformed(t *testing.T) {
	for _, input := range []string{
		`{"layout":"x"}`,
		`{"layout":{"version":"one","blocks":[]}}`,
		`{"layout":{"version":1,"blocks":{}}}`,
	} {
		var settings ProfileSettings
		if err := json.Unmarshal([]byte(input), &settings); err == nil {
			t.Fatalf("%s decoded to %+v, want error", input, settings)
		}
	}
}

func TestProfileSettingsStatusIsTriState(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantEmoji *string
		wantText  *string
	}{
		{"absent", `{}`, nil, nil},
		{"empty strings clear", `{"statusEmoji":"","statusText":""}`, ptr(""), ptr("")},
		{"values", `{"statusEmoji":"🎮","statusText":"фармлю боссов"}`, ptr("🎮"), ptr("фармлю боссов")},
		{"only text", `{"statusText":"hi"}`, nil, ptr("hi")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			settings := decodeSettings(t, tc.input)
			if !equalPtr(settings.StatusEmoji, tc.wantEmoji) || !equalPtr(settings.StatusText, tc.wantText) {
				t.Fatalf("status = %s / %s", deref(settings.StatusEmoji), deref(settings.StatusText))
			}
			keys := settingsKeys(t, settings)
			if _, ok := keys["statusEmoji"]; ok != (tc.wantEmoji != nil) {
				t.Fatalf("statusEmoji present = %v, want %v", ok, tc.wantEmoji != nil)
			}
			if _, ok := keys["statusText"]; ok != (tc.wantText != nil) {
				t.Fatalf("statusText present = %v, want %v", ok, tc.wantText != nil)
			}
		})
	}
}

func TestUpdateProfileKeepsPatchStatesOnTheWire(t *testing.T) {
	unknown := `{"future":"<b>&</b>","nested":{"n":[1,2,3]},"z":null}`
	layout := `{"version":1,"blocks":[{"id":"b1","type":"hologram","width":"half","config":` + unknown + `}]}`
	tests := []struct {
		name    string
		profile string
		want    map[string]string
	}{
		{"all absent", `{"visibility":"friends"}`, map[string]string{"layout": "", "statusEmoji": "", "statusText": ""}},
		{"null resets layout", `{"visibility":"friends","layout":null}`, map[string]string{"layout": "null", "statusEmoji": "", "statusText": ""}},
		{"empty strings clear status", `{"visibility":"friends","statusEmoji":"","statusText":""}`, map[string]string{"layout": "", "statusEmoji": `""`, "statusText": `""`}},
		{"unknown block survives byte for byte", `{"visibility":"friends","layout":` + layout + `}`, map[string]string{"layout": layout, "statusEmoji": "", "statusText": ""}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var raw []byte
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var err error
				raw, err = io.ReadAll(r.Body)
				if err != nil {
					t.Errorf("read body: %v", err)
				}
				writeJSON(t, w, http.StatusOK, CurrentUser{ID: "u1"})
			}))
			defer srv.Close()

			var patch Patch
			if err := json.Unmarshal([]byte(`{"profile":`+tc.profile+`}`), &patch); err != nil {
				t.Fatal(err)
			}
			c := newTestClient(t, srv.URL, tokenOK("t"))
			if _, err := c.UpdateProfile(context.Background(), patch); err != nil {
				t.Fatalf("UpdateProfile: %v", err)
			}

			var body struct {
				Profile map[string]json.RawMessage `json:"profile"`
			}
			if err := json.Unmarshal(raw, &body); err != nil {
				t.Fatalf("decode sent body %s: %v", raw, err)
			}
			for key, want := range tc.want {
				got, present := body.Profile[key]
				if want == "" {
					if present {
						t.Fatalf("%s present in %s, want absent", key, raw)
					}
					continue
				}
				if !present {
					t.Fatalf("%s missing in %s", key, raw)
				}
				if !bytes.Equal(got, []byte(want)) {
					t.Fatalf("%s = %s, want %s", key, got, want)
				}
			}
		})
	}
}

func TestCurrentUserNullLayoutReadsAsNeverCustomised(t *testing.T) {
	for _, layout := range []string{``, `"layout":null,`} {
		var user CurrentUser
		if err := json.Unmarshal([]byte(`{"profile":{`+layout+`"visibility":"public","showcase":["favorites"]}}`), &user); err != nil {
			t.Fatal(err)
		}
		user = withProfileDefaults(user)
		if user.Profile.Layout.Set {
			t.Fatalf("%q: layout stayed Set, an unchanged settings object would reset the layout on the next PATCH", layout)
		}
	}
}

func TestCurrentUserKeepsLayoutAndStatusWhenShowcaseIsMissing(t *testing.T) {
	var user CurrentUser
	data := `{"profile":{"statusEmoji":"🎮","statusText":"hi","layout":{"version":1,"blocks":[{"id":"b1","type":"about","width":"full","config":{}}]}}}`
	if err := json.Unmarshal([]byte(data), &user); err != nil {
		t.Fatal(err)
	}
	user = withProfileDefaults(user)
	if user.Profile.Layout.Value == nil || len(user.Profile.Layout.Value.Blocks) != 1 {
		t.Fatalf("layout lost: %+v", user.Profile.Layout)
	}
	if deref(user.Profile.StatusEmoji) != "🎮" || deref(user.Profile.StatusText) != "hi" {
		t.Fatalf("status lost: %s / %s", deref(user.Profile.StatusEmoji), deref(user.Profile.StatusText))
	}
}

func TestProfileAppearanceNewFieldsMergeDefaults(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  func(a *ProfileAppearance)
	}{
		{"legacy appearance gets every default", `{"theme":"orbital","accent":"#123456"}`, func(a *ProfileAppearance) {
			a.Theme, a.Accent = "orbital", "#123456"
		}},
		{"explicit values", `{"theme":"custom","customFrom":"#1a1033","customTo":"#0b2a3a","customAngle":90,"autoSource":"most_played","avatarFrame":"neon","nameStyle":"glow","parallax":false}`, func(a *ProfileAppearance) {
			a.Theme, a.CustomFrom, a.CustomTo, a.CustomAngle = "custom", "#1a1033", "#0b2a3a", 90
			a.AutoSource, a.AvatarFrame, a.NameStyle, a.Parallax = "most_played", "neon", "glow", false
		}},
		{"angle upper bound", `{"theme":"custom","customAngle":360}`, func(a *ProfileAppearance) { a.Theme, a.CustomAngle = "custom", 360 }},
		{"angle zero", `{"theme":"custom","customAngle":0}`, func(a *ProfileAppearance) { a.Theme, a.CustomAngle = "custom", 0 }},
		{"angle above range", `{"theme":"custom","customAngle":361}`, func(a *ProfileAppearance) { a.Theme = "custom" }},
		{"angle below range", `{"theme":"custom","customAngle":-5}`, func(a *ProfileAppearance) { a.Theme = "custom" }},
		{"empty strings fall back", `{"theme":"midnight","customFrom":"","autoSource":"","avatarFrame":"","nameStyle":""}`, func(a *ProfileAppearance) {}},
		{"missing parallax stays on", `{"theme":"forest"}`, func(a *ProfileAppearance) { a.Theme = "forest" }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var user CurrentUser
			if err := json.Unmarshal([]byte(`{"profile":{"appearance":`+tc.input+`}}`), &user); err != nil {
				t.Fatal(err)
			}
			got := withProfileDefaults(user).Profile.Appearance
			want := DefaultProfileAppearance()
			tc.want(&want)
			if got != want {
				t.Fatalf("appearance = %+v, want %+v", got, want)
			}
		})
	}
}
