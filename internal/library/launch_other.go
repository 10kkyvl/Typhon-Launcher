//go:build !windows

package library

import "errors"

func needsElevation(error) bool {
	return false
}

func startElevated(string, []string, string) (launched, error) {
	return launched{}, errors.New("запуск с правами администратора поддерживается только в Windows")
}
