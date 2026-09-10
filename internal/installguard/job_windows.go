//go:build windows

package installguard

import (
	"encoding/binary"
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// RunJob contains all installer descendants, including an extracted setup.tmp
// whose loader has already exited. Wine's own idle shell services do not keep
// a completed installation alive and are preserved when the job is released.
func RunJob(args []string, cancelFile string, hidden bool, compatibility bool) (code int, stopped bool, err error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, true, err
	}
	//nolint:errcheck // best-effort release of an owned native resource on exit.
	defer windows.CloseHandle(job)
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	//nolint:gosec // G103: Win32/COM ABI uses pointers to live typed buffers; the call is synchronous.
	if _, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		return 0, true, err
	}
	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = syscall.EscapeArg(arg)
	}
	if len(args) > 1 && strings.HasPrefix(args[len(args)-1], "/D=") {
		quoted[len(args)-1] = args[len(args)-1]
	}
	line, err := windows.UTF16PtrFromString(strings.Join(quoted, " "))
	if err != nil {
		return 0, true, err
	}
	si := windows.StartupInfo{Cb: uint32(unsafe.Sizeof(windows.StartupInfo{}))}
	if hidden {
		si.Flags = windows.STARTF_USESHOWWINDOW
		si.ShowWindow = windows.SW_HIDE
	}
	var pi windows.ProcessInformation
	if err = windows.CreateProcess(nil, line, nil, nil, false, windows.CREATE_SUSPENDED, nil, nil, &si, &pi); err != nil {
		return 0, true, err
	}
	//nolint:errcheck // best-effort release of an owned native resource on exit.
	defer windows.CloseHandle(pi.Process)
	//nolint:errcheck // best-effort release of an owned native resource on exit.
	defer windows.CloseHandle(pi.Thread)
	if err = windows.AssignProcessToJobObject(job, pi.Process); err != nil {
		stopErr := windows.TerminateProcess(pi.Process, 1)
		wait, waitErr := windows.WaitForSingleObject(pi.Process, 5000)
		return 0, waitErr == nil && wait == windows.WAIT_OBJECT_0, errors.Join(err, stopErr, waitErr)
	}
	if _, err = windows.ResumeThread(pi.Thread); err != nil {
		return 0, false, err
	}
	type child struct {
		handle           windows.Handle
		patched, service bool
	}
	children := map[uint32]*child{}
	defer func() {
		for _, p := range children {
			//nolint:errcheck,gosec // best-effort release of an owned native resource on exit. G104: native result/output is used; last-error is not authoritative, cleanup is best effort.
			windows.CloseHandle(p.handle)
		}
	}()
	cancelled := false
	deadline := time.Time{}
	lastTrace := time.Time{}
	for {
		ids, err := jobProcessIDs(job)
		if err != nil {
			return 0, false, err
		}
		payloads := 0
		active := 0
		trace := os.Getenv("TYPHON_INSTALLGUARD_TRACE") == "1" && time.Since(lastTrace) > 5*time.Second
		for _, id := range ids {
			p := children[id]
			if p == nil {
				access := uint32(windows.PROCESS_QUERY_INFORMATION | windows.PROCESS_VM_READ | windows.PROCESS_VM_WRITE | windows.PROCESS_VM_OPERATION | windows.PROCESS_SUSPEND_RESUME)
				h, e := windows.OpenProcess(access, false, id)
				if e != nil {
					if trace {
						fmt.Fprintf(os.Stderr, "installguard pid=%d open: %v\n", id, e)
					}
					active++
					payloads++
					continue
				}
				p = &child{handle: h}
				children[id] = p
			}
			var childExit uint32
			if windows.GetExitCodeProcess(p.handle, &childExit) == nil && childExit != 259 {
				continue
			}
			active++
			// The image name may be unavailable while Wine initializes a process.
			p.service = id != pi.ProcessId && isWineService(processImage(p.handle))
			if trace {
				fmt.Fprintf(os.Stderr, "installguard pid=%d image=%q exit=%d service=%t\n", id, processImage(p.handle), childExit, p.service)
			}
			if p.service {
				continue
			}
			payloads++
			if compatibility && !p.patched && !cancelled {
				var wow bool
				if e := windows.IsWow64Process(p.handle, &wow); e != nil {
					continue
				}
				if !wow {
					p.patched = true
					continue
				}
				if base := wow64Ntdll(p.handle); base != 0 {
					//nolint:errcheck // Win32/COM result is carried by the return value or output buffer; last-error is not authoritative here.
					status, _, _ := windows.NewLazySystemDLL("ntdll.dll").NewProc("NtSuspendProcess").Call(uintptr(p.handle))
					if status == 0 {
						e := installAllocationGuard(p.handle, base)
						//nolint:errcheck // Win32/COM result is carried by the return value or output buffer; last-error is not authoritative here.
						resumed, _, _ := windows.NewLazySystemDLL("ntdll.dll").NewProc("NtResumeProcess").Call(uintptr(p.handle))
						if e != nil {
							return 0, false, e
						}
						if resumed != 0 {
							return 0, false, fmt.Errorf("resume installer: %x", resumed)
						}
						p.patched = true
					}
				}
			}
		}
		if trace {
			lastTrace = time.Now()
		}
		var exit uint32
		if err = windows.GetExitCodeProcess(pi.Process, &exit); err != nil {
			return 0, false, err
		}
		if active == 0 {
			return int(exit), true, nil
		}
		if payloads == 0 && exit != 259 && !cancelled {
			// Only Wine's persistent explorer/rpcss remain. They aren't game writers.
			limits.BasicLimitInformation.LimitFlags = 0
			//nolint:gosec // G103: Win32/COM ABI uses pointers to live typed buffers; the call is synchronous.
			if _, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
				return 0, false, err
			}
			return int(exit), true, nil
		}
		if !cancelled && cancelFile != "" {
			if _, e := os.Stat(cancelFile); e == nil {
				if err = windows.TerminateJobObject(job, 1); err != nil {
					return 0, false, err
				}
				cancelled = true
				deadline = time.Now().Add(3 * time.Second)
			}
		}
		if cancelled && time.Now().After(deadline) {
			return 0, false, fmt.Errorf("installer job did not stop")
		}
		delay := 50 * time.Millisecond
		if compatibility {
			delay = 10 * time.Millisecond
		}
		//nolint:forbidigo // bounded polling of native processes/windows across process boundaries; cancellation is checked each iteration.
		time.Sleep(delay)
	}
}
func jobProcessIDs(job windows.Handle) ([]uint32, error) {
	for capacity := 128; capacity <= 32768; capacity *= 2 {
		buffer := make([]uintptr, capacity+2)
		//nolint:gosec // G103: Win32/COM ABI uses pointers to live typed buffers; the call is synchronous.
		err := windows.QueryInformationJobObject(job, 3, uintptr(unsafe.Pointer(&buffer[0])), uint32(len(buffer))*uint32(unsafe.Sizeof(uintptr(0))), nil)
		if errors.Is(err, windows.ERROR_MORE_DATA) {
			continue
		}
		if err != nil {
			return nil, err
		}
		//nolint:gosec // G103: Win32/COM ABI uses pointers to live typed buffers; the call is synchronous.
		raw := unsafe.Slice((*byte)(unsafe.Pointer(&buffer[0])), len(buffer)*int(unsafe.Sizeof(uintptr(0))))
		count := binary.LittleEndian.Uint32(raw[4:])
		if count > uint32(capacity) {
			continue
		}
		ids := make([]uint32, count)
		for i := range ids {
			offset := 8 + i*int(unsafe.Sizeof(uintptr(0)))
			ids[i] = binary.LittleEndian.Uint32(raw[offset:])
		}
		return ids, nil
	}
	return nil, fmt.Errorf("installer process list too large")
}

