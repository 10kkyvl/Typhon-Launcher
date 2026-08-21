package feed

import "testing"

func TestMagnetInfoHashHex(t *testing.T) {
	hash, ok := MagnetInfoHash("magnet:?xt=urn:btih:AABBCCDDEEFF00112233445566778899AABBCCDD&dn=Test")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if hash != "aabbccddeeff00112233445566778899aabbccdd" {
		t.Errorf("hash = %q", hash)
	}
}

func TestMagnetInfoHashBase32(t *testing.T) {
	hex, ok := MagnetInfoHash("magnet:?xt=urn:btih:AABBCCDDEEFF00112233445566778899AABBCCDD")
	if !ok {
		t.Fatal("expected ok=true")
	}

	b32, ok := MagnetInfoHash("magnet:?xt=urn:btih:VK54ZXPO74ABCIRTIRKWM54ITGVLXTG5")
	if !ok {
		t.Fatal("expected ok=true for base32")
	}
	if b32 != hex {
		t.Errorf("base32 decode = %q, want %q", b32, hex)
	}
}

func TestMagnetInfoHashMultipleParams(t *testing.T) {
	hash, ok := MagnetInfoHash("magnet:?dn=Test&tr=udp://tracker&xt=urn:btih:AABBCCDDEEFF00112233445566778899AABBCCDD&tr=udp://tracker2")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if hash != "aabbccddeeff00112233445566778899aabbccdd" {
		t.Errorf("hash = %q", hash)
	}
}

func TestMagnetInfoHashCaseInsensitivePrefix(t *testing.T) {
	hash, ok := MagnetInfoHash("magnet:?xt=URN:BTIH:AABBCCDDEEFF00112233445566778899AABBCCDD")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if hash != "aabbccddeeff00112233445566778899aabbccdd" {
		t.Errorf("hash = %q", hash)
	}
}

func TestMagnetInfoHashMultipleXT(t *testing.T) {
	hash, ok := MagnetInfoHash("magnet:?xt=urn:sha1:garbage&xt=urn:btih:AABBCCDDEEFF00112233445566778899AABBCCDD")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if hash != "aabbccddeeff00112233445566778899aabbccdd" {
		t.Errorf("hash = %q", hash)
	}
}

func TestMagnetInfoHashGarbage(t *testing.T) {
	cases := []string{
		"",
		"not a magnet",
		"http://example.com/file.torrent",
		"magnet:?dn=NoXT",
		"magnet:?xt=urn:btih:tooShort",
		"magnet:?xt=urn:btih:AABBCCDDEEFF00112233445566778899AABBCCDDZZ",
	}
	for _, c := range cases {
		if hash, ok := MagnetInfoHash(c); ok {
			t.Errorf("case %q: expected ok=false, got hash=%q", c, hash)
		}
	}
}
