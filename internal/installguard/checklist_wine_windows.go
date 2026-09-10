//go:build windows

package installguard

import (
	"encoding/binary"
	"golang.org/x/sys/windows"
	"runtime"
	"unsafe"
)

var collectControlProperties = windows.NewCallback(func(hwnd, name, data, param uintptr) uintptr {
	//nolint:gosec // G103: Win32/COM ABI uses pointers to live typed buffers; the call is synchronous.
	value, ok := callbackLists.Load(param)
	if !ok {
		return 0
	}
	values, ok := value.(*[]uintptr)
	if !ok {
		return 0
	}
	*values = append(*values, data)
	return 1
})

// Wine's standard accessible list object rejects child IDs, so Inno's MSAA
// wrapper cannot return Checked. Read only the validated Delphi TItemState;
// changes still go through the control's keyboard handler, never memory writes.
// Layout: Inno Components/NewCheckListBox.pas (5.5.9 and 6.x).
func wineChecklistState(hwnd, index, count uintptr) (int64, bool) {
	if windows.NewLazySystemDLL("ntdll.dll").NewProc("wine_get_version").Find() != nil {
		return 0, false
	}
	var pid uint32
	//nolint:errcheck,gosec // Win32/COM result is carried by the return value or output buffer; last-error is not authoritative here. G104: native result/output is used; last-error is not authoritative, cleanup is best effort.
	windowPID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_INFORMATION|windows.PROCESS_VM_READ, false, pid)
	if err != nil {
		return 0, false
	}
	//nolint:errcheck // best-effort release of an owned native resource on exit.
	defer windows.CloseHandle(process)
	var wow bool
	if err := windows.IsWow64Process(process, &wow); err != nil {
		return 0, false
	}
	width := 8
	if wow || unsafe.Sizeof(uintptr(0)) == 4 {
		width = 4
	}
	read := func(address uintptr, size int) []byte {
		if address < 0x10000 || size <= 0 || size > 16384 {
			return nil
		}
		b := make([]byte, size)
		if windows.ReadProcessMemory(process, address, &b[0], uintptr(size), nil) != nil {
			return nil
		}
		return b
	}
	number := func(b []byte) uintptr {
		if len(b) < width {
			return 0
		}
		if width == 4 {
			return uintptr(binary.LittleEndian.Uint32(b))
		}
		return uintptr(binary.LittleEndian.Uint64(b))
	}
	// RTTI class name and the immediately following instance size locate fields
	// without relying on a particular Delphi release's negative VMT offset.
	classSize := func(object uintptr, want string) int {
		vmt := number(read(object, width))
		if vmt < 0x10000 {
			return 0
		}
		meta := read(vmt-256, 256)
		if meta == nil {
			return 0
		}
		for i := 0; i+2*width <= len(meta); i += width {
			name := read(number(meta[i:]), len(want)+1)
			if name != nil && int(name[0]) == len(want) && string(name[1:]) == want {
				size := number(meta[i+width:])
				if size >= uintptr(width) && size <= 16384 {
					return int(size)
				}
			}
		}
		return 0
	}
	var objects []uintptr
	key, release := collectList(&objects)
	defer release()
	//nolint:errcheck,gosec // Win32/COM result is carried by the return value or output buffer; last-error is not authoritative here. G104: native result/output is used; last-error is not authoritative, cleanup is best effort.
	user32.NewProc("EnumPropsExW").Call(hwnd, collectControlProperties, key)
	runtime.KeepAlive(objects)
	for _, object := range objects {
		size := classSize(object, "TNewCheckListBox")
		if size == 0 {
			continue
		}
		fields := read(object, size)
		if fields == nil {
			continue
		}
		for offset := width; offset+width <= len(fields); offset += width {
			list := number(fields[offset:])
			if classSize(list, "TList") == 0 {
				continue
			}
			header := read(list, width*2+8)
			if header == nil {
				continue
			}
			n := binary.LittleEndian.Uint32(header[width*2:])
			capacity := binary.LittleEndian.Uint32(header[width*2+4:])
			if uintptr(n) != count || capacity < n || capacity > 32768 {
				continue
			}
			items := read(number(header[width:]), int(count)*width)
			if items == nil {
				continue
			}
			state := int64(0)
			valid := true
			for i := uintptr(0); i < count; i++ {
				item := number(items[int(i)*width:])
				if classSize(item, "TItemState") == 0 {
					valid = false
					break
				}
				// VMT; six byte-sized flags; aligned Obj pointer; byte-sized State.
				stateOffset := width * 3
				if width == 4 {
					stateOffset = 16
				}
				values := read(item, stateOffset+1)
				if values == nil || values[width] > 1 || values[width+4] > 2 || values[stateOffset] > 2 {
					valid = false
					break
				}
				if i == index {
					if values[width+4] != 1 {
						valid = false
						break
					}
					if values[stateOffset] == 1 {
						state |= 0x10
					}
					if values[width] == 0 {
						state |= 1
					}
				}
			}
			if valid {
				return state, true
			}
		}
	}
	return 0, false
}
