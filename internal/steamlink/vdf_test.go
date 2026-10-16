package steamlink

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"
)

func vMap(key string) []byte { return append(append([]byte{typeMap}, key...), 0) }

func vStr(key, val string) []byte {
	out := append([]byte{typeString}, key...)
	out = append(out, 0)
	out = append(out, val...)
	return append(out, 0)
}

func vInt(key string, v uint32) []byte {
	out := append([]byte{typeInt}, key...)
	out = append(out, 0)
	return binary.LittleEndian.AppendUint32(out, v)
}

func vU64(key string, v uint64) []byte {
	out := append([]byte{typeUint64}, key...)
	out = append(out, 0)
	return binary.LittleEndian.AppendUint64(out, v)
}

func vEnd() []byte { return []byte{typeEnd} }

func cat(parts ...[]byte) []byte { return bytes.Join(parts, nil) }

func foreignEntry(index string, appID uint32, name string) []byte {
	return cat(
		vMap(index),
		vInt("appid", appID),
		vStr("AppName", name),
		vStr("Exe", `"C:\Games\other.exe"`),
		vStr("StartDir", `"C:\Games"`),
		vStr("icon", ""),
		vStr("ShortcutPath", ""),
		vStr("LaunchOptions", "-windowed"),
		vInt("IsHidden", 0),
		vU64("FutureField", 0x1122334455667788),
		vInt("LastPlayTime", 1700000000),
		vMap("tags"), vStr("0", "favorite"), vEnd(),
		vEnd(),
	)
}

func sampleFile(entries ...[]byte) []byte {
	return cat(vMap("shortcuts"), cat(entries...), vEnd(), vEnd())
}

func TestVDFRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		data []byte
	}{
		{"empty list", sampleFile()},
		{"one entry with unknown u64 and tags", sampleFile(foreignEntry("0", 0x80000001, "Other"))},
		{"two entries", sampleFile(foreignEntry("0", 0x80000001, "Other"), foreignEntry("1", 0x80000002, "Another"))},
		{"top-level extras", cat(vMap("shortcuts"), vEnd(), vStr("extra", "keep"), vU64("big", 7), vEnd())},
		{"non-utf8 bytes", sampleFile(cat(vMap("0"), vStr("AppName", "\xff\xfe"), vEnd()))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			top, err := parseVDF(tc.data)
			if err != nil {
				t.Fatalf("parseVDF: %v", err)
			}
			if got := encodeVDF(top); !bytes.Equal(got, tc.data) {
				t.Fatalf("round trip differs\n got %x\nwant %x", got, tc.data)
			}
		})
	}
}

func TestVDFRejectsDamagedInput(t *testing.T) {
	good := sampleFile(foreignEntry("0", 0x80000001, "Other"))
	deep := bytes.Repeat(vMap("a"), maxVDFDepth+1)

	cases := []struct {
		name string
		data []byte
		want error
	}{
		{"empty", nil, errVDFEmpty},
		{"unknown type", cat(vMap("shortcuts"), []byte{0x05, 'k', 0, 0}, vEnd(), vEnd()), errVDFType},
		{"missing final end", good[:len(good)-1], errVDFTruncated},
		{"missing both ends", good[:len(good)-2], errVDFTruncated},
		{"trailing data", cat(good, []byte{0x00}), errVDFTrailing},
		{"too deep", deep, errVDFDepth},
		{"oversized", make([]byte, maxVDFBytes+1), errVDFTooLarge},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseVDF(tc.data); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}

	t.Run("every proper prefix fails", func(t *testing.T) {
		for n := 1; n < len(good); n++ {
			if _, err := parseVDF(good[:n]); err == nil {
				t.Fatalf("prefix of %d/%d bytes parsed without error", n, len(good))
			}
		}
	})
}
