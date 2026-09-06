//go:build !darwin || devmock

package install

import "context"

// prepareRuntime готовит окружение запуска установленной игры. Везде, кроме
// macOS, готовить нечего.
func prepareRuntime(context.Context, string, string) error { return nil }
