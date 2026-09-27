package install

import (
	"fmt"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

func ExeVersion(path string) (VersionInfo, bool) {
	size, err := windows.GetFileVersionInfoSize(path, nil)
	if err != nil || size == 0 {
		return VersionInfo{}, false
	}
	block := make([]byte, size)
	//nolint:gosec // G103: block is a freshly allocated slice of size (checked > 0 above), never reused after this call; &block[0] just hands its first byte to the Win32 API as required
	if err := windows.GetFileVersionInfo(path, 0, size, unsafe.Pointer(&block[0])); err != nil {
		return VersionInfo{}, false
	}

	info := VersionInfo{Source: "pe_metadata"}
	lang := translationID(block)
	if lang != "" {
		info.Version = queryString(block, lang, "ProductVersion")
		info.Product = queryString(block, lang, "ProductName")
		info.Company = queryString(block, lang, "CompanyName")
	}
	if info.Version != "" {
		info.Confidence = "high"
		return info, true
	}
	if fixed, ok := fixedVersion(block); ok {
		info.Version = fixed
		info.Confidence = "medium"
		return info, true
	}
	return VersionInfo{}, false
}

func translationID(block []byte) string {
	var ptr unsafe.Pointer
	var size uint32
	//nolint:gosec // G103: block is only ever the non-empty slice ExeVersion allocated (size checked > 0 before this call chain), so &block[0] is always in bounds
	if err := windows.VerQueryValue(unsafe.Pointer(&block[0]), `\VarFileInfo\Translation`, unsafe.Pointer(&ptr), &size); err != nil || size < 4 {
		return ""
	}
	pair := (*[2]uint16)(ptr)
	return fmt.Sprintf("%04x%04x", pair[0], pair[1])
}

func queryString(block []byte, lang, key string) string {
	var ptr unsafe.Pointer
	var size uint32
	sub := `\StringFileInfo\` + lang + `\` + key
	//nolint:gosec // G103: block is only ever the non-empty slice ExeVersion allocated (size checked > 0 before this call chain), so &block[0] is always in bounds
	if err := windows.VerQueryValue(unsafe.Pointer(&block[0]), sub, unsafe.Pointer(&ptr), &size); err != nil || size == 0 {
		return ""
	}
	return strings.TrimSpace(windows.UTF16PtrToString((*uint16)(ptr)))
}

// fixedVersionSizeOK проверяет, что VerQueryValue вернул блок не меньше
// VS_FIXEDFILEINFO, прежде чем по нему кастуют указатель: без этого
// усечённый ресурс версии читался бы за своими границами (соседняя
// translationID делает тот же выбор для \VarFileInfo\Translation).
func fixedVersionSizeOK(size uint32) bool {
	return size >= uint32(unsafe.Sizeof(windows.VS_FIXEDFILEINFO{}))
}

func fixedVersion(block []byte) (string, bool) {
	var ptr unsafe.Pointer
	var size uint32
	//nolint:gosec // G103: block is only ever the non-empty slice ExeVersion allocated (size checked > 0 before this call chain), so &block[0] is always in bounds
	if err := windows.VerQueryValue(unsafe.Pointer(&block[0]), `\`, unsafe.Pointer(&ptr), &size); err != nil || !fixedVersionSizeOK(size) {
		return "", false
	}
	fixed := (*windows.VS_FIXEDFILEINFO)(ptr)
	if fixed.FileVersionMS == 0 && fixed.FileVersionLS == 0 {
		return "", false
	}
	return fmt.Sprintf("%d.%d.%d.%d",
		fixed.FileVersionMS>>16, fixed.FileVersionMS&0xffff,
		fixed.FileVersionLS>>16, fixed.FileVersionLS&0xffff), true
}
