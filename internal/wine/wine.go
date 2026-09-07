// Package wine запускает windows-программы на macOS через CrossOver.
// Пакет ничего не знает про игры, библиотеку и установку: он умеет найти
// рантайм, завести бутыль, запустить в нём команду, перечислить и убить
// процессы, перевести путь между native и windows.
package wine

import "errors"

// ErrNotInstalled — CrossOver на машине не найден или установлен неполно.
var ErrNotInstalled = errors.New("wine: CrossOver не найден")

// Runtime — пути до CLI установленного CrossOver.
type Runtime struct {
	Root       string
	CxBottle   string
	CxStart    string
	WineServer string
	Version    string
}
