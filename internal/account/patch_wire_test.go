package account

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func sentProfile(t *testing.T, patch Patch) map[string]json.RawMessage {
	t.Helper()
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
	return body.Profile
}

func TestUpdateProfileSendsAppearanceAndShowcaseOnlyWhenSet(t *testing.T) {
	const privacy = `"visibility":"private","showOnline":true,"showPlaying":true,"showPlaytime":false,"showLibrary":true,"showActivity":true,"showStats":true`
	themed := `{"theme":"forest","accent":"#91c59c","coverUrl":"","coverDim":10,"coverPosition":20,"customFrom":"#142235","customTo":"#111923","customAngle":90,"autoSource":"playing","avatarFrame":"none","nameStyle":"plain","parallax":false}`
	tests := []struct {
		name    string
		profile string
		want    map[string]string
	}{
		{"privacy only keeps both out of the body", `{` + privacy + `}`, map[string]string{"appearance": "", "showcase": ""}},
		{"null appearance is absent, not the defaults", `{` + privacy + `,"appearance":null}`, map[string]string{"appearance": "", "showcase": ""}},
		{"explicit appearance replaces", `{` + privacy + `,"appearance":` + themed + `}`, map[string]string{"appearance": themed, "showcase": ""}},
		{"showcase list replaces", `{` + privacy + `,"showcase":["most_played","favorites"]}`, map[string]string{"appearance": "", "showcase": `["most_played","favorites"]`}},
		{"empty showcase clears, unlike an absent one", `{` + privacy + `,"showcase":[]}`, map[string]string{"appearance": "", "showcase": `[]`}},
		{"both sent", `{` + privacy + `,"showcase":["favorites"],"appearance":` + themed + `}`, map[string]string{"appearance": themed, "showcase": `["favorites"]`}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var patch Patch
			if err := json.Unmarshal([]byte(`{"profile":`+tc.profile+`}`), &patch); err != nil {
				t.Fatal(err)
			}
			sent := sentProfile(t, patch)
			for key, want := range tc.want {
				got, present := sent[key]
				if want == "" {
					if present {
						t.Fatalf("%s = %s in the body, want it absent", key, got)
					}
					continue
				}
				if !present {
					t.Fatalf("%s missing from the body", key)
				}
				var gotAny, wantAny any
				if err := json.Unmarshal(got, &gotAny); err != nil {
					t.Fatalf("decode sent %s: %v", key, err)
				}
				if err := json.Unmarshal([]byte(want), &wantAny); err != nil {
					t.Fatal(err)
				}
				gotNorm, err := json.Marshal(gotAny)
				if err != nil {
					t.Fatal(err)
				}
				wantNorm, err := json.Marshal(wantAny)
				if err != nil {
					t.Fatal(err)
				}
				if string(gotNorm) != string(wantNorm) {
					t.Fatalf("%s = %s, want %s", key, got, want)
				}
			}
			if string(sent["visibility"]) != `"private"` {
				t.Fatalf("visibility = %s, want it sent", sent["visibility"])
			}
		})
	}
}

func TestUpdateProfileWithGoSettingsOmitsUnsetAppearanceAndShowcase(t *testing.T) {
	sent := sentProfile(t, Patch{Profile: &ProfileSettings{Visibility: VisibilityPublic, ShowOnline: true}})
	for _, key := range []string{"appearance", "showcase"} {
		if got, present := sent[key]; present {
			t.Fatalf("%s = %s in the body, want it absent", key, got)
		}
	}
}

func TestCachedProfileStillCarriesAppearanceAndShowcase(t *testing.T) {
	var user CurrentUser
	if err := json.Unmarshal([]byte(`{"profile":{"visibility":"public"}}`), &user); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(withProfileDefaults(user))
	if err != nil {
		t.Fatal(err)
	}
	var back struct {
		Profile map[string]json.RawMessage `json:"profile"`
	}
	if err := json.Unmarshal(encoded, &back); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"appearance", "showcase"} {
		if _, present := back.Profile[key]; !present {
			t.Fatalf("%s missing from the profile handed to the UI: %s", key, encoded)
		}
	}
}
