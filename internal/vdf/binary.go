package vdf

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
)

// Binary KeyValues, the format of Steam's shortcuts.vdf (non-Steam games).
// Each entry is a type byte, a NUL-terminated key, then the value; a map
// ends with typeEnd. Values the launcher doesn't edit are kept byte for
// byte, so a round trip writes the file back unchanged.
const (
	typeMap    = 0x00
	typeString = 0x01
	typeInt    = 0x02 // little-endian uint32
	typeFloat  = 0x03
	typePtr    = 0x04
	typeColor  = 0x06
	typeUint64 = 0x07
	typeEnd    = 0x08
	typeInt64  = 0x0a
)

// BinNode is one entry of a binary KeyValues document. Maps have Children;
// strings have Str; ints have Int; other fixed-size values keep Raw.
type BinNode struct {
	Type     byte
	Key      string
	Str      string
	Int      uint32
	Raw      []byte
	Children []*BinNode
}

// BinMap makes an empty map entry.
func BinMap(key string) *BinNode { return &BinNode{Type: typeMap, Key: key} }

// AddString appends a string entry to a map.
func (n *BinNode) AddString(key, val string) {
	n.Children = append(n.Children, &BinNode{Type: typeString, Key: key, Str: val})
}

// AddInt appends a 32-bit integer entry to a map.
func (n *BinNode) AddInt(key string, val uint32) {
	n.Children = append(n.Children, &BinNode{Type: typeInt, Key: key, Int: val})
}

// Add appends a child entry to a map.
func (n *BinNode) Add(c *BinNode) { n.Children = append(n.Children, c) }

// Child returns the direct child with this key (case-insensitive), or nil.
func (n *BinNode) Child(key string) *BinNode {
	for _, c := range n.Children {
		if strings.EqualFold(c.Key, key) {
			return c
		}
	}
	return nil
}

// IsMap reports whether the entry is a map.
func (n *BinNode) IsMap() bool { return n.Type == typeMap }

// ParseBinary reads a binary KeyValues document. The result is a root map
// (with no key of its own) holding the top-level entries.
func ParseBinary(data []byte) (*BinNode, error) {
	r := &binReader{b: data}
	root := &BinNode{Type: typeMap}
	if err := r.mapBody(root); err != nil {
		return nil, err
	}
	if r.pos != len(r.b) {
		return nil, fmt.Errorf("%d unexpected bytes after the end", len(r.b)-r.pos)
	}
	return root, nil
}

type binReader struct {
	b   []byte
	pos int
}

func (r *binReader) cstring() (string, error) {
	i := bytes.IndexByte(r.b[r.pos:], 0)
	if i < 0 {
		return "", errors.New("unterminated string")
	}
	s := string(r.b[r.pos : r.pos+i])
	r.pos += i + 1
	return s, nil
}

func (r *binReader) fixed(n int) ([]byte, error) {
	if r.pos+n > len(r.b) {
		return nil, errors.New("file ends inside a value")
	}
	v := r.b[r.pos : r.pos+n]
	r.pos += n
	return append([]byte(nil), v...), nil
}

func (r *binReader) mapBody(m *BinNode) error {
	for {
		if r.pos >= len(r.b) {
			return errors.New("file ends inside a map")
		}
		t := r.b[r.pos]
		r.pos++
		if t == typeEnd {
			return nil
		}
		key, err := r.cstring()
		if err != nil {
			return err
		}
		n := &BinNode{Type: t, Key: key}
		switch t {
		case typeMap:
			err = r.mapBody(n)
		case typeString:
			n.Str, err = r.cstring()
		case typeInt:
			var v []byte
			if v, err = r.fixed(4); err == nil {
				n.Int = binary.LittleEndian.Uint32(v)
			}
		case typeFloat, typePtr, typeColor:
			n.Raw, err = r.fixed(4)
		case typeUint64, typeInt64:
			n.Raw, err = r.fixed(8)
		default:
			return fmt.Errorf("unsupported value type 0x%02x for %q", t, key)
		}
		if err != nil {
			return err
		}
		m.Children = append(m.Children, n)
	}
}

// Bytes writes the document back out. Call it on the root from ParseBinary.
func (n *BinNode) Bytes() []byte {
	var b bytes.Buffer
	writeBody(&b, n)
	return b.Bytes()
}

func writeBody(b *bytes.Buffer, m *BinNode) {
	for _, c := range m.Children {
		b.WriteByte(c.Type)
		b.WriteString(c.Key)
		b.WriteByte(0)
		switch c.Type {
		case typeMap:
			writeBody(b, c)
			continue // writeBody wrote its end marker
		case typeString:
			b.WriteString(c.Str)
			b.WriteByte(0)
		case typeInt:
			binary.Write(b, binary.LittleEndian, c.Int)
		default:
			b.Write(c.Raw)
		}
	}
	b.WriteByte(typeEnd)
}
