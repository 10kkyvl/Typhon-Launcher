//go:build windows

package instance

import "context"

// Takeover makes this process the first instance. A successor launcher started
// beside the running one by a seamless update calls it after the old process
// has exited; second launches then reach sink.
func Takeover(ctx context.Context, id string, sink func(args []string)) (Listener, error) {
	return takeover(ctx, systemAPI(), Listen, id, sink)
}
