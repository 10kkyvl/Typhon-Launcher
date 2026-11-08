package instance

import (
	"context"
	"errors"
	"fmt"
)

// ErrNotFirst means another launcher became the first instance between the
// old process exiting and the successor claiming the role.
var ErrNotFirst = errors.New("another launcher took the single instance first")

type listenFunc func(ctx context.Context, id string, sink func(args []string)) (Listener, error)

// takeover is the Windows claim: the early mutex first, then the receiver
// window second launches are forwarded to. It is the same pair a first
// launcher takes in forward and Listen, so a later launch finds the successor.
func takeover(ctx context.Context, sys api, listen listenFunc, id string, sink func(args []string)) (Listener, error) {
	if id == "" {
		return nil, ErrNoID
	}
	first, err := sys.Claim(earlyPrefix + id + earlySuffix)
	if err != nil {
		return nil, fmt.Errorf("claim the first instance: %w", err)
	}
	if !first {
		return nil, ErrNotFirst
	}
	l, err := listen(ctx, id, sink)
	if err != nil {
		return nil, fmt.Errorf("listen for second launches: %w", err)
	}
	return l, nil
}
