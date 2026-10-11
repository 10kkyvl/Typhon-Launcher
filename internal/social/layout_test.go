package social

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

func serveJSON(t *testing.T, body string) *client {
	t.Helper()
	return newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := io.WriteString(w, body); err != nil {
			t.Error(err)
		}
	})
}

func TestPublicProfileLayoutPassesThroughUntouched(t *testing.T) {
	config := `{"igdbId":1942,"caption":"лучшая <RPG> & точка"}`
	unknownConfig := `{"future":[1,{"a":null}],"html":"<b>"}`
	data := `{"game":{"igdbId":1942,"title":"The Witcher 3","coverUrl":"c","heroUrl":"","playtimeSeconds":7200,"status":"completed","completedAt":"2026-01-02T03:04:05Z"}}`
	body := `{"id":"u1","username":"alice","statusEmoji":"🎮","statusText":"фармлю боссов",` +
		`"layout":{"version":1,"blocks":[` +
		`{"id":"b1","type":"pinned","width":"full","config":` + config + `,"data":` + data + `},` +
		`{"id":"b2","type":"hologram","width":"third","config":` + unknownConfig + `},` +
		`{"id":"b3","type":"genres","width":"half","config":{},"data":{"error":"unresolved"}}]},` +
		`"autoGame":{"igdbId":7,"title":"Auto","coverUrl":"a","heroUrl":"h"}}`

	profile, err := serveJSON(t, body).profile(t.Context(), "alice")
	if err != nil {
		t.Fatal(err)
	}
	if profile.StatusEmoji != "🎮" || profile.StatusText != "фармлю боссов" {
		t.Fatalf("status = %q %q", profile.StatusEmoji, profile.StatusText)
	}
	if profile.AutoGame == nil || profile.AutoGame.IGDBID != 7 || profile.AutoGame.HeroURL != "h" {
		t.Fatalf("autoGame = %+v", profile.AutoGame)
	}
	if profile.Layout == nil || profile.Layout.Version != 1 || len(profile.Layout.Blocks) != 3 {
		t.Fatalf("layout = %+v", profile.Layout)
	}
	blocks := profile.Layout.Blocks
	if string(blocks[0].Config) != config || string(blocks[0].Data) != data {
		t.Fatalf("pinned block changed: config=%s data=%s", blocks[0].Config, blocks[0].Data)
	}
	if blocks[1].Type != "hologram" || string(blocks[1].Config) != unknownConfig || blocks[1].Data != nil {
		t.Fatalf("unknown block changed: %+v", blocks[1])
	}
	if string(blocks[2].Data) != `{"error":"unresolved"}` {
		t.Fatalf("error data = %s", blocks[2].Data)
	}

	encoded, err := json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	var again PublicProfile
	if err := json.Unmarshal(encoded, &again); err != nil {
		t.Fatal(err)
	}
	var want, got any
	if err := json.Unmarshal([]byte(unknownConfig), &want); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(again.Layout.Blocks[1].Config, &got); err != nil {
		t.Fatal(err)
	}
	if !jsonEqual(t, want, got) {
		t.Fatalf("unknown config after Wails round trip = %s", again.Layout.Blocks[1].Config)
	}
}

func jsonEqual(t *testing.T, a, b any) bool {
	t.Helper()
	left, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	right, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	return string(left) == string(right)
}

func TestPublicProfileWithoutNewFieldsStaysEmpty(t *testing.T) {
	profile, err := serveJSON(t, `{"id":"u1","username":"alice"}`).profile(t.Context(), "alice")
	if err != nil {
		t.Fatal(err)
	}
	if profile.Layout != nil || profile.AutoGame != nil || profile.StatusEmoji != "" || profile.StatusText != "" || profile.Card != nil {
		t.Fatalf("old backend produced new fields: %+v", profile)
	}
	encoded, err := json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &keys); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"layout", "autoGame", "statusEmoji", "statusText", "card"} {
		if _, ok := keys[key]; ok {
			t.Errorf("%s leaked into %s", key, encoded)
		}
	}
}

func TestPublicProfileEmptyLayoutKeepsBlocksArray(t *testing.T) {
	profile, err := serveJSON(t, `{"id":"u1","username":"a","layout":{"version":1,"blocks":null}}`).profile(t.Context(), "a")
	if err != nil {
		t.Fatal(err)
	}
	if profile.Layout == nil || profile.Layout.Blocks == nil || len(profile.Layout.Blocks) != 0 {
		t.Fatalf("layout = %+v", profile.Layout)
	}
}

