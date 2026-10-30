// Package vdf parses Valve's text KeyValues format (libraryfolders.vdf,
// appmanifest_*.acf). It is read-only and tolerant of comments and
// unquoted tokens; conditionals like [$WIN32] are skipped.
package vdf

import (
	"errors"
	"fmt"
	"strings"
)

// Node is either a string value (Children == nil) or a block.
type Node struct {
	Value    string
	Children map[string]*Node
	Order    []string
}

// Get walks a path of keys, case-insensitively, as Steam does.
func (n *Node) Get(path ...string) *Node {
	cur := n
	for _, k := range path {
		if cur == nil || cur.Children == nil {
			return nil
		}
		next, ok := cur.Children[strings.ToLower(k)]
		if !ok {
			return nil
		}
		cur = next
	}
	return cur
}

// String returns the value at path, or "".
func (n *Node) String(path ...string) string {
	if v := n.Get(path...); v != nil && v.Children == nil {
		return v.Value
	}
	return ""
}

// Keys returns child keys in file order (original case).
func (n *Node) Keys() []string { return n.Order }

type parser struct {
	s   string
	pos int
}

// Parse parses a KeyValues document into a root block.
func Parse(src string) (*Node, error) {
	src = strings.TrimPrefix(src, "\ufeff")
	p := &parser{s: src}
	root := &Node{Children: map[string]*Node{}}
	if err := p.block(root, false); err != nil {
		return nil, err
	}
	return root, nil
}

func (p *parser) skip() {
	for p.pos < len(p.s) {
		c := p.s[p.pos]
		switch {
		case c == ' ' || c == '\t' || c == '\r' || c == '\n':
			p.pos++
		case strings.HasPrefix(p.s[p.pos:], "//"):
			for p.pos < len(p.s) && p.s[p.pos] != '\n' {
				p.pos++
			}
		default:
			return
		}
	}
}

// token returns the next token; quoted strings are unescaped.
func (p *parser) token() (string, bool, error) {
	p.skip()
	if p.pos >= len(p.s) {
		return "", false, nil
	}
	c := p.s[p.pos]
	if c == '{' || c == '}' {
		p.pos++
		return string(c), true, nil
	}
	if c == '"' {
		p.pos++
		var b strings.Builder
		for p.pos < len(p.s) {
			c := p.s[p.pos]
			if c == '\\' && p.pos+1 < len(p.s) {
				p.pos++
				switch e := p.s[p.pos]; e {
				case 'n':
					b.WriteByte('\n')
				case 't':
					b.WriteByte('\t')
				default:
					b.WriteByte(e)
				}
				p.pos++
				continue
			}
			if c == '"' {
				p.pos++
				return b.String(), true, nil
			}
			b.WriteByte(c)
			p.pos++
		}
		return "", false, errors.New("vdf: unterminated string")
	}
	start := p.pos
	for p.pos < len(p.s) && !strings.ContainsRune(" \t\r\n{}\"", rune(p.s[p.pos])) {
		p.pos++
	}
	return p.s[start:p.pos], true, nil
}

func (p *parser) block(n *Node, nested bool) error {
	for {
		key, ok, err := p.token()
		if err != nil {
			return err
		}
		if !ok {
			if nested {
				return errors.New("vdf: unexpected end of input")
			}
			return nil
		}
		if key == "}" {
			if !nested {
				return errors.New("vdf: unexpected '}'")
			}
			return nil
		}
		if key == "{" {
			return fmt.Errorf("vdf: unexpected '{' at %d", p.pos)
		}
		val, ok, err := p.token()
		if err != nil {
			return err
		}
		if !ok {
			return errors.New("vdf: key without value")
		}
		// Platform conditional after a value, e.g. "key" "val" [$WIN32]
		p.skipConditional()
		child := &Node{}
		if val == "{" {
			child.Children = map[string]*Node{}
			if err := p.block(child, true); err != nil {
				return err
			}
		} else {
			child.Value = val
		}
		lk := strings.ToLower(key)
		if _, dup := n.Children[lk]; !dup {
			n.Order = append(n.Order, key)
		}
		n.Children[lk] = child
	}
}

func (p *parser) skipConditional() {
	p.skip()
	if p.pos < len(p.s) && p.s[p.pos] == '[' {
		if end := strings.IndexByte(p.s[p.pos:], ']'); end >= 0 {
			p.pos += end + 1
		}
	}
}
