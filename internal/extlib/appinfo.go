package extlib

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

const (
	appInfoMagic27 = 0x07564427
	appInfoMagic28 = 0x07564428
	appInfoMagic29 = 0x07564429

	maxAppInfoBytes      = 1 << 30
	maxAppInfoEntryBytes = 8 << 20
	maxAppInfoTableBytes = 64 << 20
	maxKVDepth           = 32
)

var (
	errAppInfoMagic     = errors.New("неизвестная версия appinfo.vdf")
	errAppInfoCorrupt   = errors.New("appinfo.vdf повреждён")
	errAppInfoTooLarge  = errors.New("appinfo.vdf слишком большой")
	errAppInfoNoStrings = errors.New("в appinfo.vdf нет таблицы строк")
)

const (
	kvNested  = 0x00
	kvString  = 0x01
	kvInt32   = 0x02
	kvFloat   = 0x03
	kvPointer = 0x04
	kvWide    = 0x05
	kvColor   = 0x06
	kvUint64  = 0x07
	kvEnd     = 0x08
	kvEndAlt  = 0x0B
	kvInt64   = 0x0A
)

func readAppTypes(ctx context.Context, path string, wanted map[uint32]bool) (map[string]string, error) {
	f, err := os.Open(path) //nolint:gosec // G703: путь собран из корня Steam, размер ограничен maxAppInfoBytes (инварианты 32, 34)
	if err != nil {
		return nil, err
	}
	types, readErr := parseAppInfo(ctx, f, wanted)
	closeErr := f.Close()
	if readErr != nil {
		return nil, fmt.Errorf("%s: %w", path, readErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close %s: %w", path, closeErr)
	}
	return types, nil
}

type appInfoFile interface {
	io.ReaderAt
	Stat() (os.FileInfo, error)
}

func parseAppInfo(ctx context.Context, f appInfoFile, wanted map[uint32]bool) (map[string]string, error) {
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	size := st.Size()
	if size > maxAppInfoBytes {
		return nil, errAppInfoTooLarge
	}
	var head [16]byte
	n, err := f.ReadAt(head[:8], 0)
	if n < 8 {
		return nil, fmt.Errorf("%w: заголовок: %w", errAppInfoCorrupt, orEOF(err))
	}
	magic := binary.LittleEndian.Uint32(head[0:4])
	var strs []string
	start := int64(8)
	end := size
	switch magic {
	case appInfoMagic27, appInfoMagic28:
	case appInfoMagic29:
		n, err = f.ReadAt(head[:], 0)
		if n < len(head) {
			return nil, fmt.Errorf("%w: заголовок: %w", errAppInfoCorrupt, orEOF(err))
		}
		lo, hi := binary.LittleEndian.Uint32(head[8:12]), binary.LittleEndian.Uint32(head[12:16])
		off := int64(lo)
		if hi != 0 || off < 16 || off >= size {
			return nil, fmt.Errorf("%w: смещение таблицы строк %d:%d при размере %d", errAppInfoNoStrings, hi, lo, size)
		}
		strs, err = readStringTable(io.NewSectionReader(f, off, size-off), size-off)
		if err != nil {
			return nil, err
		}
		start = 16
		end = off
	default:
		return nil, fmt.Errorf("%w: 0x%08x", errAppInfoMagic, magic)
	}

	meta := int64(60)
	if magic == appInfoMagic27 {
		meta = 40
	}
	types := map[string]string{}
	pos := start
	var hdr [8]byte
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := readFullAt(f, hdr[:4], pos, end); err != nil {
			return nil, fmt.Errorf("%w: запись: %w", errAppInfoCorrupt, err)
		}
		appID := binary.LittleEndian.Uint32(hdr[0:4])
		if appID == 0 {
			return types, nil
		}
		if err := readFullAt(f, hdr[4:8], pos+4, end); err != nil {
			return nil, fmt.Errorf("%w: размер записи %d: %w", errAppInfoCorrupt, appID, err)
		}
		entry := int64(binary.LittleEndian.Uint32(hdr[4:8]))
		if entry < meta {
			return nil, fmt.Errorf("%w: запись %d короче служебной части", errAppInfoCorrupt, appID)
		}
		bodyAt := pos + 8
		pos = bodyAt + entry
		if !wanted[appID] {
			if pos > end {
				return nil, fmt.Errorf("%w: запись %d выходит за конец данных", errAppInfoCorrupt, appID)
			}
			continue
		}
		if entry-meta > maxAppInfoEntryBytes {
			return nil, fmt.Errorf("%w: запись %d", errAppInfoTooLarge, appID)
		}
		body := make([]byte, entry-meta)
		if err := readFullAt(f, body, bodyAt+meta, end); err != nil {
			return nil, fmt.Errorf("%w: запись %d: %w", errAppInfoCorrupt, appID, err)
		}
		typ, err := appTypeOf(body, strs)
		if err != nil {
			return nil, fmt.Errorf("запись %d: %w", appID, err)
		}
		if typ != "" {
			types[strconv.FormatUint(uint64(appID), 10)] = typ
		}
	}
}

