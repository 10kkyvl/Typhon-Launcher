package accountsync

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

func rawGames(t *testing.T, raw []byte) []map[string]any {
	t.Helper()
	var body struct {
		Games []map[string]any `json:"games"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode PUT body %s: %v", raw, err)
	}
	return body.Games
}

func TestFeatureHeaderTokens(t *testing.T) {
	tests := []struct {
		name       string
		values     []string
		wantHidden bool
		wantDone   bool
	}{
		{"none", nil, false, false},
		{"empty value", []string{""}, false, false},
		{"hidden only", []string{"hidden"}, true, false},
		{"completion only", []string{"completion"}, false, true},
		{"both with a comma and a space", []string{"hidden, completion"}, true, true},
		{"both reversed without spaces", []string{"completion,hidden"}, true, true},
		{"padding and case", []string{"  Completion ,  HIDDEN  "}, true, true},
		{"two header lines", []string{"hidden", "completion"}, true, true},
		{"unknown tokens around", []string{"extra,completion,other"}, false, true},
		{"a longer token is not the feature", []string{"completionx,xhidden"}, false, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := &httpClient{}
			h := http.Header{}
			for _, v := range tc.values {
				h.Add(featuresHeader, v)
			}
			c.noteFeatures("tok", h)
			got := c.features()
			if got.hidden != tc.wantHidden || got.completion != tc.wantDone {
				t.Fatalf("features = %+v, want hidden=%v completion=%v", got, tc.wantHidden, tc.wantDone)
			}
		})
	}
}

func TestFeaturesResetWhenTheAccountChanges(t *testing.T) {
	c := &httpClient{}
	h := http.Header{}
	h.Set(featuresHeader, "hidden, completion")
	c.noteFeatures("one", h)
	c.forgetFeaturesIfAccountChanged("one")
	if got := c.features(); !got.hidden || !got.completion {
		t.Fatalf("features dropped for the same account: %+v", got)
	}
	c.forgetFeaturesIfAccountChanged("two")
	if got := c.features(); got.hidden || got.completion {
		t.Fatalf("features survived an account change: %+v", got)
	}
}

func TestCompletionFieldsFollowServerFeatureHeader(t *testing.T) {
	stamp := time.Now().UTC().Add(-48 * time.Hour).Truncate(time.Microsecond)
	done := stamp.Add(-24 * time.Hour)

	tests := []struct {
		name         string
		features     string
		local        Game
		wantStatus   string
		wantCompKey  bool
		wantComplete *time.Time
	}{
		{"no header: mastered goes as completed, no completedAt key", "", Game{Status: statusMastered, StatusAt: &stamp, CompletedAt: &done}, "completed", false, nil},
		{"hidden only: mastered goes as completed, no completedAt key", "hidden", Game{Status: statusMastered, StatusAt: &stamp, CompletedAt: &done}, "completed", false, nil},
		{"no header: completed keeps its status and has no completedAt key", "", Game{Status: "completed", StatusAt: &stamp, CompletedAt: &done}, "completed", false, nil},
		{"completion: mastered goes as is with the date", "completion", Game{Status: statusMastered, StatusAt: &stamp, CompletedAt: &done}, "mastered", true, &done},
		{"hidden and completion: completed goes with the date", "hidden, completion", Game{Status: "completed", StatusAt: &stamp, CompletedAt: &done}, "completed", true, &done},
		{"completion: completed with an unknown date has no key", "completion", Game{Status: "completed", StatusAt: &stamp}, "completed", false, nil},
		{"completion: a date on a status that is not completed is not sent", "completion", Game{Status: "playing", StatusAt: &stamp, CompletedAt: &done}, "playing", false, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			h.catalog.link("game-1", "1")
			tc.local.CanonicalGameID = "game-1"
			h.library.setLocalGame(tc.local)
			srv := &featureServer{features: tc.features}
			srv.install(h)
			if err := h.service.Sync(context.Background()); err != nil {
				t.Fatalf("Sync: %v", err)
			}
			raw := lastPutRaw(h)
			games := rawGames(t, raw)
			if len(games) != 1 {
				t.Fatalf("games in PUT = %d: %s", len(games), raw)
			}
			if got := games[0]["status"]; got != tc.wantStatus {
				t.Fatalf("wire status = %v, want %q: %s", got, tc.wantStatus, raw)
			}
			_, hasKey := games[0]["completedAt"]
			if hasKey != tc.wantCompKey {
				t.Fatalf("completedAt key present = %v, want %v: %s", hasKey, tc.wantCompKey, raw)
			}
			if !tc.wantCompKey && bytes.Contains(raw, []byte("completedAt")) {
				t.Fatalf("PUT mentions completedAt for a server that would answer 400: %s", raw)
			}
			if tc.wantComplete != nil {
				var typed putRequest
				if err := json.Unmarshal(raw, &typed); err != nil {
					t.Fatal(err)
				}
				if typed.Games[0].CompletedAt == nil || !typed.Games[0].CompletedAt.Equal(*tc.wantComplete) {
					t.Fatalf("wire completedAt = %v, want %v", typed.Games[0].CompletedAt, tc.wantComplete)
				}
			}
		})
	}
}

func TestMasteredDoesNotRollBackToCompletedWithoutTheFeature(t *testing.T) {
	stamp := time.Now().UTC().Add(-48 * time.Hour).Truncate(time.Microsecond)
	done := stamp.Add(-24 * time.Hour)
	h := newHarness(t)
	h.catalog.link("game-1", "1")
	h.library.setLocalGame(Game{CanonicalGameID: "game-1", Status: statusMastered, StatusAt: &stamp, CompletedAt: &done})
	srv := &featureServer{}
	srv.install(h)

	if err := h.service.Sync(context.Background()); err != nil {
		t.Fatalf("first Sync: %v", err)
	}
	if len(srv.remote) != 1 || srv.remote[0].Status != "completed" {
		t.Fatalf("server holds %+v, want the game as completed", srv.remote)
	}
	puts := h.server.putCalls

	if err := h.service.Sync(context.Background()); err != nil {
		t.Fatalf("second Sync: %v", err)
	}
	got := h.library.gameOf("game-1")
	if got.Status != statusMastered || got.CompletedAt == nil || !got.CompletedAt.Equal(done) {
		t.Fatalf("local game after the round trip = %+v, want mastered with the date kept", got)
	}
	if h.server.putCalls != puts {
		t.Fatalf("second Sync pushed again (%d -> %d puts): the wire status has to drive upToDate", puts, h.server.putCalls)
	}

	h.reopen()
	if err := h.service.Sync(context.Background()); err != nil {
		t.Fatalf("Sync after reopen: %v", err)
	}
	if got := h.library.gameOf("game-1"); got.Status != statusMastered {
		t.Fatalf("status after reopen = %q", got.Status)
	}
	if h.server.putCalls != puts {
		t.Fatalf("Sync after reopen pushed again (%d -> %d puts)", puts, h.server.putCalls)
	}
}

func TestCompletionDateTravelsWithTheNewerStatus(t *testing.T) {
	old := time.Now().UTC().Add(-72 * time.Hour).Truncate(time.Microsecond)
	fresh := old.Add(48 * time.Hour)
	localDone := old.Add(-24 * time.Hour)
	remoteDone := old.Add(-12 * time.Hour)

	tests := []struct {
		name       string
		features   string
		local      Game
		remote     wireGame
		wantStatus string
		wantDate   *time.Time
	}{
		{"completion: newer remote mastered with its date", "completion", Game{Status: "completed", StatusAt: &old, CompletedAt: &localDone}, wireGame{IGDBID: 1, Status: "mastered", StatusAt: &fresh, CompletedAt: &remoteDone}, "mastered", &remoteDone},
		{"completion: newer remote completed without a date replaces the local date", "completion", Game{Status: "mastered", StatusAt: &old, CompletedAt: &localDone}, wireGame{IGDBID: 1, Status: "completed", StatusAt: &fresh}, "completed", nil},
		{"completion: newer local keeps its status and date", "completion", Game{Status: "mastered", StatusAt: &fresh, CompletedAt: &localDone}, wireGame{IGDBID: 1, Status: "completed", StatusAt: &old, CompletedAt: &remoteDone}, "mastered", &localDone},
		{"completion: newer remote non-completed status drops the date", "completion", Game{Status: "mastered", StatusAt: &old, CompletedAt: &localDone}, wireGame{IGDBID: 1, Status: "playing", StatusAt: &fresh, CompletedAt: &remoteDone}, "playing", nil},
		{"no feature: newer remote completed brings no date", "", Game{Status: "playing", StatusAt: &old}, wireGame{IGDBID: 1, Status: "completed", StatusAt: &fresh, CompletedAt: &remoteDone}, "completed", nil},
		{"no feature: newer local mastered keeps its date", "", Game{Status: "mastered", StatusAt: &fresh, CompletedAt: &localDone}, wireGame{IGDBID: 1, Status: "completed", StatusAt: &old}, "mastered", &localDone},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			h.catalog.link("game-1", "1")
			tc.local.CanonicalGameID = "game-1"
			h.library.setLocalGame(tc.local)
			srv := &featureServer{features: tc.features, remote: []wireGame{tc.remote}}
			srv.install(h)
			if err := h.service.Sync(context.Background()); err != nil {
				t.Fatalf("Sync: %v", err)
			}
			got := h.library.gameOf("game-1")
			if got.Status != tc.wantStatus {
				t.Fatalf("status = %q, want %q", got.Status, tc.wantStatus)
			}
			switch {
			case tc.wantDate == nil && got.CompletedAt != nil:
				t.Fatalf("CompletedAt = %v, want nil", got.CompletedAt)
			case tc.wantDate != nil && (got.CompletedAt == nil || !got.CompletedAt.Equal(*tc.wantDate)):
				t.Fatalf("CompletedAt = %v, want %v", got.CompletedAt, tc.wantDate)
			}
		})
	}
}

func TestCompletionDateChangeIsPushedAndSettles(t *testing.T) {
	stamp := time.Now().UTC().Add(-48 * time.Hour).Truncate(time.Microsecond)
	first := stamp.Add(-72 * time.Hour)
	second := stamp.Add(-24 * time.Hour)
	h := newHarness(t)
	h.catalog.link("game-1", "1")
	h.library.setLocalGame(Game{CanonicalGameID: "game-1", Status: "completed", StatusAt: &stamp, CompletedAt: &first})
	srv := &featureServer{features: "completion"}
	srv.install(h)
	if err := h.service.Sync(context.Background()); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	puts := h.server.putCalls

	if err := h.service.Sync(context.Background()); err != nil {
		t.Fatalf("idle Sync: %v", err)
	}
	if h.server.putCalls != puts {
		t.Fatalf("an unchanged game pushed again (%d -> %d puts)", puts, h.server.putCalls)
	}

	newer := stamp.Add(time.Hour)
	h.library.setLocalGame(Game{CanonicalGameID: "game-1", Status: "completed", StatusAt: &newer, CompletedAt: &second})
	if err := h.service.Sync(context.Background()); err != nil {
		t.Fatalf("Sync after the edit: %v", err)
	}
	if h.server.putCalls != puts+1 {
		t.Fatalf("the date edit was not pushed (%d -> %d puts)", puts, h.server.putCalls)
	}
	if len(srv.remote) != 1 || srv.remote[0].CompletedAt == nil || !srv.remote[0].CompletedAt.Equal(second) {
		t.Fatalf("server holds %+v, want the edited date", srv.remote)
	}
}

func TestUpToDateWatchesCompletionOnlyWithTheFeature(t *testing.T) {
	stamp := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	done := stamp.Add(-time.Hour)
	other := stamp.Add(-2 * time.Hour)
	results := map[string]gameCompute{"1": {combined: Game{Status: "mastered", StatusAt: &stamp, CompletedAt: &done}}}
	st := syncState{Games: map[string]gameState{"1": {}}}

	tests := []struct {
		name   string
		feat   syncFeatures
		remote wireGame
		want   bool
	}{
		{"completion: everything equal", syncFeatures{completion: true}, wireGame{Status: "mastered", StatusAt: &stamp, CompletedAt: &done}, true},
		{"completion: remote has another date", syncFeatures{completion: true}, wireGame{Status: "mastered", StatusAt: &stamp, CompletedAt: &other}, false},
		{"completion: remote has no date", syncFeatures{completion: true}, wireGame{Status: "mastered", StatusAt: &stamp}, false},
		{"completion: remote holds the downgraded status", syncFeatures{completion: true}, wireGame{Status: "completed", StatusAt: &stamp, CompletedAt: &done}, false},
		{"no feature: remote holds completed", syncFeatures{}, wireGame{Status: "completed", StatusAt: &stamp}, true},
		{"no feature: remote still says mastered", syncFeatures{}, wireGame{Status: "mastered", StatusAt: &stamp}, false},
		{"no feature: a date the server never stores is ignored", syncFeatures{}, wireGame{Status: "completed", StatusAt: &stamp, CompletedAt: &other}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := upToDate(st, results, map[string]wireGame{"1": tc.remote}, tc.feat); got != tc.want {
				t.Fatalf("upToDate = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestFeatureAppearingLaterUpgradesTheServerCopy(t *testing.T) {
	stamp := time.Now().UTC().Add(-48 * time.Hour).Truncate(time.Microsecond)
	done := stamp.Add(-24 * time.Hour)
	h := newHarness(t)
	h.catalog.link("game-1", "1")
	h.library.setLocalGame(Game{CanonicalGameID: "game-1", Status: "mastered", StatusAt: &stamp, CompletedAt: &done})
	srv := &featureServer{}
	srv.install(h)
	if err := h.service.Sync(context.Background()); err != nil {
		t.Fatalf("Sync without the feature: %v", err)
	}
	puts := h.server.putCalls

	srv.features = "completion"
	if err := h.service.Sync(context.Background()); err != nil {
		t.Fatalf("Sync with the feature: %v", err)
	}
	if h.server.putCalls != puts+1 {
		t.Fatalf("the upgrade was not pushed (%d -> %d puts)", puts, h.server.putCalls)
	}
	games := rawGames(t, lastPutRaw(h))
	if games[0]["status"] != "mastered" || games[0]["completedAt"] == nil {
		t.Fatalf("PUT after the upgrade = %v", games[0])
	}
}
