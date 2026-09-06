//go:build !darwin || devmock

package install

// prepareRuntime готовит окружение запуска установленной игры. Везде, кроме
// macOS, готовить нечего.
func prepareRuntime(string, string) error { return nil }
