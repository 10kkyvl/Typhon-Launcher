package emulators

import (
	"context"
	"errors"
	"fmt"

	"typhon/internal/metadata"
	"typhon/internal/uierr"
)

// CoverSource is metadata.Service.FindCover.
type CoverSource func(ctx context.Context, title string, accept func(platforms []string) bool) (string, error)

// FindCover takes a catalog cover only when the title and the console agree: a
// wrong cover is worse than none.
func FindCover(ctx context.Context, source CoverSource, title, platformID string) (string, error) {
	target, ok := Lookup(platformID)
	if !ok {
		return "", fmt.Errorf("%w: %q", ErrUnknownPlatform, platformID)
	}
	cover, err := source(ctx, title, target.OnCatalog)
	switch {
	case err == nil:
		return cover, nil
	case errors.Is(err, metadata.ErrNoMatch):
		return "", ErrCoverNotFound
	case ctx.Err() != nil:
		return "", ctx.Err()
	default:
		return "", uierr.Wrap(uierr.Code(ErrCoverLookup), fmt.Errorf("поиск обложки: %w", err))
	}
}
