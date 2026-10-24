package typhonapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"typhon/internal/catalog"
)

func TestRelatedStatusesAndPayload(t *testing.T) {
	huge := `{"items":[{"id":"a","title":"` + strings.Repeat("x", 5<<20) + `","relation":"remake"}]}`
	cases := []struct {
		name     string
		status   int
		body     string
		wantErr  error
		anyErr   bool
		wantIDs  []string
		wantYear bool
	}{
		{name: "ok", status: 200, body: `{"items":[{"id":"r1","title":"Game Remake","gameType":"Remake","relation":"remake","releaseYear":2024,"platforms":["PC (Microsoft Windows)"],"coverUrl":"https://img.example/c.jpg"}]}`, wantIDs: []string{"r1"}, wantYear: true},
		{name: "not found", status: 404, body: `{"error":{"code":"not_found"}}`, wantErr: catalog.ErrRelatedNotFound},
		{name: "server error", status: 500, body: `{}`, wantErr: ErrUpstream},
		{name: "broken json", status: 200, body: `{"items":[`, wantErr: ErrUpstream},
		{name: "oversized body", status: 200, body: huge, wantErr: ErrUpstream},
		{name: "unknown relation dropped", status: 200, body: `{"items":[{"id":"r1","title":"A","relation":"sequel"},{"id":"r2","title":"B","relation":"port"}]}`, wantIDs: []string{"r2"}},
		{name: "empty id and title dropped", status: 200, body: `{"items":[{"id":"","title":"A","relation":"port"},{"id":"r3","title":" \u0000 ","relation":"port"}]}`, wantIDs: nil},
		{name: "self and duplicate dropped", status: 200, body: `{"items":[{"id":"self","title":"A","relation":"port"},{"id":"r4","title":"B","relation":"port"},{"id":"r4","title":"B","relation":"port"}]}`, wantIDs: []string{"r4"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotPath string
			client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				writeJSON(t, w, tc.status, tc.body)
			}), nil)
			got, err := client.Related(context.Background(), "self")
			if gotPath != "/v1/catalog/games/self/related" {
				t.Fatalf("path = %q", gotPath)
			}
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(tc.wantIDs) {
				t.Fatalf("items = %+v, want ids %v", got, tc.wantIDs)
			}
			for i, id := range tc.wantIDs {
				if got[i].ID != id {
					t.Fatalf("item %d id = %q, want %q", i, got[i].ID, id)
				}
			}
			if tc.wantYear && (got[0].ReleaseYear == nil || *got[0].ReleaseYear != 2024 || got[0].CoverURL == "") {
				t.Fatalf("fields lost: %+v", got[0])
			}
		})
	}
}

func TestRelatedRejectsEmptyID(t *testing.T) {
	client := newTestClient(t, http.NotFoundHandler(), nil)
	if _, err := client.Related(context.Background(), ""); !errors.Is(err, ErrBadRequest) {
		t.Fatalf("err = %v, want ErrBadRequest", err)
	}
}

func TestRelatedCleansFields(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, 200, `{"items":[{"id":"r1","title":"Ti\u0007tle","relation":"edition","releaseYear":99999,"platforms":["","PC (Microsoft Windows)"],"coverUrl":"javascript:alert(1)"}]}`)
	}), nil)
	got, err := client.Related(context.Background(), "self")
	if err != nil || len(got) != 1 {
		t.Fatalf("got %+v err %v", got, err)
	}
	g := got[0]
	if g.Title != "Title" || g.ReleaseYear != nil || g.CoverURL != "" || len(g.Platforms) != 1 {
		t.Fatalf("not cleaned: %+v", g)
	}
}
