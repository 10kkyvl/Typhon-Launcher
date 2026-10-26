//go:build !windows

package instance

import "context"

func systemAPI() api { return nil }

type noListener struct{}

func (noListener) Close() error { return nil }

func Listen(context.Context, string, func(args []string)) (Listener, error) {
	return noListener{}, nil
}
