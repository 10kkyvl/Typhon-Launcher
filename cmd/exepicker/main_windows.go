//go:build windows

package main

import (
	"fmt"
	"os"
	"unsafe"

	"typhon/internal/exepicker"

	"golang.org/x/sys/windows"
)

const (
	ofnExplorer       = 0x00080000
	ofnFileMustExist  = 0x00001000
	ofnPathMustExist  = 0x00000800
	ofnNoChangeDir    = 0x00000008
	maxWindowsPathLen = 32768
)

type openFileName struct {
	structSize       uint32
	owner            windows.Handle
	instance         windows.Handle
	filter           *uint16
	customFilter     *uint16
	maxCustomFilter  uint32
	filterIndex      uint32
	file             *uint16
	maxFile          uint32
	fileTitle        *uint16
	maxFileTitle     uint32
	initialDirectory *uint16
	title            *uint16
	flags            uint32
	fileOffset       uint16
	fileExtension    uint16
	defaultExtension *uint16
	customData       uintptr
	hook             uintptr
	templateName     *uint16
	reserved         unsafe.Pointer
	reservedValue    uint32
	extraFlags       uint32
}

func utf16Ptr(value string) (*uint16, error) {
	if value == "" {
		return nil, nil
	}
	return windows.UTF16PtrFromString(value)
}

func run(args []string) error {
	if len(args) != 3 {
		return fmt.Errorf("usage: exepicker <result-file> <initial-path> <title>")
	}
	resultPath, initialPath, title := args[0], args[1], args[2]
	initialDir, initialFile := exepicker.InitialLocation(initialPath)
	file := make([]uint16, maxWindowsPathLen)
	copy(file, windows.StringToUTF16(initialFile))
	filter := make([]uint16, 0, 64)
	for _, part := range []string{"Executable files (*.exe)", "*.exe", "All files", "*.*", ""} {
		encoded := windows.StringToUTF16(part)
		filter = append(filter, encoded...)
	}
	dirPtr, err := utf16Ptr(initialDir)
	if err != nil {
		return err
	}
	titlePtr, err := utf16Ptr(title)
	if err != nil {
		return err
	}
	ext := windows.StringToUTF16("exe")
	of := openFileName{
		filter: filterPtr(filter), filterIndex: 1,
		//nolint:gosec // G115: buffer is fixed at maxWindowsPathLen (32768).
		file: &file[0], maxFile: uint32(len(file)),
		initialDirectory: dirPtr, title: titlePtr,
		flags:            ofnExplorer | ofnFileMustExist | ofnPathMustExist | ofnNoChangeDir,
		defaultExtension: &ext[0],
	}
	of.structSize = uint32(unsafe.Sizeof(of))
	//nolint:gosec // G103: Win32/COM ABI uses pointers to live typed buffers; the call is synchronous.
	ok, _, callErr := windows.NewLazySystemDLL("comdlg32.dll").NewProc("GetOpenFileNameW").Call(uintptr(unsafe.Pointer(&of)))
	if ok == 0 {
		//nolint:errcheck // Win32/COM result is carried by the return value or output buffer; last-error is not authoritative here.
		code, _, _ := windows.NewLazySystemDLL("comdlg32.dll").NewProc("CommDlgExtendedError").Call()
		if code == 0 {
			return nil // user cancelled
		}
		return fmt.Errorf("GetOpenFileNameW: 0x%x: %w", code, callErr)
	}
	selected := windows.UTF16ToString(file)
	//nolint:forbidigo,gosec // G703: local user-selected game/helper path; no network path input or privileged filesystem access. fresh private IPC file passed by the launcher, never persistent user state.
	return os.WriteFile(resultPath, []byte(selected), 0600)
}

func filterPtr(filter []uint16) *uint16 {
	if len(filter) == 0 {
		return nil
	}
	return &filter[0]
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		//nolint:forbidigo // this is the standalone helper main entry point.
		os.Exit(1)
	}
}
