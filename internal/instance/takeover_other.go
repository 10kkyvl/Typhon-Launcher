//go:build !windows && !darwin

package instance

import "context"

// Takeover has nothing to claim where the platform's own single-instance
// handling is not in use.
func Takeover(context.Context, string, func(args []string)) (Listener, error) {
	return noListener{}, nil
}