// Reject the unarc probe that rounds up to >=2 GiB (Wine bug 50824).
// Wine rounds the requested size to 4 KiB pages before the buggy allocation.
// Do not carve up usable address space: real decompression still needs large
// buffers. Patch the verified x86 syscall stub while the target process is suspended, as soon as its
// 32-bit loader is ready (before this installer reaches decompression).
func installAllocationGuard(process windows.Handle, module uintptr) error {
	read := func(address uintptr, size int) ([]byte, error) {
		b := make([]byte, size)
		err := windows.ReadProcessMemory(process, address, &b[0], uintptr(size), nil)
		return b, err
	}
	header, err := read(module, 1024)
	if err != nil {
		return err
	}
	pe := int(binary.LittleEndian.Uint32(header[60:]))
	if pe < 64 || pe+128 > len(header) || binary.LittleEndian.Uint16(header[pe+4:]) != 0x14c {
		return fmt.Errorf("unsupported repack ntdll image")
	}
	directory := pe + 24 + 96
	rva := binary.LittleEndian.Uint32(header[directory:])
	size := binary.LittleEndian.Uint32(header[directory+4:])
	if size < 40 || size > 1<<20 {
		return fmt.Errorf("invalid repack ntdll exports")
	}
	exports, err := read(module+uintptr(rva), int(size))
	if err != nil {
		return err
	}
	at := func(address uint32, n int) []byte {
		if n < 0 || n > 1<<20 || address < rva || uint64(address-rva)+uint64(n) > uint64(len(exports)) {
			return nil
		}
		return exports[address-rva : address-rva+uint32(n)]
	}
	u32 := binary.LittleEndian.Uint32
	count := u32(exports[24:])
	functions := u32(exports[28:])
	names := u32(exports[32:])
	ordinals := u32(exports[36:])
	if count > 20000 {
		return fmt.Errorf("invalid export count")
	}
	var function uintptr
	for i := uint32(0); i < count; i++ {
		namePtr := at(names+i*4, 4)
		ordinal := at(ordinals+i*2, 2)
		if namePtr == nil || ordinal == nil {
			break
		}
		name := at(u32(namePtr), len("NtAllocateVirtualMemory")+1)
		if name == nil || string(name) != "NtAllocateVirtualMemory\x00" {
			continue
		}
		target := at(functions+uint32(binary.LittleEndian.Uint16(ordinal))*4, 4)
		if target == nil {
			break
		}
		function = module + uintptr(u32(target))
		break
	}
	if function == 0 {
		return fmt.Errorf("NtAllocateVirtualMemory export not found")
	}
	original, err := read(function, 5)
	if err != nil {
		return err
	}
	if original[0] != 0xb8 {
		return fmt.Errorf("unsupported x86 allocation stub")
	} // MOV EAX, imm32: exactly 5 bytes
	kernel := windows.NewLazySystemDLL("kernel32.dll")
	code, _, err := kernel.NewProc("VirtualAllocEx").Call(uintptr(process), 0, 4096, windows.MEM_RESERVE|windows.MEM_COMMIT, windows.PAGE_READWRITE)
	if code == 0 || uint64(code) > 0xffffffff {
		return fmt.Errorf("allocate 32-bit guard: %w", err)
	}
	// mov eax,[esp+16]; cmp dword [eax],7ffff001h; jb original;
	// mov eax,STATUS_NO_MEMORY; ret 24; original MOV; jmp original+5.
	wrapper := []byte{0x8b, 0x44, 0x24, 0x10, 0x81, 0x38, 0x01, 0xf0, 0xff, 0x7f, 0x72, 0x08, 0xb8, 0x17, 0, 0, 0xc0, 0xc2, 0x18, 0}
	wrapper = append(wrapper, original...)
	wrapper = append(wrapper, 0xe9, 0, 0, 0, 0)
	//nolint:gosec // G115: x86 relative jump displacement intentionally wraps modulo 2^32.
	binary.LittleEndian.PutUint32(wrapper[len(wrapper)-4:], uint32(function+5)-uint32(code+uintptr(len(wrapper))))
	if err := windows.WriteProcessMemory(process, code, &wrapper[0], uintptr(len(wrapper)), nil); err != nil {
		return err
	}
	var old uint32
	if err := windows.VirtualProtectEx(process, code, 4096, windows.PAGE_EXECUTE_READ, &old); err != nil {
		return err
	}
	jump := []byte{0xe9, 0, 0, 0, 0}
	binary.LittleEndian.PutUint32(jump[1:], uint32(code)-uint32(function+5))
	if err := windows.VirtualProtectEx(process, function, 5, windows.PAGE_EXECUTE_READWRITE, &old); err != nil {
		return err
	}
	writeErr := windows.WriteProcessMemory(process, function, &jump[0], 5, nil)
	var unused uint32
	restoreErr := windows.VirtualProtectEx(process, function, 5, old, &unused)
	if ok, _, flushErr := kernel.NewProc("FlushInstructionCache").Call(uintptr(process), code, uintptr(len(wrapper))); ok == 0 {
		return errors.Join(writeErr, restoreErr, flushErr)
	}
	if ok, _, flushErr := kernel.NewProc("FlushInstructionCache").Call(uintptr(process), function, 5); ok == 0 {
		return errors.Join(writeErr, restoreErr, flushErr)
	}
	if writeErr != nil {
		return writeErr
	}
	return restoreErr
}

