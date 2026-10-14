//go:build windows

package redist

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"

	"golang.org/x/sys/windows/registry"
)

const (
	uninstallPath = `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`
	vcRuntimes    = `SOFTWARE\Microsoft\VisualStudio\14.0\VC\Runtimes`
	vcRuntimes32  = `SOFTWARE\WOW6432Node\Microsoft\VisualStudio\14.0\VC\Runtimes`
)

type uninstallRoot struct {
	hive   registry.Key
	label  string
	access uint32
}

var uninstallRoots = []uninstallRoot{
	{registry.LOCAL_MACHINE, "HKLM", registry.ENUMERATE_SUB_KEYS | registry.QUERY_VALUE | registry.WOW64_64KEY},
	{registry.LOCAL_MACHINE, "HKLM32", registry.ENUMERATE_SUB_KEYS | registry.QUERY_VALUE | registry.WOW64_32KEY},
	{registry.CURRENT_USER, "HKCU", registry.ENUMERATE_SUB_KEYS | registry.QUERY_VALUE},
}

type vcKey struct{ path, arch string }

var vcKeys = []vcKey{
	{vcRuntimes + `\x64`, "x64"},
	{vcRuntimes + `\x86`, "x86"},
	{vcRuntimes32 + `\x86`, "x86"},
}

func InstalledVC(ctx context.Context) ([]Runtime, error) {
	probe, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft`, registry.QUERY_VALUE|registry.WOW64_64KEY)
	if err != nil {
		return nil, fmt.Errorf(`открыть HKLM\SOFTWARE\Microsoft: %w`, err)
	}
	closeKey(probe)

	seen := map[Runtime]struct{}{}
	out := []Runtime{}
	add := func(r Runtime) {
		if _, dup := seen[r]; dup {
			return
		}
		seen[r] = struct{}{}
		out = append(out, r)
	}
	for _, k := range vcKeys {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		r, ok, err := readVCRuntime(k)
		if err != nil {
			return nil, err
		}
		if ok {
			add(r)
		}
	}
	for _, root := range uninstallRoots {
		runtimes, err := readUninstallRuntimes(ctx, root)
		if err != nil {
			return nil, err
		}
		for _, r := range runtimes {
			add(r)
		}
	}
	return out, nil
}

func readVCRuntime(k vcKey) (Runtime, bool, error) {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, k.path, registry.QUERY_VALUE|registry.WOW64_64KEY)
	if errors.Is(err, registry.ErrNotExist) {
		return Runtime{}, false, nil
	}
	if err != nil {
		return Runtime{}, false, fmt.Errorf(`открыть HKLM\%s: %w`, k.path, err)
	}
	defer closeKey(key)
	installed, _, err := key.GetIntegerValue("Installed")
	if errors.Is(err, registry.ErrNotExist) {
		return Runtime{}, false, nil
	}
	if err != nil {
		return Runtime{}, false, fmt.Errorf(`прочитать Installed в %s: %w`, k.path, err)
	}
	if installed != 1 {
		return Runtime{}, false, nil
	}
	version, _, err := key.GetStringValue("Version")
	if err != nil && !errors.Is(err, registry.ErrNotExist) {
		return Runtime{}, false, fmt.Errorf(`прочитать Version в %s: %w`, k.path, err)
	}
	return Runtime{Name: "Visual C++ 2015-2022", Version: version, Arch: k.arch}, true, nil
}

func readUninstallRuntimes(ctx context.Context, root uninstallRoot) ([]Runtime, error) {
	key, err := registry.OpenKey(root.hive, uninstallPath, root.access)
	if errors.Is(err, registry.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf(`открыть %s\%s: %w`, root.label, uninstallPath, err)
	}
	defer closeKey(key)
	names, err := key.ReadSubKeyNames(-1)
	if err != nil {
		return nil, fmt.Errorf(`перечислить %s\%s: %w`, root.label, uninstallPath, err)
	}
	var out []Runtime
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		sub, err := registry.OpenKey(key, name, registry.QUERY_VALUE)
		if errors.Is(err, registry.ErrNotExist) || errors.Is(err, fs.ErrPermission) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("открыть ключ удаления %s: %w", name, err)
		}
		display, _, derr := sub.GetStringValue("DisplayName")
		version, _, verr := sub.GetStringValue("DisplayVersion")
		closeKey(sub)
		if errors.Is(derr, registry.ErrNotExist) || errors.Is(derr, registry.ErrUnexpectedType) {
			continue
		}
		if derr != nil {
			return nil, fmt.Errorf("прочитать DisplayName в %s: %w", name, derr)
		}
		if verr != nil && !errors.Is(verr, registry.ErrNotExist) && !errors.Is(verr, registry.ErrUnexpectedType) {
			return nil, fmt.Errorf("прочитать DisplayVersion в %s: %w", name, verr)
		}
		if r, ok := parseVCDisplayName(display, version); ok {
			out = append(out, r)
		}
	}
	return out, nil
}

func closeKey(key registry.Key) {
	if err := key.Close(); err != nil {
		slog.Warn("close registry key", "error", err)
	}
}
