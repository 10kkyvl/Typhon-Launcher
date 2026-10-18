package extlib

import (
	"errors"
	"fmt"
	"strings"
)

const maxVDFDepth = 32

var errVDF = errors.New("некорректный VDF")

type vdfNode struct {
	value    string
	children map[string]*vdfNode
}

func (n *vdfNode) child(key string) *vdfNode {
	if n == nil {
		return nil
	}
	for k, c := range n.children {
		if strings.EqualFold(k, key) {
			return c
		}
	}
	return nil
}

func (n *vdfNode) str(key string) string {
	c := n.child(key)
	if c == nil || c.children != nil {
		return ""
	}
	return c.value
}

type vdfParser struct {
	src string
	pos int
}

func parseVDF(data []byte) (*vdfNode, error) {
	p := &vdfParser{src: string(data)}
	root := &vdfNode{children: map[string]*vdfNode{}}
	if err := p.block(root, 0, true); err != nil {
		return nil, err
	}
	if len(root.children) == 0 {
		return nil, fmt.Errorf("%w: пустой документ", errVDF)
	}
	return root, nil
}

func (p *vdfParser) skip() {
	for p.pos < len(p.src) {
		c := p.src[p.pos]
		switch {
		case c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == 0xEF || c == 0xBB || c == 0xBF:
			p.pos++
		case c == '/' && p.pos+1 < len(p.src) && p.src[p.pos+1] == '/':
			for p.pos < len(p.src) && p.src[p.pos] != '\n' {
				p.pos++
			}
		default:
			return
		}
	}
}

func (p *vdfParser) quoted() (string, error) {
	p.pos++
	var b strings.Builder
	for p.pos < len(p.src) {
		c := p.src[p.pos]
		switch c {
		case '"':
			p.pos++
			return b.String(), nil
		case '\\':
			p.pos++
			if p.pos >= len(p.src) {
				return "", fmt.Errorf("%w: оборванная escape-последовательность", errVDF)
			}
			switch e := p.src[p.pos]; e {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case '\\', '"':
				b.WriteByte(e)
			default:
				b.WriteByte('\\')
				b.WriteByte(e)
			}
			p.pos++
		default:
			b.WriteByte(c)
			p.pos++
		}
	}
	return "", fmt.Errorf("%w: не закрыта строка", errVDF)
}

func (p *vdfParser) block(into *vdfNode, depth int, top bool) error {
	if depth > maxVDFDepth {
		return fmt.Errorf("%w: слишком глубокая вложенность", errVDF)
	}
	for {
		p.skip()
		if p.pos >= len(p.src) {
			if top {
				return nil
			}
			return fmt.Errorf("%w: не закрыт блок", errVDF)
		}
		switch p.src[p.pos] {
		case '}':
			if top {
				return fmt.Errorf("%w: лишняя закрывающая скобка", errVDF)
			}
			p.pos++
			return nil
		case '"':
		default:
			return fmt.Errorf("%w: ожидалась строка в позиции %d", errVDF, p.pos)
		}
		key, err := p.quoted()
		if err != nil {
			return err
		}
		p.skip()
		if p.pos >= len(p.src) {
			return fmt.Errorf("%w: ключ %q без значения", errVDF, key)
		}
		node := &vdfNode{}
		switch p.src[p.pos] {
		case '"':
			if node.value, err = p.quoted(); err != nil {
				return err
			}
		case '{':
			p.pos++
			node.children = map[string]*vdfNode{}
			if err := p.block(node, depth+1, false); err != nil {
				return err
			}
		default:
			return fmt.Errorf("%w: ключ %q без значения", errVDF, key)
		}
		into.children[key] = node
	}
}
