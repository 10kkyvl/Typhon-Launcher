//go:build windows

package shortcut

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"

	"typhon/internal/settings"
)

// portableMode is a seam for tests: a portable launcher leaves the links of any
// installed copy of itself alone.
var portableMode = settings.IsPortable

const (
	stgmReadWrite = 2
	maxLinkPath   = 32768
)

var procPropVariantClear = modole32.NewProc("PropVariantClear")

// LinkSpot is a folder that may hold the launcher's own links. An empty Name
// means every .lnk in Dir, otherwise only that file name.
type LinkSpot struct {
	Dir  string
	Name string
}

// LauncherLinkSpots lists where the launcher's Start menu, desktop and
// taskbar-pinned links live for the current user.
func LauncherLinkSpots() ([]LinkSpot, error) {
	if portableMode() {
		return nil, nil
	}
	programs, err := windows.KnownFolderPath(windows.FOLDERID_Programs, windows.KF_FLAG_DEFAULT)
	if err != nil {
		return nil, fmt.Errorf("shortcut: известная папка Programs: %w", err)
	}
	desktop, err := DesktopDir()
	if err != nil {
		return nil, err
	}
	roaming, err := windows.KnownFolderPath(windows.FOLDERID_RoamingAppData, windows.KF_FLAG_DEFAULT)
	if err != nil {
		return nil, fmt.Errorf("shortcut: известная папка AppData: %w", err)
	}
	pinned := filepath.Join(roaming, "Microsoft", "Internet Explorer", "Quick Launch", "User Pinned", "TaskBar")
	return []LinkSpot{
		{Dir: programs, Name: "Typhon.lnk"},
		{Dir: desktop, Name: "Typhon.lnk"},
		{Dir: pinned},
	}, nil
}

// RefreshLauncherLinks sets the AppUserModelID on existing links that point at
// exe, in place: the rest of each link is left as the user has it. Links to
// other targets, game links (--play) and links that already carry id are not
// written.
func RefreshLauncherLinks(ctx context.Context, spots []LinkSpot, exe, id string) error {
	if portableMode() {
		return nil
	}
	if err := validateLinkAppID(exe, id); err != nil {
		return err
	}

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := coInitialize(); err != nil {
		return err
	}
	defer windows.CoUninitialize()

	var errs []error
	for _, spot := range spots {
		paths, err := spotPaths(spot)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		for _, p := range paths {
			if err := ctx.Err(); err != nil {
				return errors.Join(append(errs, err)...)
			}
			if err := setLinkAppID(p, exe, id, func(args string) bool { return !IsPlayArgs(args) }); err != nil {
				errs = append(errs, fmt.Errorf("shortcut: ярлык %s: %w", p, err))
			}
		}
	}
	return errors.Join(errs...)
}

func spotPaths(spot LinkSpot) ([]string, error) {
	if spot.Name != "" {
		return []string{filepath.Join(spot.Dir, spot.Name)}, nil
	}
	entries, err := os.ReadDir(spot.Dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("shortcut: каталог %s: %w", spot.Dir, err)
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ".lnk") {
			out = append(out, filepath.Join(spot.Dir, e.Name()))
		}
	}
	return out, nil
}

