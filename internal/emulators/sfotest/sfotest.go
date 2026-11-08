// Package sfotest writes param.sfo files for tests of other packages.
package sfotest

import (
	"encoding/binary"
	"slices"
)

const (
	headerSize = 20
	entrySize  = 16
	utf8Format = 0x0204
)

// Bytes encodes the fields in the PSF format of the PlayStation consoles.
func Bytes(fields map[string]string) []byte {
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	var names, values, table []byte
	for _, key := range keys {
		value := append([]byte(fields[key]), 0)
		table = binary.LittleEndian.AppendUint16(table, narrow16(len(names)))
		table = binary.LittleEndian.AppendUint16(table, utf8Format)
		table = binary.LittleEndian.AppendUint32(table, narrow32(len(value)))
		table = binary.LittleEndian.AppendUint32(table, narrow32(len(value)))
		table = binary.LittleEndian.AppendUint32(table, narrow32(len(values)))
		names = append(append(names, key...), 0)
		values = append(values, value...)
	}
	keyStart := headerSize + len(table)

	out := []byte("\x00PSF\x01\x01\x00\x00")
	out = binary.LittleEndian.AppendUint32(out, narrow32(keyStart))
	out = binary.LittleEndian.AppendUint32(out, narrow32(keyStart+len(names)))
	out = binary.LittleEndian.AppendUint32(out, narrow32(len(keys)))
	out = append(append(out, table...), names...)
	return append(out, values...)
}

func narrow32(n int) uint32 {
	return uint32(n & 0xFFFFFFFF)
}

func narrow16(n int) uint16 {
	return uint16(n & 0xFFFF)
}
