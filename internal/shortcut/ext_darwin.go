//go:build darwin && !devmock

package shortcut

// На macOS ярлык — настоящий бандл приложения, то есть каталог, а не файл.
const shortcutExt = ".app"