func readFullAt(f io.ReaderAt, buf []byte, off, end int64) error {
	if off < 0 || off+int64(len(buf)) > end {
		return io.ErrUnexpectedEOF
	}
	n, err := f.ReadAt(buf, off)
	if n == len(buf) {
		return nil
	}
	return orEOF(err)
}

func orEOF(err error) error {
	if err == nil || errors.Is(err, io.EOF) {
		return io.ErrUnexpectedEOF
	}
	return err
}

func readStringTable(src io.Reader, limit int64) ([]string, error) {
	if limit > maxAppInfoTableBytes {
		return nil, fmt.Errorf("%w: таблица строк", errAppInfoTooLarge)
	}
	data, err := io.ReadAll(io.LimitReader(src, limit))
	if err != nil {
		return nil, fmt.Errorf("%w: таблица строк: %w", errAppInfoCorrupt, err)
	}
	if len(data) < 4 {
		return nil, fmt.Errorf("%w: таблица строк обрезана", errAppInfoCorrupt)
	}
	count := int64(binary.LittleEndian.Uint32(data[:4]))
	data = data[4:]
	if count > int64(len(data)) {
		return nil, fmt.Errorf("%w: в таблице %d строк при %d байтах", errAppInfoCorrupt, count, len(data))
	}
	out := make([]string, 0, count)
	for i := int64(0); i < count; i++ {
		j := bytes.IndexByte(data, 0)
		if j < 0 {
			return nil, fmt.Errorf("%w: строка %d таблицы не завершена", errAppInfoCorrupt, i)
		}
		out = append(out, string(data[:j]))
		data = data[j+1:]
	}
	return out, nil
}

type kvReader struct {
	data []byte
	pos  int
	strs []string
}

func (k *kvReader) need(n int) error {
	if n < 0 || k.pos+n > len(k.data) {
		return fmt.Errorf("%w: обрыв данных записи", errAppInfoCorrupt)
	}
	return nil
}

func (k *kvReader) key() (string, error) {
	if k.strs == nil {
		return k.cstring()
	}
	if err := k.need(4); err != nil {
		return "", err
	}
	idx := binary.LittleEndian.Uint32(k.data[k.pos:])
	k.pos += 4
	if int64(idx) >= int64(len(k.strs)) {
		return "", fmt.Errorf("%w: индекс ключа %d вне таблицы из %d строк", errAppInfoCorrupt, idx, len(k.strs))
	}
	return k.strs[idx], nil
}

func (k *kvReader) cstring() (string, error) {
	j := bytes.IndexByte(k.data[k.pos:], 0)
	if j < 0 {
		return "", fmt.Errorf("%w: строка не завершена", errAppInfoCorrupt)
	}
	s := string(k.data[k.pos : k.pos+j])
	k.pos += j + 1
	return s, nil
}

func (k *kvReader) skip(n int) error {
	if err := k.need(n); err != nil {
		return err
	}
	k.pos += n
	return nil
}

func (k *kvReader) wide() error {
	for {
		if err := k.need(2); err != nil {
			return err
		}
		c := binary.LittleEndian.Uint16(k.data[k.pos:])
		k.pos += 2
		if c == 0 {
			return nil
		}
	}
}

// object читает ключи до kvEnd, спускаясь по path; строка, лежащая в конце
// path, возвращается как found. Остальные значения пропускаются с проверкой границ.
func (k *kvReader) object(depth int, path []string) (string, error) {
	if depth > maxKVDepth {
		return "", fmt.Errorf("%w: слишком глубокая вложенность", errAppInfoCorrupt)
	}
	found := ""
	for {
		if depth == 0 && k.pos == len(k.data) {
			return found, nil
		}
		if err := k.need(1); err != nil {
			return "", err
		}
		t := k.data[k.pos]
		k.pos++
		if t == kvEnd || t == kvEndAlt {
			return found, nil
		}
		name, err := k.key()
		if err != nil {
			return "", err
		}
		match := len(path) > 0 && strings.EqualFold(name, path[0])
		switch t {
		case kvNested:
			var sub []string
			if match {
				sub = path[1:]
			}
			v, err := k.object(depth+1, sub)
			if err != nil {
				return "", err
			}
			if match && v != "" {
				found = v
			}
		case kvString:
			s, err := k.cstring()
			if err != nil {
				return "", err
			}
			if match && len(path) == 1 {
				found = s
			}
		case kvInt32, kvFloat, kvPointer, kvColor:
			err = k.skip(4)
		case kvUint64, kvInt64:
			err = k.skip(8)
		case kvWide:
			err = k.wide()
		default:
			err = fmt.Errorf("%w: неизвестный тип значения 0x%02x", errAppInfoCorrupt, t)
		}
		if err != nil {
			return "", err
		}
	}
}

func appTypeOf(body []byte, strs []string) (string, error) {
	k := &kvReader{data: body, strs: strs}
	typ, err := k.object(0, []string{"appinfo", "common", "type"})
	if err != nil {
		return "", err
	}
	return strings.ToLower(strings.TrimSpace(typ)), nil
}
