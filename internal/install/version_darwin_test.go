//go:build darwin && !devmock

package install

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"unicode/utf16"
)

// Блок VS_VERSIONINFO собирается байтами вручную: это единственный способ
// проверить разбор, не таща в репозиторий чужой exe. Формат простой, но
// придирчивый — каждая структура выровнена по четыре байта.
type verBuf struct{ b []byte }

func (v *verBuf) align() {
	for len(v.b)%4 != 0 {
		v.b = append(v.b, 0)
	}
}

func (v *verBuf) u16(x uint16) {
	v.b = binary.LittleEndian.AppendUint16(v.b, x)
}

func (v *verBuf) wide(s string) {
	for _, u := range utf16.Encode([]rune(s)) {
		v.u16(u)
	}
	v.u16(0)
}

// node пишет структуру с заголовком и заранее собранным телом, проставляя
// wLength постфактум — как это делает настоящий ресурсный компилятор.
func node(key string, valueLen uint16, typ uint16, value []byte, children ...[]byte) []byte {
	v := &verBuf{}
	v.u16(0) // место под wLength
	v.u16(valueLen)
	v.u16(typ)
	v.wide(key)
	v.align()
	v.b = append(v.b, value...)
	for _, c := range children {
		v.align()
		v.b = append(v.b, c...)
	}
	//nolint:gosec // G115: тестовые блоки заведомо короче 64 КБ, а wLength в формате шире и не бывает
	binary.LittleEndian.PutUint16(v.b[0:2], uint16(len(v.b)))
	return v.b
}

func stringEntry(key, value string) []byte {
	v := &verBuf{}
	v.wide(value)
	// wValueLength у строки считается в словах вместе с завершающим нулём.
	//nolint:gosec // G115: строки в тестовых фикстурах заведомо короче 64 КБ
	return node(key, uint16(len(utf16.Encode([]rune(value)))+1), 1, v.b)
}

func fixedFileInfo(major, minor, patch, build uint16) []byte {
	b := make([]byte, 52)
	binary.LittleEndian.PutUint32(b[0:], 0xFEEF04BD) // dwSignature
	binary.LittleEndian.PutUint32(b[4:], 0x00010000) // dwStrucVersion
	binary.LittleEndian.PutUint32(b[8:], uint32(major)<<16|uint32(minor))
	binary.LittleEndian.PutUint32(b[12:], uint32(patch)<<16|uint32(build))
	return b
}

func buildVersionBlock(t *testing.T, withStrings bool) []byte {
	t.Helper()
	children := [][]byte{}
	if withStrings {
		table := node("040904b0", 0, 1, nil,
			stringEntry("ProductVersion", "1.0.30000"),
			stringEntry("ProductName", "Hollow Knight Silksong"),
			stringEntry("CompanyName", "Team Cherry"),
		)
		children = append(children, node("StringFileInfo", 0, 1, nil, table))
	}
	trans := &verBuf{}
	trans.u16(0x0409)
	trans.u16(0x04b0)
	children = append(children, node("VarFileInfo", 0, 1, nil,
		node("Translation", 4, 0, trans.b)))
	return node("VS_VERSION_INFO", 52, 0, fixedFileInfo(1, 0, 30000, 0), children...)
}

func TestParseVersionResourceReadsStrings(t *testing.T) {
	got, ok := parseVersionResource(buildVersionBlock(t, true))
	if !ok {
		t.Fatal("parseVersionResource: not found")
	}
	if got.Version != "1.0.30000" {
		t.Fatalf("Version = %q, want %q", got.Version, "1.0.30000")
	}
	if got.Product != "Hollow Knight Silksong" {
		t.Fatalf("Product = %q", got.Product)
	}
	if got.Company != "Team Cherry" {
		t.Fatalf("Company = %q", got.Company)
	}
	if got.Confidence != "high" || got.Source != "pe_metadata" {
		t.Fatalf("Confidence = %q, Source = %q", got.Confidence, got.Source)
	}
}

// Без StringFileInfo остаётся только числовая версия из VS_FIXEDFILEINFO:
// она беднее, поэтому и уверенность ниже — ровно как на Windows.
func TestParseVersionResourceFallsBackToFixed(t *testing.T) {
	got, ok := parseVersionResource(buildVersionBlock(t, false))
	if !ok {
		t.Fatal("parseVersionResource: not found")
	}
	if got.Version != "1.0.30000.0" {
		t.Fatalf("Version = %q, want %q", got.Version, "1.0.30000.0")
	}
	if got.Confidence != "medium" {
		t.Fatalf("Confidence = %q, want medium", got.Confidence)
	}
}

func TestParseVersionResourceRejectsGarbage(t *testing.T) {
	for _, payload := range [][]byte{nil, {1, 2, 3}, make([]byte, 64)} {
		if _, ok := parseVersionResource(payload); ok {
			t.Fatalf("parseVersionResource(%d bytes) = ok, want false", len(payload))
		}
	}
}

func TestExeVersionOnNonPE(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-an-exe.exe")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, ok := ExeVersion(path); ok {
		t.Fatal("ExeVersion on a non-PE file: want false")
	}
}

func TestExeVersionOnMissingFile(t *testing.T) {
	if _, ok := ExeVersion(filepath.Join(t.TempDir(), "nope.exe")); ok {
		t.Fatal("ExeVersion on a missing file: want false")
	}
}

// Живая проверка на настоящем windows-исполняемом файле: путь задаётся
// снаружи, потому что своего exe в репозитории нет и быть не должно.
func TestLiveExeVersion(t *testing.T) {
	path := os.Getenv("TYPHON_PE_LIVE")
	if path == "" {
		t.Skip("не задан TYPHON_PE_LIVE")
	}
	got, ok := ExeVersion(path)
	if !ok {
		t.Fatalf("ExeVersion(%s) = not found", path)
	}
	t.Logf("version=%q product=%q company=%q confidence=%s",
		got.Version, got.Product, got.Company, got.Confidence)
	if got.Version == "" {
		t.Fatal("версия пустая")
	}
}
