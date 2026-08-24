package feed

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
)

func loopbackClient() *http.Client {
	return newGuardedClient(guard{blocked: func(netip.Addr) bool { return false }})
}

const validFeedJSON = `{"name":"Test","version":1,"downloads":[{"title":"Game A","uri":"magnet:?xt=urn:btih:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}]}`

func TestFetchSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("ETag", "\"abc\"")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(validFeedJSON))
	}))
	defer srv.Close()

	res, err := Fetch(context.Background(), loopbackClient(), srv.URL, Conditional{})
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	if res.NotModified {
		t.Error("expected NotModified=false")
	}
	if len(res.Feed.Entries) != 1 {
		t.Fatalf("entries = %d", len(res.Feed.Entries))
	}
	if res.ETag != "\"abc\"" {
		t.Errorf("ETag = %q", res.ETag)
	}
	if res.Bytes == 0 {
		t.Error("expected Bytes > 0")
	}
}

func TestFetchNotModified(t *testing.T) {
	var gotIfNoneMatch, gotIfModifiedSince string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotIfNoneMatch = r.Header.Get("If-None-Match")
		gotIfModifiedSince = r.Header.Get("If-Modified-Since")
		w.Header().Set("ETag", "\"xyz\"")
		w.WriteHeader(http.StatusNotModified)
	}))
	defer srv.Close()

	cond := Conditional{ETag: "\"old-etag\"", LastModified: "Mon, 01 Jan 2024 00:00:00 GMT"}
	res, err := Fetch(context.Background(), loopbackClient(), srv.URL, cond)
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	if !res.NotModified {
		t.Error("expected NotModified=true")
	}
	if gotIfNoneMatch != cond.ETag {
		t.Errorf("If-None-Match = %q, want %q", gotIfNoneMatch, cond.ETag)
	}
	if gotIfModifiedSince != cond.LastModified {
		t.Errorf("If-Modified-Since = %q, want %q", gotIfModifiedSince, cond.LastModified)
	}
}

func TestFetchServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := Fetch(context.Background(), loopbackClient(), srv.URL, Conditional{})
	if err == nil {
		t.Fatal("expected error")
	}
	var statusErr *StatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("expected *StatusError, got %T: %v", err, err)
	}
	if statusErr.StatusCode != 500 {
		t.Errorf("StatusCode = %d", statusErr.StatusCode)
	}
}

func TestFetchBadContentType(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(validFeedJSON))
	}))
	defer srv.Close()

	_, err := Fetch(context.Background(), loopbackClient(), srv.URL, Conditional{})
	if !errors.Is(err, ErrBadContentType) {
		t.Errorf("got %v, want ErrBadContentType", err)
	}
}

func TestFetchContentLengthTooLarge(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", MaxBytes+1))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	_, err := Fetch(context.Background(), loopbackClient(), srv.URL, Conditional{})
	if !errors.Is(err, ErrTooLarge) {
		t.Errorf("got %v, want ErrTooLarge", err)
	}
}

func TestFetchBodyTooLargeWithoutContentLength(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		chunk := make([]byte, 1<<20)
		for i := range chunk {
			chunk[i] = ' '
		}
		written := 0
		for written <= MaxBytes {
			n, werr := w.Write(chunk)
			if werr != nil {
				return
			}
			written += n
			if flusher != nil {
				flusher.Flush()
			}
		}
	}))
	defer srv.Close()

	_, err := Fetch(context.Background(), loopbackClient(), srv.URL, Conditional{})
	if !errors.Is(err, ErrTooLarge) {
		t.Errorf("got %v, want ErrTooLarge", err)
	}
}

func TestFetchRedirectLoop(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, srv.URL+"/next", http.StatusFound)
	}))
	defer srv.Close()

	_, err := Fetch(context.Background(), loopbackClient(), srv.URL, Conditional{})
	if err == nil {
		t.Fatal("expected redirect loop error")
	}
	if !strings.Contains(err.Error(), "редирект") {
		t.Errorf("error message doesn't mention redirects: %v", err)
	}
}

func TestValidateURL(t *testing.T) {
	cases := []struct {
		in      string
		wantErr error
	}{
		{"ftp://example.com/feed.json", ErrBadScheme},
		{"file:///etc/passwd", ErrBadScheme},
		{"https://example.com/feed.json", nil},
		{"http://example.com/feed.json", nil},
	}
	for _, c := range cases {
		_, err := ValidateURL(c.in)
		if c.wantErr != nil {
			if !errors.Is(err, c.wantErr) {
				t.Errorf("%s: got %v, want %v", c.in, err, c.wantErr)
			}
		} else if err != nil {
			t.Errorf("%s: unexpected error %v", c.in, err)
		}
	}
}
