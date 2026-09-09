//go:build !darwin || devmock

package library

import "context"

// prepareRuntime готовит окружение запуска игры. Везде, кроме macOS, готовить
// нечего: игра запускается системой напрямую.
func prepareRuntime(context.Context, launch) error { return nil }
