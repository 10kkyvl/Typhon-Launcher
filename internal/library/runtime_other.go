//go:build !darwin || devmock

package library

// prepareRuntime готовит окружение запуска игры. Везде, кроме macOS, готовить
// нечего: игра запускается системой напрямую.
func prepareRuntime(string, string) error { return nil }
