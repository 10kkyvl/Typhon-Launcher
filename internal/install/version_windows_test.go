package install

import (
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// TestFixedVersionSizeOK closes finding 6 (version_windows.go): fixedVersion
// used to check only size == 0 before casting the VerQueryValue result to
// *windows.VS_FIXEDFILEINFO, so a truncated version resource (size between 1
// and sizeof(VS_FIXEDFILEINFO)-1) would still be cast and read past its own
// bounds. translationID already guards its own cast this way for
// \VarFileInfo\Translation; fixedVersion must do the same for \.
func TestFixedVersionSizeOK(t *testing.T) {
	want := uint32(unsafe.Sizeof(windows.VS_FIXEDFILEINFO{}))
	cases := []struct {
		name string
		size uint32
		ok   bool
	}{
		{"zero", 0, false},
		{"one byte", 1, false},
		{"one short of the struct", want - 1, false},
		{"exact struct size", want, true},
		{"larger than the struct", want + 8, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := fixedVersionSizeOK(tc.size); got != tc.ok {
				t.Fatalf("fixedVersionSizeOK(%d) = %v, want %v (struct size = %d)", tc.size, got, tc.ok, want)
			}
		})
	}
}
