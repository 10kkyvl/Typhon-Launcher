package social

import (
	"encoding/json"
	"testing"
)

func TestPlayerCardFrameImage(t *testing.T) {
	tests := []struct {
		name     string
		json     string
		wantURL  string
		wantNil  bool
		wantHole float64
	}{
		{"custom frame", `{"avatarFrame":"custom:x","frameImage":{"url":"https://cdn.example.com/a.png","hole":0.7}}`, "https://cdn.example.com/a.png", false, 0.7},
		{"no frame image", `{"avatarFrame":"ring"}`, "", true, 0},
		{"http public dropped", `{"avatarFrame":"custom:x","frameImage":{"url":"http://cdn.example.com/a.png","hole":0.7}}`, "", true, 0},
		{"javascript dropped", `{"frameImage":{"url":"javascript:alert(1)","hole":0.7}}`, "", true, 0},
		{"empty url dropped", `{"frameImage":{"url":"","hole":0.7}}`, "", true, 0},
		{"hole zero dropped", `{"frameImage":{"url":"https://cdn.example.com/a.png","hole":0}}`, "", true, 0},
		{"hole two dropped", `{"frameImage":{"url":"https://cdn.example.com/a.png","hole":2}}`, "", true, 0},
		{"loopback http kept", `{"frameImage":{"url":"http://127.0.0.1:8080/a.png","hole":0.4}}`, "http://127.0.0.1:8080/a.png", false, 0.4},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var card PlayerCardStyle
			if err := json.Unmarshal([]byte(tc.json), &card); err != nil {
				t.Fatal(err)
			}
			if tc.wantNil {
				if card.FrameImage != nil {
					t.Fatalf("frameImage = %+v, want nil", card.FrameImage)
				}
				return
			}
			if card.FrameImage == nil || card.FrameImage.URL != tc.wantURL || card.FrameImage.Hole != tc.wantHole {
				t.Fatalf("frameImage = %+v", card.FrameImage)
			}
		})
	}
}

func TestUserCardDecodesFrameInsideCard(t *testing.T) {
	var card UserCard
	body := `{"id":"u","username":"u","displayName":"U","avatarUrl":"","card":{"accent":"#fff","avatarFrame":"custom:x","nameStyle":"plain","frameImage":{"url":"https://cdn.example.com/a.png","hole":0.6}}}`
	if err := json.Unmarshal([]byte(body), &card); err != nil {
		t.Fatal(err)
	}
	if card.Card == nil || card.Card.FrameImage == nil || card.Card.FrameImage.Hole != 0.6 || card.Card.AvatarFrame != "custom:x" {
		t.Fatalf("card = %+v", card.Card)
	}
}

func TestPublicProfileAvatarFrameImage(t *testing.T) {
	tests := []struct {
		name    string
		extra   string
		wantNil bool
	}{
		{"valid", `,"avatarFrameImage":{"url":"https://cdn.example.com/a.png","hole":0.65}`, false},
		{"absent", ``, true},
		{"http public", `,"avatarFrameImage":{"url":"http://cdn.example.com/a.png","hole":0.65}`, true},
		{"bad hole", `,"avatarFrameImage":{"url":"https://cdn.example.com/a.png","hole":2}`, true},
		{"javascript", `,"avatarFrameImage":{"url":"javascript:1","hole":0.65}`, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var p PublicProfile
			if err := json.Unmarshal([]byte(`{"id":"u","username":"u"`+tc.extra+`}`), &p); err != nil {
				t.Fatal(err)
			}
			p = normalizeProfile(p)
			if (p.AvatarFrameImage == nil) != tc.wantNil {
				t.Fatalf("avatarFrameImage = %+v, wantNil %v", p.AvatarFrameImage, tc.wantNil)
			}
		})
	}
}
