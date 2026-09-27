//go:build darwin && !devmock

package install

import (
	"debug/pe"
	"encoding/binary"
	"fmt"
	"log/slog"
	"strings"
	"unicode/utf16"
)

// Версия windows-игры лежит в ресурсе PE, и на Windows её достаёт системный
// API. Здесь его нет, поэтому разбираем файл сами: без версии обновления игр
// опираются на одно название, а это гадание.
const (
	rtVersion       = 16
	resourceDirSize = 16
	resourceEntry   = 8
	versionSigMagic = 0xFEEF04BD
)

func ExeVersion(path string) (VersionInfo, bool) {
	block, ok := versionResource(path)
	if !ok {
		return VersionInfo{}, false
	}
	return parseVersionResource(block)
}

// versionResource достаёт блок VS_VERSIONINFO из секции ресурсов. Дерево
// ресурсов трёхуровневое: тип, имя, язык, — и нас интересует первая же
// запись под типом RT_VERSION.
func versionResource(path string) ([]byte, bool) {
	file, err := pe.Open(path)
	if err != nil {
		return nil, false
	}
	defer func() {
		if err := file.Close(); err != nil {
			slog.Debug("close pe file", "path", path, "error", err)
		}
	}()

	section := file.Section(".rsrc")
	if section == nil {
		return nil, false
	}
	data, err := section.Data()
	if err != nil {
		return nil, false
	}
	entry, ok := findResourceData(data, 0, rtVersion, 0)
	if !ok {
		return nil, false
	}
	// Записи хранят RVA, а на руках у нас только эта секция.
	offset := int64(entry.rva) - int64(section.VirtualAddress)
	if offset < 0 || offset+int64(entry.size) > int64(len(data)) {
		return nil, false
	}
	return data[offset : offset+int64(entry.size)], true
}

type resourceData struct {
	rva  uint32
	size uint32
}

// findResourceData спускается по дереву: на нулевом уровне ищет нужный тип,
// глубже берёт первую попавшуюся запись — имя и язык ресурса версии нам
// безразличны, файл несёт её в единственном экземпляре.
func findResourceData(section []byte, dirOffset int64, wantType uint32, depth int) (resourceData, bool) {
	if depth > 3 || dirOffset < 0 || dirOffset+resourceDirSize > int64(len(section)) {
		return resourceData{}, false
	}
	named := binary.LittleEndian.Uint16(section[dirOffset+12:])
	ids := binary.LittleEndian.Uint16(section[dirOffset+14:])
	first := dirOffset + resourceDirSize

	for i := range int(named) + int(ids) {
		entry := first + int64(i)*resourceEntry
		if entry+resourceEntry > int64(len(section)) {
			return resourceData{}, false
		}
		name := binary.LittleEndian.Uint32(section[entry:])
		next := binary.LittleEndian.Uint32(section[entry+4:])
		if depth == 0 {
			// На верхнем уровне отбираем строго по типу; имена (старший бит)
			// типами не бывают.
			if name&0x80000000 != 0 || name != wantType {
				continue
			}
		}
		if next&0x80000000 != 0 {
			//nolint:gosec // G115: смещение внутри секции ресурсов, проверяется границами ниже
			if found, ok := findResourceData(section, int64(next&0x7FFFFFFF), wantType, depth+1); ok {
				return found, true
			}
			continue
		}
		leaf := int64(next)
		if leaf+8 > int64(len(section)) {
			return resourceData{}, false
		}
		return resourceData{
			rva:  binary.LittleEndian.Uint32(section[leaf:]),
			size: binary.LittleEndian.Uint32(section[leaf+4:]),
		}, true
	}
	return resourceData{}, false
}

