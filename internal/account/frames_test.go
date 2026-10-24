package account

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"typhon/internal/storage"
)

func centred(url string, hole float64) FrameImage {
	return FrameImage{URL: url, Hole: hole, HoleX: 0.5, HoleY: 0.5}
}

func TestFrameImageCenter(t *testing.T) {
	const base = "https://cdn.example.com/f.png"
	tests := []struct {
		name  string
		body  string
		wantX float64
		wantY float64
		err   error
	}{
		{"fields absent mean centre", `{"url":"` + base + `","hole":0.5}`, 0.5, 0.5, nil},
		{"offset kept", `{"url":"` + base + `","hole":0.5,"holeX":0.4,"holeY":0.62}`, 0.4, 0.62, nil},
		{"bounds", `{"url":"` + base + `","hole":0.5,"holeX":0.05,"holeY":0.95}`, 0.05, 0.95, nil},
		{"only x given", `{"url":"` + base + `","hole":0.5,"holeX":0.3}`, 0.3, 0.5, nil},
		{"explicit zero is invalid", `{"url":"` + base + `","hole":0.5,"holeX":0}`, 0, 0.5, ErrFrameCenter},
		{"below min", `{"url":"` + base + `","hole":0.5,"holeY":0.04}`, 0.5, 0.04, ErrFrameCenter},
		{"above max", `{"url":"` + base + `","hole":0.5,"holeX":1}`, 1, 0.5, ErrFrameCenter},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var img FrameImage
			if err := json.Unmarshal([]byte(tc.body), &img); err != nil {
				t.Fatal(err)
			}
			if img.HoleX != tc.wantX || img.HoleY != tc.wantY {
				t.Fatalf("center = %v,%v want %v,%v", img.HoleX, img.HoleY, tc.wantX, tc.wantY)
			}
			if err := ValidateFrameImage(img); !errors.Is(err, tc.err) {
				t.Fatalf("err = %v, want %v", err, tc.err)
			}
		})
	}
	nan := centred("https://cdn.example.com/f.png", 0.5)
	nan.HoleX = math.NaN()
	if err := ValidateFrameImage(nan); !errors.Is(err, ErrFrameCenter) {
		t.Fatalf("nan err = %v", err)
	}
	if err := json.Unmarshal([]byte(`{"url":1}`), &nan); err == nil {
		t.Fatal("malformed frame must fail to decode")
	}
}

func TestOwnedFrameCenter(t *testing.T) {
	var f OwnedFrame
	if err := json.Unmarshal([]byte(`{"id":"a","name":"G","url":"https://cdn.example.com/a.png","hole":0.7,"holeX":0.45,"holeY":0.55}`), &f); err != nil {
		t.Fatal(err)
	}
	if f.HoleX != 0.45 || f.HoleY != 0.55 || f.ID != "a" || f.Name != "G" {
		t.Fatalf("frame = %+v", f)
	}
	if err := json.Unmarshal([]byte(`{"id":"a","name":"G","url":"https://cdn.example.com/a.png","hole":0.7}`), &f); err != nil {
		t.Fatal(err)
	}
	if f.HoleX != 0.5 || f.HoleY != 0.5 {
		t.Fatalf("absent centre = %+v", f)
	}
	kept := sanitizeFrames([]OwnedFrame{{ID: "a", URL: "https://cdn.example.com/a.png", Hole: 0.7, HoleX: 0, HoleY: 0.5}})
	if len(kept) != 0 {
		t.Fatalf("zero centre kept: %+v", kept)
	}
}

func TestValidateFrameImage(t *testing.T) {
	tests := []struct {
		name    string
		img     FrameImage
		wantErr error
	}{
		{"https ok", centred("https://cdn.example.com/f.png", 0.65), nil},
		{"lower bound", centred("https://cdn.example.com/f.png", 0.3), nil},
		{"upper bound", centred("https://cdn.example.com/f.png", 1), nil},
		{"http loopback ok", centred("http://127.0.0.1:9000/f.png", 0.5), nil},
		{"http localhost ok", centred("http://localhost:9000/f.png", 0.5), nil},
		{"http public", centred("http://cdn.example.com/f.png", 0.5), ErrFrameURL},
		{"javascript", centred("javascript:alert(1)", 0.5), ErrFrameURL},
		{"data uri", centred("data:image/png;base64,AAAA", 0.5), ErrFrameURL},
		{"empty url", centred("", 0.5), ErrFrameURL},
		{"no host", centred("https:///f.png", 0.5), ErrFrameURL},
		{"hole zero", centred("https://cdn.example.com/f.png", 0), ErrFrameHole},
		{"hole too big", centred("https://cdn.example.com/f.png", 2), ErrFrameHole},
		{"hole below min", centred("https://cdn.example.com/f.png", 0.29), ErrFrameHole},
		{"hole nan", centred("https://cdn.example.com/f.png", math.NaN()), ErrFrameHole},
		{"hole inf", centred("https://cdn.example.com/f.png", math.Inf(1)), ErrFrameHole},
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
			[]OwnedFrame{{ID: "a", Name: "Gold", URL: "https://cdn.example.com/a.png", Hole: 0.7, HoleX: 0.5, HoleY: 0.5}}},
		{"empty array", `"frames":[]`, []OwnedFrame{}},
		{"missing field becomes empty list", `"x":1`, []OwnedFrame{}},
		{"null becomes empty list", `"frames":null`, []OwnedFrame{}},
		{"invalid frames dropped, valid kept", `"frames":[` +
			`{"id":"a","name":"Gold","url":"https://cdn.example.com/a.png","hole":0.7},` +
			`{"id":"b","name":"Bad","url":"http://cdn.example.com/b.png","hole":0.7},` +
			`{"id":"c","name":"Bad","url":"https://cdn.example.com/c.png","hole":2},` +
			`{"id":"d","name":"Bad","url":"javascript:alert(1)","hole":0.5},` +
			`{"id":"","name":"NoID","url":"https://cdn.example.com/e.png","hole":0.5}]`,
			[]OwnedFrame{{ID: "a", Name: "Gold", URL: "https://cdn.example.com/a.png", Hole: 0.7, HoleX: 0.5, HoleY: 0.5}}},
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
	in := cachedProfile{User: CurrentUser{ID: "u1", Frames: []OwnedFrame{{ID: "a", Name: "G", URL: "https://cdn.example.com/a.png", Hole: 0.5, HoleX: 0.5, HoleY: 0.5}}}}
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
