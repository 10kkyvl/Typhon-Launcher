package redact

import (
	"errors"
	"net/url"
	"strings"
	"testing"
)

const magnetURI = "magnet:?xt=urn:btih:a748597437835a2fd0d2e06f8edd86fee316a84d&dn=Startup+Panic&tr=udp%3A%2F%2Ftracker.example%3A80"

func TestURL(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"https://feed.example/list.json?token=s3cret", "https://feed.example"},
		{"http://user:pass@feed.example:8443/a/b", "http://feed.example"},
		{"https://feed.example", "https://feed.example"},
		{"not a url at all", Hidden},
		{"://broken", Hidden},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			if got := URL(c.in); got != c.want {
				t.Fatalf("URL(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestSource(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"magnet", magnetURI, Magnet},
		{"magnet uppercase", "MAGNET:?xt=urn:btih:a748597437835a2fd0d2e06f8edd86fee316a84d", Magnet},
		{"http", "http://feed.example/x.torrent?token=s3cret", "http://feed.example"},
		{"windows path", `D:\Downloads\game.torrent`, File},
		{"unix path", "/home/egor/game.torrent", File},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Source(c.in)
			if got != c.want {
				t.Fatalf("Source(%q) = %q, want %q", c.in, got, c.want)
			}
			if strings.Contains(got, "btih") || strings.Contains(got, "s3cret") {
				t.Fatalf("Source(%q) leaked sensitive data: %q", c.in, got)
			}
		})
	}
}

func TestErrorStripsRequestURL(t *testing.T) {
	inner := errors.New("connection refused")
	err := Error(&url.Error{
		Op:  "Get",
		URL: "https://feed.example/list.json?token=s3cret",
		Err: inner,
	})
	if !errors.Is(err, inner) {
		t.Fatalf("unwrapping broke: %v", err)
	}
	for _, leak := range []string{"s3cret", "token", "list.json"} {
		if strings.Contains(err.Error(), leak) {
			t.Fatalf("error leaks %q: %v", leak, err)
		}
	}
	if !strings.Contains(err.Error(), "feed.example") {
		t.Fatalf("error lost the host: %v", err)
	}
}

func TestErrorPassesThroughOtherErrors(t *testing.T) {
	if got := Error(nil); got != nil {
		t.Fatalf("Error(nil) = %v, want nil", got)
	}
	plain := errors.New("no route to host")
	if got := Error(plain); !errors.Is(got, plain) {
		t.Fatalf("Error(%v) = %v, want the same error", plain, got)
	}
}
