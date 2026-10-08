package winpath

import "strings"

var deviceNames = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true, "CONIN$": true, "CONOUT$": true,
}

// Reserved is decided by this table, not by asking the running system: Windows
// 11 dropped the reservation of device names with an extension (aux.txt) that
// Windows 10 still enforces, and a torrent or manifest accepted on one must not
// turn into an unwritable path on the other.
func Reserved(name string) bool {
	if strings.HasSuffix(name, ".") || strings.HasSuffix(name, " ") {
		return true
	}
	stem := name
	if i := strings.IndexByte(stem, '.'); i >= 0 {
		stem = stem[:i]
	}
	stem = strings.ToUpper(strings.TrimRight(stem, " "))
	if deviceNames[stem] {
		return true
	}
	runes := []rune(stem)
	if len(runes) != 4 {
		return false
	}
	if prefix := string(runes[:3]); prefix != "COM" && prefix != "LPT" {
		return false
	}
	last := runes[3]
	return last >= '0' && last <= '9' || last == '¹' || last == '²' || last == '³'
}
