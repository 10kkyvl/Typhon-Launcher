package matchprogress

import "context"

type key struct{}

// With carries fn through catalog.ResolveSourceQueries down to the release
// matcher, which cannot take a callback without changing the catalog interface.
func With(ctx context.Context, fn func(done, total int)) context.Context {
	return context.WithValue(ctx, key{}, fn)
}

// Report tells the caller of the matcher how many queries are answered so far.
func Report(ctx context.Context, done, total int) {
	if fn, ok := ctx.Value(key{}).(func(done, total int)); ok {
		fn(done, total)
	}
}
