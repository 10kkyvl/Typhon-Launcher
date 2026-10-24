package wrappers

import (
	"fmt"
	"strings"
)

var dgVoodooResolution = map[string]string{
	"default": "unforced",
	"1080p":   "max_fhd",
	"native":  "desktop",
}

func patchConf(data []byte, preset string) ([]byte, error) {
	res, ok := dgVoodooResolution[preset]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrBadPreset, preset)
	}
	edits := []struct{ section, key, value string }{
		{"DirectX", "dgVoodooWatermark", "false"},
		{"DirectX", "Resolution", res},
		{"Glide", "3DfxWatermark", "false"},
		{"Glide", "Resolution", res},
	}
	lines := strings.Split(string(data), "\n")
	for _, e := range edits {
		if err := setKey(lines, e.section, e.key, e.value); err != nil {
			return nil, err
		}
	}
	return []byte(strings.Join(lines, "\n")), nil
}

func setKey(lines []string, section, key, value string) error {
	in := false
	for i, line := range lines {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "[") {
			in = strings.EqualFold(t, "["+section+"]")
			continue
		}
		if !in {
			continue
		}
		name, _, found := strings.Cut(t, "=")
		if !found || !strings.EqualFold(strings.TrimSpace(name), key) {
			continue
		}
		eol := ""
		if strings.HasSuffix(line, "\r") {
			eol = "\r"
		}
		lines[i] = fmt.Sprintf("%-36s= %s%s", key, value, eol)
		return nil
	}
	return fmt.Errorf("%w: [%s] %s not found", ErrBadArchive, section, key)
}
