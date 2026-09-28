package platform

import (
	"errors"
	"fmt"
	"syscall"

	"golang.org/x/sys/windows"
)

const asfwAny = 0xFFFFFFFF

var (
	user32                       = windows.NewLazySystemDLL("user32.dll")
	procAllowSetForegroundWindow = user32.NewProc("AllowSetForegroundWindow")
)

var errAllowForeground = errors.New("AllowSetForegroundWindow завершился с ошибкой")

// Право вывести окно на передний план есть у второго экземпляра, которого
// запустил ярлык, но он лишь пересылает запрос. У экземпляра в трее, который
// на самом деле запускает игру или показывает окно, такого права нет, и
// Windows открывает игру позади других окон без фокуса. Передача права любому
// процессу покрывает оба случая до следующего ввода пользователя.
func AllowForegroundHandoff() error {
	if err := procAllowSetForegroundWindow.Find(); err != nil {
		return fmt.Errorf("user32.AllowSetForegroundWindow: %w", err)
	}
	r1, _, callErr := syscall.SyscallN(procAllowSetForegroundWindow.Addr(), asfwAny)
	if r1 != 0 {
		return nil
	}
	if errors.Is(callErr, windows.ERROR_ACCESS_DENIED) {
		return ErrNoForegroundRight
	}
	if callErr != 0 {
		return callErr
	}
	return errAllowForeground
}
