//go:build darwin && !devmock

package install

import "errors"

// systemExecutable на Windows резолвит системный exe вроде msiexec по пути
// системы-хозяина. В бутыле такие программы живут за буквой C: и резолвятся
// уже внутри wine, поэтому снаружи резолвить нечего.
var errNoSystemExecutable = errors.New("системные исполняемые файлы вне бутыля недоступны")

func systemExecutable(string) (string, error) {
	return "", errNoSystemExecutable
}
