package messaging

import (
	"encoding/json"
	"testing"
)

func TestPeerFrameFields(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		wantNil bool
	}{
		{"valid", `{"id":"u","avatarFrame":"custom:x","frameColor":"#fff","frameImage":{"url":"https://cdn.example.com/a.png","hole":0.5}}`, false},
		{"absent", `{"id":"u","avatarFrame":"ring"}`, true},
		{"http public", `{"id":"u","frameImage":{"url":"http://cdn.example.com/a.png","hole":0.5}}`, true},
		{"hole zero", `{"id":"u","frameImage":{"url":"https://cdn.example.com/a.png","hole":0}}`, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var c Conversation
			if err := json.Unmarshal([]byte(`{"peer":`+tc.json+`}`), &c); err != nil {
				t.Fatal(err)
			}
			if (c.Peer.FrameImage == nil) != tc.wantNil {
				t.Fatalf("frameImage = %+v", c.Peer.FrameImage)
			}
			if !tc.wantNil && (c.Peer.AvatarFrame != "custom:x" || c.Peer.FrameColor != "#fff") {
				t.Fatalf("peer = %+v", c.Peer)
			}
		})
	}
}
