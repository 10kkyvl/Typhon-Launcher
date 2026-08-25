package install

import (
	"encoding/binary"
	"reflect"
	"strings"
	"testing"
	"unicode/utf16"
)

func encodeUTF16LEWithBOM(t *testing.T, s string) []byte {
	t.Helper()
	units := utf16.Encode([]rune(s))
	out := make([]byte, 2+2*len(units))
	out[0] = 0xFF
	out[1] = 0xFE
	for i, u := range units {
		binary.LittleEndian.PutUint16(out[2+2*i:], u)
	}
	return out
}

const tropicoInf = "[Setup]\r\nLang=ru\r\nDir=G:\\games\\TyphonLibrary\\_typhon-probe4\r\nGroup=by.xatab\r\nNoIcons=0\r\nSetupType=full\r\nComponents=compgame,lang,lang\\rus,redist,redist\\directxcheck,redist\\vccheck,desktopicon\r\nTasks=\r\n"

func TestInfComponents(t *testing.T) {
	cases := []struct {
		name      string
		data      []byte
		wantList  []string
		wantFound bool
	}{
		{
			name:      "utf-16le with bom, real inno byte shape",
			data:      encodeUTF16LEWithBOM(t, tropicoInf),
			wantList:  []string{"compgame", "lang", "lang\\rus", "redist", "redist\\directxcheck", "redist\\vccheck", "desktopicon"},
			wantFound: true,
		},
		{
			name:      "utf-8 without bom",
			data:      []byte(tropicoInf),
			wantList:  []string{"compgame", "lang", "lang\\rus", "redist", "redist\\directxcheck", "redist\\vccheck", "desktopicon"},
			wantFound: true,
		},
		{
			name:      "lf line endings",
			data:      []byte(strings.ReplaceAll(tropicoInf, "\r\n", "\n")),
			wantList:  []string{"compgame", "lang", "lang\\rus", "redist", "redist\\directxcheck", "redist\\vccheck", "desktopicon"},
			wantFound: true,
		},
		{
			name:      "components key missing",
			data:      []byte("[Setup]\r\nLang=ru\r\nDir=C:\\Games\\Foo\r\n"),
			wantList:  nil,
			wantFound: false,
		},
		{
			name:      "components key present but empty",
			data:      []byte("[Setup]\r\nComponents=\r\n"),
			wantList:  nil,
			wantFound: true,
		},
		{
			name:      "not an ini file at all",
			data:      []byte{0x00, 0x01, 0x02, 0xDE, 0xAD, 0xBE, 0xEF, 0x03, 0x04, 0x05},
			wantList:  nil,
			wantFound: false,
		},
		{
			name:      "spaces around key and equals sign",
			data:      []byte("[Setup]\r\n Components = a,b \r\n"),
			wantList:  []string{"a", "b"},
			wantFound: true,
		},
		{
			name:      "no setup section at all",
			data:      []byte("Components=a,b\r\n"),
			wantList:  nil,
			wantFound: false,
		},
		{
			name:      "empty input",
			data:      nil,
			wantList:  nil,
			wantFound: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, found := infComponents(tc.data)
			if found != tc.wantFound {
				t.Fatalf("infComponents() found = %v, want %v", found, tc.wantFound)
			}
			if !reflect.DeepEqual(got, tc.wantList) {
				t.Fatalf("infComponents() list = %#v, want %#v", got, tc.wantList)
			}
		})
	}
}

func TestFilterComponents(t *testing.T) {
	tropico := []string{"compgame", "lang", "lang\\rus", "redist", "redist\\directxcheck", "redist\\vccheck", "desktopicon"}

	cases := []struct {
		name        string
		list        []string
		opts        installOptions
		wantList    []string
		wantChanged bool
	}{
		{
			name:        "tropico with skip extras and shortcuts",
			list:        tropico,
			opts:        installOptions{SkipExtras: true, SkipShortcuts: true},
			wantList:    []string{"compgame", "lang", "lang\\rus"},
			wantChanged: true,
		},
		{
			name:        "no options set, list untouched",
			list:        tropico,
			opts:        installOptions{},
			wantList:    tropico,
			wantChanged: false,
		},
		{
			name:        "everything in the list matches the dict, original list returned",
			list:        []string{"redist", "redist\\directxcheck", "desktopicon"},
			opts:        installOptions{SkipExtras: true, SkipShortcuts: true},
			wantList:    []string{"redist", "redist\\directxcheck", "desktopicon"},
			wantChanged: false,
		},
		{
			name:        "case insensitive segment match",
			list:        []string{"compgame", "Redist\\DirectXCheck"},
			opts:        installOptions{SkipExtras: true},
			wantList:    []string{"compgame"},
			wantChanged: true,
		},
		{
			name:        "short marker dx does not match substrings sdx or dxtory",
			list:        []string{"sdx", "dxtory", "compgame"},
			opts:        installOptions{SkipExtras: true, SkipShortcuts: true},
			wantList:    []string{"sdx", "dxtory", "compgame"},
			wantChanged: false,
		},
		{
			name:        "empty input list",
			list:        nil,
			opts:        installOptions{SkipExtras: true, SkipShortcuts: true},
			wantList:    nil,
			wantChanged: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, changed := filterComponents(tc.list, tc.opts)
			if changed != tc.wantChanged {
				t.Fatalf("filterComponents() changed = %v, want %v (list=%v)", changed, tc.wantChanged, got)
			}
			if !reflect.DeepEqual(got, tc.wantList) {
				t.Fatalf("filterComponents() list = %#v, want %#v", got, tc.wantList)
			}
		})
	}
}
