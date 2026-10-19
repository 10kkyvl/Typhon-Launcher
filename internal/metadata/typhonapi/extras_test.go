package typhonapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"typhon/internal/account"
	"typhon/internal/metadata"
)

const baseGame = `"providerId":"2657","title":"Prey"`

func getWith(t *testing.T, body string) (metadata.GameMetadata, error) {
	t.Helper()
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, body)
	}), nil)
	return client.Get(context.Background(), "2657")
}

func TestGetParsesSteamBlock(t *testing.T) {
	meta, err := getWith(t, `{`+baseGame+`,"steamAppId":480,"steam":{
		"appId":480,"url":"https://store.steampowered.com/app/480",
		"requirements":{"minimum":[{"label":"ОС","value":"Windows 10"}],"recommended":[{"label":"ОЗУ","value":"8 GB"}]},
		"dlc":[{"appId":481,"name":"Expansion"}],
		"controller":"full","metacritic":87,"recommendations":12345}}`)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if meta.SteamAppID != "480" {
		t.Fatalf("steam app id = %q, want 480 for a non-Steam provider game", meta.SteamAppID)
	}
	s := meta.Steam
	if s == nil || s.AppID != 480 || s.URL != "https://store.steampowered.com/app/480" {
		t.Fatalf("steam block = %+v", s)
	}
	if len(s.Requirements.Minimum) != 1 || s.Requirements.Minimum[0].Value != "Windows 10" || len(s.Requirements.Recommended) != 1 {
		t.Fatalf("requirements = %+v", s.Requirements)
	}
	if len(s.DLC) != 1 || s.DLC[0].AppID != 481 || s.Controller != "full" {
		t.Fatalf("dlc/controller = %+v / %q", s.DLC, s.Controller)
	}
	if s.Metacritic == nil || *s.Metacritic != 87 || s.Recommendations == nil || *s.Recommendations != 12345 {
		t.Fatalf("numbers = %v %v", s.Metacritic, s.Recommendations)
	}
}

func TestGetOldBackendHasNoSteamBlock(t *testing.T) {
	meta, err := getWith(t, `{`+baseGame+`}`)
	if err != nil {
		t.Fatalf("old backend must not be an error: %v", err)
	}
	if meta.Steam != nil {
		t.Fatalf("steam = %+v, want nil", meta.Steam)
	}
}

func TestGetSanitizesSteamBlock(t *testing.T) {
	long := strings.Repeat("я", 400)
	rows := make([]string, 0, 30)
	for range 30 {
		rows = append(rows, `{"label":"L","value":"V"}`)
	}
	dlc := make([]string, 0, 60)
	for i := 1; i <= 60; i++ {
		dlc = append(dlc, fmt.Sprintf(`{"appId":%d,"name":"d"}`, 1000+i))
	}
	meta, err := getWith(t, `{`+baseGame+`,"steam":{
		"appId":480,"url":"https://evil.example/app/480",
		"requirements":{"minimum":[{"label":"`+long+`","value":"`+long+`"},{"label":"x","value":"a\u0000b\u202ec"},{"label":"empty","value":""}],
			"recommended":[`+strings.Join(rows, ",")+`]},
		"dlc":[`+strings.Join(dlc, ",")+`,{"appId":0,"name":"zero"},{"appId":7,"name":""}],
		"controller":"maybe","metacritic":-5,"recommendations":-1}}`)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	s := meta.Steam
	if s == nil {
		t.Fatal("steam block dropped entirely")
	}
	if s.URL != "" {
		t.Fatalf("foreign host url kept: %q", s.URL)
	}
	if got := len([]rune(s.Requirements.Minimum[0].Label)); got != 64 {
		t.Fatalf("label runes = %d, want 64", got)
	}
	if got := len([]rune(s.Requirements.Minimum[0].Value)); got != 300 {
		t.Fatalf("value runes = %d, want 300", got)
	}
	if len(s.Requirements.Minimum) != 2 || s.Requirements.Minimum[1].Value != "abc" {
		t.Fatalf("control characters or empty row kept: %+v", s.Requirements.Minimum)
	}
	if len(s.Requirements.Recommended) != 20 {
		t.Fatalf("rows = %d, want 20", len(s.Requirements.Recommended))
	}
	if len(s.DLC) != 50 {
		t.Fatalf("dlc = %d, want 50", len(s.DLC))
	}
	if s.Controller != "" || s.Metacritic != nil || s.Recommendations != nil {
		t.Fatalf("controller %q metacritic %v recommendations %v", s.Controller, s.Metacritic, s.Recommendations)
	}
}

func TestGetDropsSteamBlockWithoutAppID(t *testing.T) {
	meta, err := getWith(t, `{`+baseGame+`,"steam":{"appId":0,"url":"https://store.steampowered.com/app/0"}}`)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if meta.Steam != nil {
		t.Fatalf("steam = %+v, want nil", meta.Steam)
	}
}

func TestSteamURLMustMatchAppID(t *testing.T) {
	for _, url := range []string{
		"http://store.steampowered.com/app/480",
		"https://store.steampowered.com/app/481",
		"https://store.steampowered.com/app/480?x=1",
		"https://store.steampowered.com.evil.com/app/480",
		"https://user@store.steampowered.com/app/480",
		"https://store.steampowered.com/agecheck/app/480",
	} {
		meta, err := getWith(t, `{`+baseGame+`,"steam":{"appId":480,"url":"`+url+`"}}`)
		if err != nil {
			t.Fatalf("%s: %v", url, err)
		}
		if meta.Steam == nil || meta.Steam.URL != "" {
			t.Fatalf("%s: url kept: %+v", url, meta.Steam)
		}
	}
	meta, err := getWith(t, `{`+baseGame+`,"steam":{"appId":480,"url":"https://store.steampowered.com/app/480/"}}`)
	if err != nil || meta.Steam == nil || meta.Steam.URL != "https://store.steampowered.com/app/480" {
		t.Fatalf("trailing slash form: %+v %v", meta.Steam, err)
	}
}

