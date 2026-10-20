package platform

import (
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const (
	displayClassGUID = "{4d36e968-e325-11ce-bfc1-08002be10318}"
	displayClassKey  = `SYSTEM\CurrentControlSet\Control\Class\` + displayClassGUID
)

var adapterKeyName = regexp.MustCompile(`^\d{4}$`)

func readAdapters() ([]adapter, error) {
	root, err := registry.OpenKey(registry.LOCAL_MACHINE, displayClassKey, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return nil, fmt.Errorf("open display class: %w", err)
	}
	defer closeKey(root)
	present, err := presentDriverKeys()
	if err != nil {
		return nil, err
	}
	names, err := root.ReadSubKeyNames(-1)
	if err != nil {
		return nil, fmt.Errorf("list display adapters: %w", err)
	}
	var out []adapter
	for _, name := range names {
		if !adapterKeyName.MatchString(name) {
			continue
		}
		a, ok, err := readAdapter(displayClassKey+`\`+name, !present[name])
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, a)
		}
	}
	return out, nil
}

func readAdapter(path string, absent bool) (adapter, bool, error) {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, path, registry.QUERY_VALUE)
	if err != nil {
		return adapter{}, false, fmt.Errorf("open %s: %w", path, err)
	}
	defer closeKey(key)
	desc, _, err := key.GetStringValue("DriverDesc")
	if errors.Is(err, registry.ErrNotExist) {
		return adapter{}, false, nil
	}
	if err != nil {
		return adapter{}, false, fmt.Errorf("read DriverDesc of %s: %w", path, err)
	}
	a := adapter{Desc: desc, Absent: absent}
	if a.Absent {
		return a, true, nil
	}
	if err := readMemory(key, path, &a); err != nil {
		return adapter{}, false, err
	}
	return a, true, nil
}

func readMemory(key registry.Key, path string, a *adapter) error {
	q, err := integerOrBinary(key, "HardwareInformation.qwMemorySize")
	if err != nil {
		return fmt.Errorf("read qwMemorySize of %s: %w", path, err)
	}
	if q.ok {
		a.QWord, a.HasQWord = q.value, true
	}
	d, err := integerOrBinary(key, "HardwareInformation.MemorySize")
	if err != nil {
		return fmt.Errorf("read MemorySize of %s: %w", path, err)
	}
	switch {
	case !d.ok:
	case d.wide:
		if !a.HasQWord {
			a.QWord, a.HasQWord = d.value, true
		}
	default:
		a.DWord, a.HasDWord = uint32(d.value), true //nolint:gosec // G115: 4-byte registry value, fits uint32 by construction
	}
	return nil
}

type regNumber struct {
	value uint64
	wide  bool
	ok    bool
}

// integerOrBinary reads a size the drivers store as REG_QWORD, REG_DWORD or a
// 4/8-byte REG_BINARY; a missing value is not an error.
func integerOrBinary(key registry.Key, name string) (regNumber, error) {
	v, typ, err := key.GetIntegerValue(name)
	switch {
	case err == nil:
		return regNumber{value: v, wide: typ == registry.QWORD, ok: true}, nil
	case errors.Is(err, registry.ErrNotExist):
		return regNumber{}, nil
	case !errors.Is(err, registry.ErrUnexpectedType):
		return regNumber{}, err
	}
	bin, _, err := key.GetBinaryValue(name)
	if err != nil {
		return regNumber{}, err
	}
	value, ok := decodeLE(bin)
	if !ok {
		return regNumber{}, nil
	}
	return regNumber{value: value, wide: len(bin) == 8, ok: true}, nil
}

// presentDriverKeys lists the Class subkey names (0000, 0001, ...) of display
// devices that exist now. The class key itself keeps subkeys of removed or
// unplugged cards, so only the PnP manager can say which are live.
func presentDriverKeys() (map[string]bool, error) {
	guid, err := windows.GUIDFromString(displayClassGUID)
	if err != nil {
		return nil, fmt.Errorf("display class guid: %w", err)
	}
	devs, err := windows.SetupDiGetClassDevsEx(&guid, "", 0, windows.DIGCF_PRESENT, 0, "")
	if err != nil {
		return nil, fmt.Errorf("list present display devices: %w", err)
	}
	defer func() {
		if err := devs.Close(); err != nil {
			slog.Warn("close display device list", "error", err)
		}
	}()
	out := map[string]bool{}
	for i := 0; ; i++ {
		data, err := devs.EnumDeviceInfo(i)
		if errors.Is(err, windows.ERROR_NO_MORE_ITEMS) {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("enumerate display device %d: %w", i, err)
		}
		prop, err := devs.DeviceRegistryProperty(data, windows.SPDRP_DRIVER)
		if errors.Is(err, windows.ERROR_INVALID_DATA) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("driver key of display device %d: %w", i, err)
		}
		if driver, ok := prop.(string); ok {
			out[driver[strings.LastIndex(driver, `\`)+1:]] = true
		}
	}
}