// SetLinkAppID sets the AppUserModelID on an existing link in place: the rest
// of the link (icon, hotkey, run mode, arguments) stays as the user has it.
// A link is touched only when it targets exe and match accepts its arguments,
// and is not written when it already carries id. A missing file is skipped.
func SetLinkAppID(path, exe, id string, match func(args string) bool) error {
	if err := validateLinkAppID(exe, id); err != nil {
		return err
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := coInitialize(); err != nil {
		return err
	}
	defer windows.CoUninitialize()
	return setLinkAppID(path, exe, id, match)
}

func validateLinkAppID(exe, id string) error {
	if exe == "" {
		return errors.New("shortcut: путь лаунчера пуст")
	}
	return ValidateAppID(id)
}

func setLinkAppID(path, exe, id string, match func(args string) bool) error {
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	sl, err := createShellLink()
	if err != nil {
		return err
	}
	defer sl.release()
	pf, err := sl.queryPersistFile()
	if err != nil {
		return err
	}
	defer pf.release()
	if err := pf.load(path); err != nil {
		return err
	}

	target, err := sl.getPath()
	if err != nil {
		return err
	}
	same, err := sameFile(target, exe)
	if err != nil {
		return err
	}
	if !same {
		slog.Debug("link target is not the launcher, skipped", "link", path, "target", target, "launcher", exe)
		return nil
	}
	args, err := sl.getArguments()
	if err != nil {
		return err
	}
	if !match(args) {
		return nil
	}

	ps, err := sl.queryPropertyStore()
	if err != nil {
		return err
	}
	defer ps.release()
	current, err := ps.getString(&pkeyAppUserModelID)
	if err != nil {
		return err
	}
	if current == id {
		return nil
	}
	if err := ps.setString(&pkeyAppUserModelID, id); err != nil {
		return err
	}
	if err := ps.commit(); err != nil {
		return err
	}
	return pf.save(path)
}

// sameFile compares by file identity: the link stores the long path while the
// launcher may run from an 8.3 or \\?\ form of the same path.
func sameFile(target, exe string) (bool, error) {
	if strings.EqualFold(filepath.Clean(target), filepath.Clean(exe)) {
		return true, nil
	}
	if target == "" {
		return false, nil
	}
	ti, err := os.Stat(target)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("shortcut: цель ярлыка %s: %w", target, err)
	}
	ei, err := os.Stat(exe)
	if err != nil {
		return false, fmt.Errorf("shortcut: лаунчер %s: %w", exe, err)
	}
	return os.SameFile(ti, ei), nil
}

//nolint:gosec // G103: addresses of the receiver and of a local UTF-16 buffer, live for the call. G115: HRESULT is the 32-bit value in r0; the buffer length is a constant.
func (s *shellLink) getPath() (string, error) {
	buf := make([]uint16, maxLinkPath)
	r0 := callMethod(s.vtbl.GetPath, uintptr(unsafe.Pointer(s)), uintptr(unsafe.Pointer(&buf[0])), maxLinkPath, 0, 0)
	if hr := int32(r0); hr < 0 {
		return "", hresultErr("GetPath", hr)
	}
	return windows.UTF16ToString(buf), nil
}

//nolint:gosec // G103: addresses of the receiver and of a local UTF-16 buffer, live for the call. G115: HRESULT is the 32-bit value in r0; the buffer length is a constant.
func (s *shellLink) getArguments() (string, error) {
	buf := make([]uint16, maxLinkPath)
	r0 := callMethod(s.vtbl.GetArguments, uintptr(unsafe.Pointer(s)), uintptr(unsafe.Pointer(&buf[0])), maxLinkPath)
	if hr := int32(r0); hr < 0 {
		return "", hresultErr("GetArguments", hr)
	}
	return windows.UTF16ToString(buf), nil
}

//nolint:gosec // G103: addresses of the receiver and of the UTF-16 path buffer, both live for the call. G115: HRESULT is the 32-bit value in r0.
func (p *persistFile) load(path string) error {
	ptr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return fmt.Errorf("shortcut: Load: кодирование пути %q: %w", path, err)
	}
	r0 := callMethod(p.vtbl.Load, uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(ptr)), stgmReadWrite)
	if hr := int32(r0); hr < 0 {
		return hresultErr("Load", hr)
	}
	return nil
}

// getString returns "" when the property is absent (VT_EMPTY).
//
//nolint:gosec // G103: addresses of the receiver, the key and the variant, all live for the call. G115: HRESULT is the 32-bit value in r0.
func (p *propertyStore) getString(key *propertyKey) (string, error) {
	var pv propVariant
	r0 := callMethod(p.vtbl.GetValue, uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(key)), uintptr(unsafe.Pointer(&pv)))
	if hr := int32(r0); hr < 0 {
		return "", hresultErr("IPropertyStore.GetValue", hr)
	}
	defer func() {
		callMethod(procPropVariantClear.Addr(), uintptr(unsafe.Pointer(&pv)))
	}()
	if pv.vt != vtLPWSTR {
		return "", nil
	}
	return windows.UTF16PtrToString((*uint16)(pv.data[0])), nil
}