func TestUserCardCardDecodesWhereverCardsAppear(t *testing.T) {
	card := `{"accent":"#67d8ef","avatarFrame":"neon","nameStyle":"glow","statusEmoji":"🎮","statusText":"hi","theme":"custom","customFrom":"#111111","customTo":"#222222","customAngle":0,"coverUrl":"https://c/x.webp","pinned":{"igdbId":1,"title":"G","coverUrl":"c","heroUrl":""}}`
	user := func(id string, withCard bool) string {
		s := `{"id":"` + id + `","username":"` + id + `","displayName":"","avatarUrl":""`
		if withCard {
			s += `,"card":` + card
		}
		return s + `}`
	}
	t.Run("friends page", func(t *testing.T) {
		body := `{"friends":[{"id":"f1","username":"f1","displayName":"","avatarUrl":"","since":"2026-01-01T00:00:00Z","card":` + card + `},` +
			`{"id":"f2","username":"f2","displayName":"","avatarUrl":"","since":"2026-01-01T00:00:00Z"}],` +
			`"incoming":[{"id":"i1","username":"i1","displayName":"","avatarUrl":"","createdAt":"2026-01-01T00:00:00Z","mutualCount":1,"commonCount":2,"card":{"accent":"#fff","avatarFrame":"ring","nameStyle":"plain"}}],"outgoing":[]}`
		page, err := serveJSON(t, body).friendsPage(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		first := page.Friends[0].Card
		if first == nil || first.AvatarFrame != "neon" || first.NameStyle != "glow" || first.CustomAngle == nil || *first.CustomAngle != 0 || first.Pinned == nil || first.Pinned.IGDBID != 1 || first.CoverURL != "https://c/x.webp" {
			t.Fatalf("friend card = %+v", first)
		}
		if page.Friends[1].Card != nil {
			t.Fatalf("old backend friend got a card: %+v", page.Friends[1].Card)
		}
		cosmetics := page.Incoming[0].Card
		if cosmetics == nil || cosmetics.AvatarFrame != "ring" || cosmetics.Theme != "" || cosmetics.Pinned != nil || cosmetics.CustomAngle != nil {
			t.Fatalf("cosmetics-only card = %+v", cosmetics)
		}
	})
	t.Run("feed author", func(t *testing.T) {
		body := `{"events":[{"id":1,"user":` + user("a", true) + `,"kind":"completed","game":{"igdbId":1,"title":"G","coverUrl":"","heroUrl":""},"createdAt":"2026-01-02T03:04:05Z","reactions":[],"mine":[],"note":""},` +
			`{"id":2,"user":` + user("b", false) + `,"kind":"completed","game":{"igdbId":1,"title":"G","coverUrl":"","heroUrl":""},"createdAt":"2026-01-02T03:04:05Z","reactions":[],"mine":[],"note":""}],"next":0}`
		page, err := serveJSON(t, body).feed(t.Context(), 0, 20)
		if err != nil {
			t.Fatal(err)
		}
		if page.Events[0].User.Card == nil || page.Events[0].User.Card.StatusText != "hi" {
			t.Fatalf("author card = %+v", page.Events[0].User.Card)
		}
		if page.Events[1].User.Card != nil {
			t.Fatalf("old backend author got a card")
		}
	})
	t.Run("blocks and mutual friends", func(t *testing.T) {
		blocks, err := serveJSON(t, `{"blocks":[`+user("x", true)+`,`+user("y", false)+`]}`).blocks(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if blocks[0].Card == nil || blocks[1].Card != nil {
			t.Fatalf("blocks = %+v", blocks)
		}
		profile, err := serveJSON(t, `{"id":"u","username":"u","mutualFriends":[`+user("m", true)+`]}`).profile(t.Context(), "u")
		if err != nil {
			t.Fatal(err)
		}
		if len(profile.MutualFriends) != 1 || profile.MutualFriends[0].Card == nil || profile.MutualFriends[0].Card.Accent != "#67d8ef" {
			t.Fatalf("mutual = %+v", profile.MutualFriends)
		}
	})
}
