package steamlink

import (
	"errors"
	"fmt"
	"hash/crc32"
	"strconv"
	"strings"
)

// Shortcut is one non-Steam game entry as Typhon describes it. Everything else
// Steam stores about an entry stays untouched.
type Shortcut struct {
	AppID         uint32
	Name          string
	Exe           string
	StartDir      string
	Icon          string
	LaunchOptions string
}

// AppID is the stable Steam app id of the shortcut for a library game. The high
// bit marks non-Steam apps, which is how Steam derives the id of its own
// shortcuts.
func AppID(gameID string) uint32 {
	return crc32.ChecksumIEEE([]byte("typhon:"+gameID)) | 0x80000000
}

type document struct {
	top []*node
}

func parseShortcuts(data []byte) (*document, error) {
	if len(data) == 0 {
		return &document{}, nil
	}
	top, err := parseVDF(data)
	if err != nil {
		return nil, err
	}
	d := &document{top: top}
	if list := d.find(); list != nil && list.kind != typeMap {
		return nil, fmt.Errorf("%w: shortcuts не вложенная карта", errVDFType)
	}
	return d, nil
}

func (d *document) bytes() []byte {
	return encodeVDF(d.top)
}

func (d *document) find() *node {
	for _, n := range d.top {
		if strings.EqualFold(n.key, "shortcuts") {
			return n
		}
	}
	return nil
}

func (d *document) ensure() *node {
	if list := d.find(); list != nil {
		return list
	}
	list := &node{key: "shortcuts", kind: typeMap}
	d.top = append(d.top, list)
	return list
}

func field(entry *node, name string) *node {
	for _, n := range entry.kids {
		if strings.EqualFold(n.key, name) {
			return n
		}
	}
	return nil
}

func entryAppID(entry *node) (uint32, bool) {
	if entry.kind != typeMap {
		return 0, false
	}
	f := field(entry, "appid")
	if f == nil || f.kind != typeInt {
		return 0, false
	}
	return f.i32, true
}

func (d *document) index(appID uint32) int {
	list := d.find()
	if list == nil {
		return -1
	}
	for i, entry := range list.kids {
		if id, ok := entryAppID(entry); ok && id == appID {
			return i
		}
	}
	return -1
}

func (d *document) has(appID uint32) bool {
	return d.index(appID) >= 0
}

var errBadShortcut = errors.New("некорректный ярлык Steam")

// Validate rejects what cannot be written as a shortcut: a missing field, or a
// NUL byte that would end a VDF string early and shift everything after it.
func (sc Shortcut) Validate() error {
	if sc.AppID == 0 || strings.TrimSpace(sc.Name) == "" || strings.TrimSpace(sc.Exe) == "" {
		return fmt.Errorf("%w: нет идентификатора, названия или исполняемого файла", errBadShortcut)
	}
	for name, value := range map[string]string{
		"название": sc.Name, "исполняемый файл": sc.Exe, "рабочая папка": sc.StartDir,
		"значок": sc.Icon, "параметры запуска": sc.LaunchOptions,
	} {
		if strings.ContainsRune(value, 0) {
			return fmt.Errorf("%w: поле «%s» содержит нулевой символ", errBadShortcut, name)
		}
	}
	return nil
}

func quoted(s string) string {
	return `"` + s + `"`
}

// upsert adds the shortcut or rewrites the fields Typhon owns in the entry that
// already carries its app id, so playtime, tags and anything else Steam or the
// user stored there survives. It reports whether the document changed.
func (d *document) upsert(sc Shortcut) bool {
	if d.update(sc) {
		return true
	}
	if d.has(sc.AppID) {
		return false
	}
	list := d.ensure()
	entry := &node{kind: typeMap, kids: []*node{
		{key: "appid", kind: typeInt, i32: sc.AppID},
		{key: "AppName", kind: typeString, str: sc.Name},
		{key: "Exe", kind: typeString, str: quoted(sc.Exe)},
		{key: "StartDir", kind: typeString, str: quoted(sc.StartDir)},
		{key: "icon", kind: typeString, str: sc.Icon},
		{key: "ShortcutPath", kind: typeString},
		{key: "LaunchOptions", kind: typeString, str: sc.LaunchOptions},
		{key: "IsHidden", kind: typeInt},
		{key: "AllowDesktopConfig", kind: typeInt, i32: 1},
		{key: "AllowOverlay", kind: typeInt, i32: 1},
		{key: "OpenVR", kind: typeInt},
		{key: "Devkit", kind: typeInt},
		{key: "DevkitGameID", kind: typeString},
		{key: "DevkitOverrideAppID", kind: typeInt},
		{key: "LastPlayTime", kind: typeInt},
		{key: "FlatpakAppID", kind: typeString},
		{key: "tags", kind: typeMap},
	}}
	list.kids = append(list.kids, entry)
	reindex(list)
	return true
}

// update rewrites the fields Typhon owns in every entry that already carries the
// app id and adds nothing.
func (d *document) update(sc Shortcut) bool {
	owned := []struct {
		name  string
		value string
	}{
		{"AppName", sc.Name},
		{"Exe", quoted(sc.Exe)},
		{"StartDir", quoted(sc.StartDir)},
		{"icon", sc.Icon},
		{"LaunchOptions", sc.LaunchOptions},
	}
	list := d.find()
	if list == nil {
		return false
	}
	changed := false
	for _, entry := range list.kids {
		if id, ok := entryAppID(entry); !ok || id != sc.AppID {
			continue
		}
		for _, o := range owned {
			if setString(entry, o.name, o.value) {
				changed = true
			}
		}
	}
	return changed
}

func setString(entry *node, name, value string) bool {
	f := field(entry, name)
	if f == nil {
		entry.kids = append(entry.kids, &node{key: name, kind: typeString, str: value})
		return true
	}
	if f.kind == typeString && f.str == value {
		return false
	}
	f.kind = typeString
	f.str = value
	f.i32 = 0
	f.num = 0
	f.kids = nil
	return true
}

// remove drops every entry with the app id, duplicates included, and renumbers
// the rest, which Steam expects to be a gapless "0".."n-1" sequence.
func (d *document) remove(appID uint32) bool {
	list := d.find()
	if list == nil {
		return false
	}
	kept := make([]*node, 0, len(list.kids))
	for _, entry := range list.kids {
		if id, ok := entryAppID(entry); ok && id == appID {
			continue
		}
		kept = append(kept, entry)
	}
	if len(kept) == len(list.kids) {
		return false
	}
	list.kids = kept
	reindex(list)
	return true
}

func reindex(list *node) {
	for i, entry := range list.kids {
		entry.key = strconv.Itoa(i)
	}
}
