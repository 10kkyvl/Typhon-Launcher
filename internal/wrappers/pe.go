package wrappers

import (
	"bytes"
	"debug/pe"
	"encoding/binary"
	"fmt"
	"os"
	"sort"
	"strings"
)

const (
	archX86 = "x86"
	archX64 = "x64"
)

func parseImports(path string) (arch string, libs []string, err error) {
	file, err := os.Open(path)
	if err != nil {
		return "", nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() {
		if cerr := file.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("close %s: %w", path, cerr)
		}
	}()
	f, err := pe.NewFile(file)
	if err != nil {
		return "", nil, fmt.Errorf("%w: %s: %w", ErrNotPE, path, err)
	}
	switch f.Machine {
	case pe.IMAGE_FILE_MACHINE_I386:
		arch = archX86
	case pe.IMAGE_FILE_MACHINE_AMD64:
		arch = archX64
	default:
		return "", nil, fmt.Errorf("%w: machine 0x%x", ErrNotPE, f.Machine)
	}
	all, err := importedLibraries(f)
	if err != nil {
		return "", nil, fmt.Errorf("read imports of %s: %w", path, err)
	}
	return arch, graphicsImports(all), nil
}

// debug/pe.File.ImportedLibraries is a stub that returns nothing, and
// ImportedSymbols drops libraries imported only by ordinal, so the import and
// delay-import tables are walked here.
func importedLibraries(f *pe.File) ([]string, error) {
	var dirs []pe.DataDirectory
	switch oh := f.OptionalHeader.(type) {
	case *pe.OptionalHeader32:
		dirs = oh.DataDirectory[:min(int(oh.NumberOfRvaAndSizes), len(oh.DataDirectory))]
	case *pe.OptionalHeader64:
		dirs = oh.DataDirectory[:min(int(oh.NumberOfRvaAndSizes), len(oh.DataDirectory))]
	default:
		return nil, nil
	}
	r := &rvaReader{f: f, data: map[*pe.Section][]byte{}}
	var out []string
	tables := []struct{ dir, size, nameAt uint32 }{
		{pe.IMAGE_DIRECTORY_ENTRY_IMPORT, 20, 12},
		{13, 32, 4},
	}
	for _, t := range tables {
		if int(t.dir) >= len(dirs) || dirs[t.dir].VirtualAddress == 0 {
			continue
		}
		for i := uint32(0); i < maxImportEntries; i++ {
			desc, err := r.bytes(dirs[t.dir].VirtualAddress+i*t.size, t.size)
			if err != nil {
				return nil, err
			}
			nameRVA := binary.LittleEndian.Uint32(desc[t.nameAt:])
			if nameRVA == 0 {
				break
			}
			name, err := r.cstring(nameRVA)
			if err != nil {
				return nil, err
			}
			out = append(out, name)
		}
	}
	return out, nil
}

const maxImportEntries = 1024

type rvaReader struct {
	f    *pe.File
	data map[*pe.Section][]byte
}

func (r *rvaReader) section(rva uint32) (*pe.Section, []byte, error) {
	for _, s := range r.f.Sections {
		if s.Offset == 0 || rva < s.VirtualAddress || rva-s.VirtualAddress >= max(s.VirtualSize, s.Size) {
			continue
		}
		d, ok := r.data[s]
		if !ok {
			var err error
			if d, err = s.Data(); err != nil {
				return nil, nil, err
			}
			r.data[s] = d
		}
		return s, d, nil
	}
	return nil, nil, fmt.Errorf("%w: rva 0x%x is outside every section", ErrNotPE, rva)
}

func (r *rvaReader) bytes(rva, n uint32) ([]byte, error) {
	s, d, err := r.section(rva)
	if err != nil {
		return nil, err
	}
	off := int(rva - s.VirtualAddress)
	if off+int(n) > len(d) {
		return nil, fmt.Errorf("%w: table at 0x%x is truncated", ErrNotPE, rva)
	}
	return d[off : off+int(n)], nil
}

func (r *rvaReader) cstring(rva uint32) (string, error) {
	s, d, err := r.section(rva)
	if err != nil {
		return "", err
	}
	tail := d[min(int(rva-s.VirtualAddress), len(d)):]
	end := bytes.IndexByte(tail[:min(len(tail), 260)], 0)
	if end < 0 {
		return "", fmt.Errorf("%w: unterminated library name at 0x%x", ErrNotPE, rva)
	}
	return string(tail[:end]), nil
}

func graphicsImports(all []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, n := range all {
		n = strings.TrimSuffix(strings.ToLower(n), ".dll")
		if !isGraphics(n) || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

func isGraphics(n string) bool {
	switch n {
	case "ddraw", "d3d8", "d3d9", "d3d11", "opengl32":
		return true
	}
	return strings.HasPrefix(n, "d3d10") || strings.HasPrefix(n, "glide")
}

func suggest(imports []string) string {
	var dgv, dx bool
	for _, n := range imports {
		switch {
		case n == "ddraw", n == "d3d8", strings.HasPrefix(n, "glide"):
			dgv = true
		case n == "d3d9", n == "d3d11", strings.HasPrefix(n, "d3d10"):
			dx = true
		}
	}
	switch {
	case dgv:
		return WrapperDgVoodoo
	case dx:
		return WrapperDXVK
	}
	return ""
}

type target struct {
	Name string
	Src  string
}

func targets(wrapper, arch string, imports []string) []target {
	seen := map[string]bool{}
	var out []target
	add := func(name, src string) {
		if !seen[name] {
			seen[name] = true
			out = append(out, target{Name: name, Src: src})
		}
	}
	for _, n := range imports {
		switch wrapper {
		case WrapperDgVoodoo:
			switch {
			case n == "ddraw" && arch == archX86:
				add("ddraw.dll", "x86/ddraw.dll")
				add("d3dimm.dll", "x86/d3dimm.dll")
			case n == "d3d8" && arch == archX86:
				add("d3d8.dll", "x86/d3d8.dll")
			case n == "d3d9":
				add("d3d9.dll", arch+"/d3d9.dll")
			case strings.HasPrefix(n, "glide") && arch == archX86:
				add("glide.dll", "x86/glide.dll")
				add("glide2x.dll", "x86/glide2x.dll")
				add("glide3x.dll", "x86/glide3x.dll")
			}
		case WrapperDXVK:
			switch {
			case n == "d3d9":
				add("d3d9.dll", arch+"/d3d9.dll")
			case n == "d3d11" || strings.HasPrefix(n, "d3d10"):
				add("d3d10core.dll", arch+"/d3d10core.dll")
				add("d3d11.dll", arch+"/d3d11.dll")
				add("dxgi.dll", arch+"/dxgi.dll")
			}
		}
	}
	if wrapper == WrapperDgVoodoo && len(out) > 0 {
		add(confName, confName)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
