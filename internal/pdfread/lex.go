// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package pdfread

import (
	"errors"
	"strconv"

	"github.com/sergev/dejapdf/internal/pdf"
)

// keyword is a bare token other than true, false and null: obj, R,
// stream, and stray operators.
type keyword string

// maxDepth limits nesting of arrays and dictionaries.
const maxDepth = 500

var errSyntax = errors.New("pdf: syntax error")

// lexer reads objects from b, starting at pos.
type lexer struct {
	b   []byte
	pos int
}

func isSpace(c byte) bool {
	switch c {
	case 0, '\t', '\n', '\f', '\r', ' ':
		return true
	}
	return false
}

func isDelim(c byte) bool {
	switch c {
	case '(', ')', '<', '>', '[', ']', '{', '}', '/', '%':
		return true
	}
	return false
}

func isRegular(c byte) bool { return !isSpace(c) && !isDelim(c) }

// skipSpace skips white space and comments.
func (l *lexer) skipSpace() {
	for l.pos < len(l.b) {
		c := l.b[l.pos]
		if c == '%' {
			for l.pos < len(l.b) && l.b[l.pos] != '\n' && l.b[l.pos] != '\r' {
				l.pos++
			}
			continue
		} else if !isSpace(c) {
			return
		}
		l.pos++
	}
}

// regular reads a run of regular characters.
func (l *lexer) regular() []byte {
	i := l.pos
	for l.pos < len(l.b) && isRegular(l.b[l.pos]) {
		l.pos++
	}
	return l.b[i:l.pos]
}

// object reads one object. Bare keywords come back as keyword values;
// "N G R" as pdf.Ref. Closing delimiters are errors.
func (l *lexer) object(depth int) (any, error) {
	if depth > maxDepth {
		return nil, errSyntax
	}
	l.skipSpace()
	if l.pos >= len(l.b) {
		return nil, errSyntax
	}
	switch c := l.b[l.pos]; c {
	case '/':
		l.pos++
		return l.name(), nil
	case '(':
		l.pos++
		return l.literal(), nil
	case '<':
		if l.pos+1 < len(l.b) && l.b[l.pos+1] == '<' {
			l.pos += 2
			return l.dict(depth)
		}
		l.pos++
		return l.hex(), nil
	case '[':
		l.pos++
		a := pdf.Array{}
		for {
			l.skipSpace()
			if l.pos >= len(l.b) {
				return a, errSyntax
			}
			if l.b[l.pos] == ']' {
				l.pos++
				return a, nil
			}
			v, err := l.object(depth + 1)
			if err != nil {
				return a, err
			}
			if kw, ok := v.(keyword); ok {
				// A stray word is dropped; one that ends an object
				// means the array is not closed.
				if endsObject(kw) {
					return a, errSyntax
				}
				continue
			}
			a = append(a, v)
		}
	case '{', '}':
		l.pos++
		return keyword(c), nil
	case ')', '>', ']':
		return nil, errSyntax
	}
	tok := l.regular()
	if len(tok) == 0 {
		l.pos++
		return nil, errSyntax
	}
	switch c := tok[0]; {
	case c >= '0' && c <= '9' || c == '+' || c == '-' || c == '.':
		return l.number(tok), nil
	}
	switch string(tok) {
	case "true":
		return true, nil
	case "false":
		return false, nil
	case "null":
		return nil, nil
	}
	return keyword(tok), nil
}

// number parses tok; an integer followed by "G R" is a reference.
func (l *lexer) number(tok []byte) any {
	n, err := strconv.ParseInt(string(tok), 10, 64)
	if err != nil {
		return parseReal(tok)
	}
	if n >= 0 {
		save := l.pos
		l.skipSpace()
		if g := l.regular(); len(g) > 0 && isDigits(g) {
			l.skipSpace()
			if r := l.regular(); string(r) == "R" {
				return pdf.Ref(n)
			}
		}
		l.pos = save
	}
	return int(n)
}

