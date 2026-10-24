package account

import (
	"context"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"typhon/internal/storage"
)

func TestValidateFrameImage(t *testing.T) {
	tests := []struct {
		name    string
		img     FrameImage
		wantErr error
	}{
		{"https ok", FrameImage{URL: "https://cdn.example.com/f.png", Hole: 0.65}, nil},
		{"lower bound", FrameImage{URL: "https://cdn.example.com/f.png", Hole: 0.3}, nil},
		{"upper bound", FrameImage{URL: "https://cdn.example.com/f.png", Hole: 1}, nil},
		{"http loopback ok", FrameImage{URL: "http://127.0.0.1:9000/f.png", Hole: 0.5}, nil},
		{"http localhost ok", FrameImage{URL: "http://localhost:9000/f.png", Hole: 0.5}, nil},
		{"http public", FrameImage{URL: "http://cdn.example.com/f.png", Hole: 0.5}, ErrFrameURL},
		{"javascript", FrameImage{URL: "javascript:alert(1)", Hole: 0.5}, ErrFrameURL},
		{"data uri", FrameImage{URL: "data:image/png;base64,AAAA", Hole: 0.5}, ErrFrameURL},
		{"empty url", FrameImage{URL: "", Hole: 0.5}, ErrFrameURL},
		{"no host", FrameImage{URL: "https:///f.png", Hole: 0.5}, ErrFrameURL},
		{"hole zero", FrameImage{URL: "https://cdn.example.com/f.png", Hole: 0}, ErrFrameHole},
		{"hole too big", FrameImage{URL: "https://cdn.example.com/f.png", Hole: 2}, ErrFrameHole},
		{"hole below min", FrameImage{URL: "https://cdn.example.com/f.png", Hole: 0.29}, ErrFrameHole},
		{"hole nan", FrameImage{URL: "https://cdn.example.com/f.png", Hole: math.NaN()}, ErrFrameHole},
		{"hole inf", FrameImage{URL: "https://cdn.example.com/f.png", Hole: math.Inf(1)}, ErrFrameHole},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateFrameImage(tc.img)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if got := SanitizeFrameImage(&tc.img); (got == nil) != (tc.wantErr != nil) {
				t.Fatalf("sanitize = %v, want nil=%v", got, tc.wantErr != nil)
			}
		})
	}
	if SanitizeFrameImage(nil) != nil {
		t.Fatal("nil frame must stay nil")
	}
}

func TestClientMeDecodesFrames(t *testing.T) {
	tests := []struct {
		name string
		body string
		want []OwnedFrame
	}{
		{"granted frame", `"frames":[{"id":"a","name":"Gold","url":"https://cdn.example.com/a.png","hole":0.7}]`,
			[]OwnedFrame{{ID: "a", Name: "Gold", URL: "https://cdn.example.com/a.png", Hole: 0.7}}},
		{"empty array", `"frames":[]`, []OwnedFrame{}},
		{"missing field becomes empty list", `"x":1`, []OwnedFrame{}},
		{"null becomes empty list", `"frames":null`, []OwnedFrame{}},
		{"invalid frames dropped, valid kept", `"frames":[` +
			`{"id":"a","name":"Gold","url":"https://cdn.example.com/a.png","hole":0.7},` +
			`{"id":"b","name":"Bad","url":"http://cdn.example.com/b.png","hole":0.7},` +
			`{"id":"c","name":"Bad","url":"https://cdn.example.com/c.png","hole":2},` +
			`{"id":"d","name":"Bad","url":"javascript:alert(1)","hole":0.5},` +
			`{"id":"","name":"NoID","url":"https://cdn.example.com/e.png","hole":0.5}]`,
			[]OwnedFrame{{ID: "a", Name: "Gold", URL: "https://cdn.example.com/a.png", Hole: 0.7}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if _, err := w.Write([]byte(`{"id":"u1","username":"egor",` + tc.body + `}`)); err != nil {
					t.Errorf("write: %v", err)
				}
			}))
			defer srv.Close()
			user, err := newTestClient(t, srv.URL, tokenOK("t")).Me(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if len(user.Frames) != len(tc.want) || user.Frames == nil {
				t.Fatalf("frames = %#v, want %#v", user.Frames, tc.want)
			}
			for i := range tc.want {
				if user.Frames[i] != tc.want[i] {
					t.Fatalf("frames[%d] = %+v, want %+v", i, user.Frames[i], tc.want[i])
				}
			}
		})
	}
}

func TestUpdateProfileFrameNotGranted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		if _, err := w.Write([]byte(`{"error":{"code":"frame_not_granted","field":"profile","message":"no"}}`)); err != nil {
			t.Errorf("write: %v", err)
		}
	}))
	defer srv.Close()
	_, err := newTestClient(t, srv.URL, tokenOK("t")).UpdateProfile(context.Background(), Patch{})
	var apiErr *Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v, want *Error", err)
	}
	if CodeFrameNotGranted != "frame_not_granted" || apiErr.Code != CodeFrameNotGranted || apiErr.Field != "profile" {
		t.Fatalf("error = %+v", apiErr)
	}
}

func TestLoadProfileFrames(t *testing.T) {
	tests := []struct {
		name string
		user map[string]any
		want int
	}{
		{"old cache without frames", map[string]any{"id": "u1", "username": "old"}, 0},
		{"valid frame kept", map[string]any{"id": "u1", "frames": []any{
			map[string]any{"id": "a", "name": "G", "url": "https://cdn.example.com/a.png", "hole": 0.5}}}, 1},
		{"tampered cache frame dropped", map[string]any{"id": "u1", "frames": []any{
			map[string]any{"id": "a", "name": "G", "url": "javascript:x", "hole": 0.5}}}, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "profile.json")
			if err := storage.Save(path, profileVersion, map[string]any{"user": tc.user}); err != nil {
				t.Fatal(err)
			}
			loaded, err := loadProfile(path)
			if err != nil {
				t.Fatal(err)
			}
			if len(loaded.User.Frames) != tc.want {
				t.Fatalf("frames = %+v, want %d", loaded.User.Frames, tc.want)
			}
		})
	}
}

func TestCachedProfileKeepsFrames(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profile.json")
	in := cachedProfile{User: CurrentUser{ID: "u1", Frames: []OwnedFrame{{ID: "a", Name: "G", URL: "https://cdn.example.com/a.png", Hole: 0.5}}}}
	if err := storage.Save(path, profileVersion, in); err != nil {
		t.Fatal(err)
	}
	out, err := loadProfile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.User.Frames) != 1 || out.User.Frames[0] != in.User.Frames[0] {
		t.Fatalf("frames = %+v", out.User.Frames)
	}
}
