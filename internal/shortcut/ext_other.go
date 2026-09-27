//go:build !darwin || devmock

package shortcut

// Формат Windows: его используют и настоящий Windows, и devmock-сборка, где
// ярлык подменён JSON-сайдкаром, но имя на диске остаётся привычным.
const shortcutExt = ".lnk"