func isDigits(b []byte) bool {
	for _, c := range b {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// parseReal is lenient: junk such as "--1" or "0.5.1" reads as much as
// makes sense, as viewers do.
func parseReal(tok []byte) float64 {
	neg := false
	i := 0
	for i < len(tok) && (tok[i] == '+' || tok[i] == '-') {
		neg = neg != (tok[i] == '-')
		i++
	}
	j := i
	dot := false
	for j < len(tok) && (tok[j] >= '0' && tok[j] <= '9' || tok[j] == '.' && !dot) {
		dot = dot || tok[j] == '.'
		j++
	}
	f, _ := strconv.ParseFloat(string(tok[i:j]), 64)
	if neg {
		f = -f
	}
	return f
}

func unhex(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return -1
}

// name reads a name after the slash, decoding #xx.
func (l *lexer) name() pdf.Name {
	tok := l.regular()
	var b []byte
	for i := 0; i < len(tok); i++ {
		if tok[i] == '#' && i+2 < len(tok) && unhex(tok[i+1]) >= 0 && unhex(tok[i+2]) >= 0 {
			if b == nil {
				b = append([]byte{}, tok[:i]...)
			}
			b = append(b, byte(unhex(tok[i+1])<<4|unhex(tok[i+2])))
			i += 2
		} else if b != nil {
			b = append(b, tok[i])
		}
	}
	if b == nil {
		return pdf.Name(tok)
	}
	return pdf.Name(b)
}

// literal reads a (string) after the open paren (7.3.4.2).
func (l *lexer) literal() pdf.String {
	var b []byte
	nest := 0
	for l.pos < len(l.b) {
		c := l.b[l.pos]
		l.pos++
		switch c {
		case '(':
			nest++
		case ')':
			if nest == 0 {
				return pdf.String(b)
			}
			nest--
		case '\r':
			// An end of line is \n, whatever its form.
			if l.pos < len(l.b) && l.b[l.pos] == '\n' {
				l.pos++
			}
			c = '\n'
		case '\\':
			if l.pos >= len(l.b) {
				return pdf.String(b)
			}
			c = l.b[l.pos]
			l.pos++
			switch c {
			case 'n':
				c = '\n'
			case 'r':
				c = '\r'
			case 't':
				c = '\t'
			case 'b':
				c = '\b'
			case 'f':
				c = '\f'
			case '\r':
				if l.pos < len(l.b) && l.b[l.pos] == '\n' {
					l.pos++
				}
				continue
			case '\n':
				continue
			default:
				if c >= '0' && c <= '7' {
					v := int(c - '0')
					for k := 0; k < 2 && l.pos < len(l.b) && l.b[l.pos] >= '0' && l.b[l.pos] <= '7'; k++ {
						v = v<<3 | int(l.b[l.pos]-'0')
						l.pos++
					}
					c = byte(v)
				}
			}
		}
		b = append(b, c)
	}
	return pdf.String(b)
}

// hex reads a <hex string> after the open bracket; an odd final digit
// is followed by 0.
func (l *lexer) hex() pdf.String {
	var b []byte
	d := -1
	for l.pos < len(l.b) {
		c := l.b[l.pos]
		l.pos++
		if c == '>' {
			break
		}
		v := unhex(c)
		if v < 0 {
			continue
		}
		if d < 0 {
			d = v
		} else {
			b = append(b, byte(d<<4|v))
			d = -1
		}
	}
	if d >= 0 {
		b = append(b, byte(d<<4))
	}
	return pdf.String(b)
}

// dict reads a dictionary after <<. A key without a value is dropped;
// null values are dropped (7.3.7).
func (l *lexer) dict(depth int) (pdf.Dict, error) {
	d := pdf.Dict{}
	for {
		l.skipSpace()
		if l.pos >= len(l.b) {
			return d, errSyntax
		}
		if l.b[l.pos] == '>' {
			l.pos++
			if l.pos < len(l.b) && l.b[l.pos] == '>' {
				l.pos++
			}
			return d, nil
		}
		k, err := l.object(depth + 1)
		if err != nil {
			return d, err
		}
		key, ok := k.(pdf.Name)
		if !ok {
			// Junk where a key should be: skip it.
			if kw, ok := k.(keyword); ok && endsObject(kw) {
				return d, errSyntax
			}
			continue
		}
		l.skipSpace()
		if l.pos < len(l.b) && l.b[l.pos] == '>' {
			continue
		}
		v, err := l.object(depth + 1)
		if err != nil {
			return d, err
		}
		if kw, ok := v.(keyword); ok {
			// A key with a stray word for its value is dropped.
			if endsObject(kw) {
				return d, errSyntax
			}
			continue
		}
		if v != nil {
			d[string(key)] = v
		}
	}
}

// endsObject reports whether a keyword can only follow a complete object.
func endsObject(kw keyword) bool {
	return kw == "endobj" || kw == "stream" || kw == "obj" || kw == "endstream"
}