// parseVersionResource читает дерево VS_VERSIONINFO. Строковая версия точнее
// числовой (там бывает «1.0.30000», а не только четыре числа), поэтому
// сначала ищем её и только потом откатываемся к VS_FIXEDFILEINFO.
func parseVersionResource(block []byte) (VersionInfo, bool) {
	root, ok := readNode(block)
	if !ok || root.key != "VS_VERSION_INFO" {
		return VersionInfo{}, false
	}

	info := VersionInfo{Source: "pe_metadata"}
	if strings, ok := stringTable(root); ok {
		info.Version = strings["ProductVersion"]
		if info.Version == "" {
			info.Version = strings["FileVersion"]
		}
		info.Product = strings["ProductName"]
		info.Company = strings["CompanyName"]
	}
	if info.Version != "" {
		info.Confidence = "high"
		return info, true
	}
	if fixed, ok := fixedFileVersion(root.value); ok {
		info.Version = fixed
		info.Confidence = "medium"
		return info, true
	}
	return VersionInfo{}, false
}

type versionNode struct {
	key      string
	value    []byte
	children []versionNode
}

// readNode разбирает одну структуру формата: заголовок из трёх слов, ключ в
// UTF-16, выравнивание, значение, снова выравнивание, дети до конца длины.
func readNode(block []byte) (versionNode, bool) {
	if len(block) < 6 {
		return versionNode{}, false
	}
	length := int(binary.LittleEndian.Uint16(block[0:]))
	valueLen := int(binary.LittleEndian.Uint16(block[2:]))
	typ := binary.LittleEndian.Uint16(block[4:])
	if length < 6 || length > len(block) {
		return versionNode{}, false
	}
	body := block[:length]

	key, offset, ok := readWideString(body, 6)
	if !ok {
		return versionNode{}, false
	}
	offset = align4(offset)
	// Текстовые значения меряются в словах, двоичные — в байтах.
	if typ == 1 {
		valueLen *= 2
	}
	if offset+valueLen > len(body) {
		return versionNode{}, false
	}
	node := versionNode{key: key, value: body[offset : offset+valueLen]}

	offset = align4(offset + valueLen)
	for offset+6 <= len(body) {
		child, ok := readNode(body[offset:])
		if !ok {
			break
		}
		node.children = append(node.children, child)
		step := int(binary.LittleEndian.Uint16(body[offset:]))
		if step <= 0 {
			break
		}
		offset = align4(offset + step)
	}
	return node, true
}

func readWideString(body []byte, offset int) (string, int, bool) {
	units := make([]uint16, 0, 16)
	for offset+2 <= len(body) {
		u := binary.LittleEndian.Uint16(body[offset:])
		offset += 2
		if u == 0 {
			return string(utf16.Decode(units)), offset, true
		}
		units = append(units, u)
	}
	return "", offset, false
}

func align4(offset int) int {
	if rem := offset % 4; rem != 0 {
		return offset + 4 - rem
	}
	return offset
}

// stringTable отдаёт первую таблицу строк: их бывает несколько по числу
// языков, но версия во всех одна и та же.
func stringTable(root versionNode) (map[string]string, bool) {
	for _, child := range root.children {
		if child.key != "StringFileInfo" {
			continue
		}
		for _, table := range child.children {
			out := map[string]string{}
			for _, entry := range table.children {
				out[entry.key] = decodeWide(entry.value)
			}
			if len(out) > 0 {
				return out, true
			}
		}
	}
	return nil, false
}

func decodeWide(value []byte) string {
	units := make([]uint16, 0, len(value)/2)
	for i := 0; i+2 <= len(value); i += 2 {
		u := binary.LittleEndian.Uint16(value[i:])
		if u == 0 {
			break
		}
		units = append(units, u)
	}
	return strings.TrimSpace(string(utf16.Decode(units)))
}

func fixedFileVersion(value []byte) (string, bool) {
	if len(value) < 16 || binary.LittleEndian.Uint32(value) != versionSigMagic {
		return "", false
	}
	most := binary.LittleEndian.Uint32(value[8:])
	least := binary.LittleEndian.Uint32(value[12:])
	return fmt.Sprintf("%d.%d.%d.%d", most>>16, most&0xFFFF, least>>16, least&0xFFFF), true
}
