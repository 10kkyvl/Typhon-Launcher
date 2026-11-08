package emulators

import (
	"context"
	"errors"
	"testing"

	"typhon/internal/metadata"
	"typhon/internal/uierr"
)

func TestOnCatalogComparesWholeNames(t *testing.T) {
	ps1, _ := Lookup(PS1)
	if ps1.OnCatalog([]string{"PlayStation 2", "PlayStation Portable", "PC (Microsoft Windows)"}) {
		t.Fatal("PS2 counted as PS1")
	}
	if !ps1.OnCatalog([]string{"PC (Microsoft Windows)", " playstation "}) {
		t.Fatal("PS1 not recognised")
	}
}

func TestFindCoverOutcomes(t *testing.T) {
	boom := errors.New("offline")
	cases := []struct {
		name     string
		platform string
		result   string
		err      error
		want     string
		code     string
	}{
		{"found", PS2, "https://x/c.jpg", nil, "https://x/c.jpg", ""},
		{"not found", PS2, "", metadata.ErrNoMatch, "", uierr.Code(ErrCoverNotFound)},
		{"network", PS2, "", boom, "", uierr.Code(ErrCoverLookup)},
		{"unknown platform", "gba", "", nil, "", uierr.Code(ErrUnknownPlatform)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var title string
			var ownConsole bool
			source := func(_ context.Context, query string, accept func([]string) bool) (string, error) {
				title, ownConsole = query, accept([]string{"PlayStation 2"})
				return c.result, c.err
			}
			got, err := FindCover(t.Context(), source, "Gran Turismo 4", c.platform)
			if got != c.want || uierr.Code(err) != c.code {
				t.Fatalf("FindCover = %q, %v", got, err)
			}
			if c.platform != "gba" && (title != "Gran Turismo 4" || !ownConsole) {
				t.Fatalf("query %q, console accepted %v", title, ownConsole)
			}
		})
	}
}

func TestFindCoverKeepsTheCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	source := func(ctx context.Context, _ string, _ func([]string) bool) (string, error) { return "", ctx.Err() }
	if _, err := FindCover(ctx, source, "Game", PS1); !errors.Is(err, context.Canceled) || uierr.Code(err) != "" {
		t.Fatalf("FindCover = %v", err)
	}
}
