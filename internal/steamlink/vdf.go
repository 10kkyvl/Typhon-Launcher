package steamlink

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	typeMap    byte = 0x00
	typeString byte = 0x01
	typeInt    byte = 0x02
	typeUint64 byte = 0x07
	typeEnd    byte = 0x08

	maxVDFBytes = 16 << 20
	maxVDFDepth = 16
)

var (
	errVDFEmpty     = errors.New("пустой файл")
	errVDFTooLarge  = errors.New("файл слишком большой")
	errVDFTruncated = errors.New("файл оборван")
	errVDFType      = errors.New("неизвестный тип записи")
	errVDFDepth     = errors.New("слишком глубокая вложенность")
	errVDFTrailing  = errors.New("лишние данные после конца файла")
)

// node is one entry of a binary VDF tree. Keeping every entry, including the
// types this package never reads, is what lets a rewrite preserve the parts of
// shortcuts.vdf that belong to Steam or to other tools.
type node struct {
	key  string
	kind byte
	str  string
	i32  uint32
	num  uint64
	kids []*node
}

func parseVDF(data []byte) ([]*node, error) {
	if len(data) == 0 {
		return nil, errVDFEmpty
	}
	if len(data) > maxVDFBytes {
		return nil, errVDFTooLarge
	}
	p := &vdfParser{data: data}
	kids, err := p.body(1)
	if err != nil {
		return nil, err
	}
	if p.pos != len(data) {
		return nil, fmt.Errorf("%w: смещение %d", errVDFTrailing, p.pos)
	}
	return kids, nil
}

type vdfParser struct {
	data []byte
	pos  int
}

func (p *vdfParser) body(depth int) ([]*node, error) {
	if depth > maxVDFDepth {
		return nil, errVDFDepth
	}
	var kids []*node
	for {
		if p.pos >= len(p.data) {
			return nil, fmt.Errorf("%w: нет завершающего 0x08", errVDFTruncated)
		}
		kind := p.data[p.pos]
		p.pos++
		if kind == typeEnd {
			return kids, nil
		}
		switch kind {
		case typeMap, typeString, typeInt, typeUint64:
		default:
			return nil, fmt.Errorf("%w 0x%02x, смещение %d", errVDFType, kind, p.pos-1)
		}
		key, err := p.cstring()
		if err != nil {
			return nil, err
		}
		n := &node{key: key, kind: kind}
		switch kind {
		case typeMap:
			n.kids, err = p.body(depth + 1)
			if err != nil {
				return nil, err
			}
		case typeString:
			n.str, err = p.cstring()
			if err != nil {
				return nil, err
			}
		case typeInt:
			raw, err := p.take(4)
			if err != nil {
				return nil, err
			}
			n.i32 = binary.LittleEndian.Uint32(raw)
		case typeUint64:
			raw, err := p.take(8)
			if err != nil {
				return nil, err
			}
			n.num = binary.LittleEndian.Uint64(raw)
		}
		kids = append(kids, n)
	}
}

func (p *vdfParser) cstring() (string, error) {
	end := bytes.IndexByte(p.data[p.pos:], 0)
	if end < 0 {
		return "", fmt.Errorf("%w: строка без 0x00", errVDFTruncated)
	}
	s := string(p.data[p.pos : p.pos+end])
	p.pos += end + 1
	return s, nil
}

func (p *vdfParser) take(n int) ([]byte, error) {
	if p.pos+n > len(p.data) {
		return nil, fmt.Errorf("%w: число обрезано", errVDFTruncated)
	}
	raw := p.data[p.pos : p.pos+n]
	p.pos += n
	return raw, nil
}

func encodeVDF(kids []*node) []byte {
	var buf bytes.Buffer
	encodeBody(&buf, kids)
	return buf.Bytes()
}

func encodeBody(buf *bytes.Buffer, kids []*node) {
	for _, n := range kids {
		buf.WriteByte(n.kind)
		buf.WriteString(n.key)
		buf.WriteByte(0)
		switch n.kind {
		case typeMap:
			encodeBody(buf, n.kids)
		case typeString:
			buf.WriteString(n.str)
			buf.WriteByte(0)
		case typeInt:
			buf.Write(binary.LittleEndian.AppendUint32(nil, n.i32))
		case typeUint64:
			buf.Write(binary.LittleEndian.AppendUint64(nil, n.num))
		}
	}
	buf.WriteByte(typeEnd)
}