func processImage(process windows.Handle) string {
	var buffer [32768]uint16
	size := uint32(len(buffer))
	if windows.QueryFullProcessImageName(process, 0, &buffer[0], &size) != nil {
		return ""
	}
	return strings.TrimPrefix(windows.UTF16ToString(buffer[:size]), `\\?\`)
}
func isWineService(path string) bool {
	system, err := windows.GetSystemDirectory()
	if err != nil {
		return false
	}
	return strings.EqualFold(path, system+`\explorer.exe`) || strings.EqualFold(path, system+`\rpcss.exe`)
}

// Wine's 64-bit module snapshot omits WoW64's 32-bit modules. Use its separate
// PEB loader list at the loader breakpoint instead of guessing a fixed base.
func wow64Ntdll(process windows.Handle) uintptr {
	var peb uintptr
	//nolint:gosec // G103: Win32/COM ABI uses pointers to live typed buffers; the call is synchronous.
	if windows.NtQueryInformationProcess(process, 26, unsafe.Pointer(&peb), uint32(unsafe.Sizeof(peb)), nil) != nil || peb == 0 {
		return 0
	}
	read := func(address uintptr, size int) []byte {
		b := make([]byte, size)
		if windows.ReadProcessMemory(process, address, &b[0], uintptr(size), nil) != nil {
			return nil
		}
		return b
	}
	p := read(peb, 16)
	if p == nil {
		return 0
	}
	ldr := uintptr(binary.LittleEndian.Uint32(p[12:]))
	if ldr == 0 {
		return 0
	}
	head := ldr + 12
	p = read(head, 8)
	if p == nil {
		return 0
	}
	next := uintptr(binary.LittleEndian.Uint32(p))
	for i := 0; i < 256 && next != head && next != 0; i++ {
		entry := read(next, 52)
		if entry == nil {
			return 0
		}
		length := int(binary.LittleEndian.Uint16(entry[44:]))
		address := uintptr(binary.LittleEndian.Uint32(entry[48:]))
		if length > 0 && length < 512 {
			b := read(address, length)
			if b != nil {
				name := make([]uint16, length/2)
				for j := range name {
					name[j] = binary.LittleEndian.Uint16(b[j*2:])
				}
				if strings.EqualFold(windows.UTF16ToString(name), "ntdll.dll") {
					return uintptr(binary.LittleEndian.Uint32(entry[24:]))
				}
			}
		}
		next = uintptr(binary.LittleEndian.Uint32(entry))
	}
	return 0
}