func savesClient(t *testing.T, status int, body string, gotPath *string) *Client {
	t.Helper()
	return newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if gotPath != nil {
			*gotPath = r.URL.Path
		}
		writeJSON(t, w, status, body)
	}), nil)
}

func TestSavePathsOK(t *testing.T) {
	var path string
	client := savesClient(t, http.StatusOK, `{"pageUrl":"https://www.pcgamingwiki.com/wiki/Prey","windows":["{appdata}/Arkane/Prey/Saves","{game}/save","{documents}/My Games/*/Saves"],"source":"PCGamingWiki","license":"CC BY-NC-SA 3.0","via":"Ludusavi Manifest"}`, &path)

	got, err := client.SavePaths(context.Background(), "2657")
	if err != nil {
		t.Fatalf("save paths: %v", err)
	}
	if path != account.APIPrefix+"/metadata/games/2657/saves" {
		t.Fatalf("path = %q", path)
	}
	if got.PageURL != "https://www.pcgamingwiki.com/wiki/Prey" || len(got.Windows) != 3 || got.Source != "PCGamingWiki" || got.License != "CC BY-NC-SA 3.0" || got.Via != "Ludusavi Manifest" {
		t.Fatalf("got %+v", got)
	}
}

func TestSavePathsBareNotFoundIsTransient(t *testing.T) {
	for _, body := range []string{`{"error":{"code":"not_found"}}`, `404 page not found`, ``} {
		client := savesClient(t, http.StatusNotFound, body, nil)
		_, err := client.SavePaths(context.Background(), "2657")
		if errors.Is(err, metadata.ErrNoSaveData) || !errors.Is(err, ErrUpstream) {
			t.Fatalf("body %q: err = %v, a 404 without the code must not read as absence", body, err)
		}
	}
}

func TestSavePathsNotFoundIsDistinct(t *testing.T) {
	client := savesClient(t, http.StatusNotFound, `{"error":{"code":"save_paths_not_found","message":"x"}}`, nil)
	_, err := client.SavePaths(context.Background(), "steam:480")
	if !errors.Is(err, metadata.ErrNoSaveData) {
		t.Fatalf("err = %v, want ErrNoSaveData", err)
	}
	if errors.Is(err, metadata.ErrNoMatch) || errors.Is(err, ErrUpstream) {
		t.Fatalf("404 must not look like a match or network failure: %v", err)
	}
}

func TestSavePathsServerFailureIsNotNoData(t *testing.T) {
	for _, status := range []int{http.StatusInternalServerError, http.StatusServiceUnavailable} {
		client := savesClient(t, status, `{"error":{"code":"save_paths_not_ready"}}`, nil)
		_, err := client.SavePaths(context.Background(), "2657")
		if !errors.Is(err, ErrUpstream) || errors.Is(err, metadata.ErrNoSaveData) {
			t.Fatalf("status %d: err = %v, want ErrUpstream only", status, err)
		}
	}
}

func TestSavePathsRejectsBadResponses(t *testing.T) {
	cases := map[string]string{
		"broken json":    `{"pageUrl":`,
		"foreign page":   `{"pageUrl":"https://evil.example/wiki/Prey","windows":[]}`,
		"http page":      `{"pageUrl":"http://www.pcgamingwiki.com/wiki/Prey","windows":[]}`,
		"lookalike host": `{"pageUrl":"https://www.pcgamingwiki.com.evil.example/wiki/Prey","windows":[]}`,
		"not a wiki url": `{"pageUrl":"https://www.pcgamingwiki.com/api/x","windows":[]}`,
		"empty page":     `{"pageUrl":"","windows":[]}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := savesClient(t, http.StatusOK, body, nil).SavePaths(context.Background(), "2657")
			if err == nil {
				t.Fatal("bad response accepted")
			}
			if errors.Is(err, metadata.ErrNoSaveData) {
				t.Fatalf("bad response reported as no data: %v", err)
			}
		})
	}
}

func TestSavePathsDropsBadPaths(t *testing.T) {
	many := make([]string, 0, 30)
	for i := range 30 {
		many = append(many, fmt.Sprintf(`"{game}/s%d"`, i))
	}
	body := `{"pageUrl":"https://www.pcgamingwiki.com/wiki/Prey","windows":["{game}/../x","C:/Users/x","{unknown}/x","{game}/C:/x","{appdata}/ok",` + strings.Join(many, ",") + `]}`
	got, err := savesClient(t, http.StatusOK, body, nil).SavePaths(context.Background(), "2657")
	if err != nil {
		t.Fatalf("save paths: %v", err)
	}
	if got.Windows[0] != "{appdata}/ok" {
		t.Fatalf("first path = %q, bad ones were not dropped: %v", got.Windows[0], got.Windows)
	}
	if len(got.Windows) != 20 {
		t.Fatalf("paths = %d, want capped at 20", len(got.Windows))
	}
	for _, p := range got.Windows {
		if strings.Contains(p, "..") || strings.Contains(p, ":") {
			t.Fatalf("unsafe path kept: %q", p)
		}
	}
}

func TestSavePathsRejectsBadID(t *testing.T) {
	client := savesClient(t, http.StatusOK, `{}`, nil)
	for _, id := range []string{"", "../x", "steam:", "12/saves"} {
		if _, err := client.SavePaths(context.Background(), id); !errors.Is(err, ErrBadRequest) {
			t.Fatalf("id %q: err = %v, want ErrBadRequest", id, err)
		}
	}
}
