package reviews

import (
	"encoding/json"
	"testing"
)

func TestAuthorFrameFields(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		wantNil bool
	}{
		{"valid", `{"id":"u","avatarFrame":"custom:x","frameColor":"#fff","frameImage":{"url":"https://cdn.example.com/a.png","hole":0.5}}`, false},
		{"absent", `{"id":"u","avatarFrame":"ring"}`, true},
		{"javascript", `{"id":"u","frameImage":{"url":"javascript:1","hole":0.5}}`, true},
		{"hole too big", `{"id":"u","frameImage":{"url":"https://cdn.example.com/a.png","hole":1.5}}`, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var r Review
			if err := json.Unmarshal([]byte(`{"author":`+tc.json+`}`), &r); err != nil {
				t.Fatal(err)
			}
			if (r.Author.FrameImage == nil) != tc.wantNil {
				t.Fatalf("frameImage = %+v", r.Author.FrameImage)
			}
			if !tc.wantNil && (r.Author.AvatarFrame != "custom:x" || r.Author.FrameColor != "#fff") {
				t.Fatalf("author = %+v", r.Author)
			}
		})
	}
}
