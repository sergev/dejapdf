// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package pdfread

import (
	"bytes"

	"github.com/sergev/dejapdf/internal/pdf"
)

// InlineImage is the operand of a BI operator: its dictionary as
// written (abbreviated keys) and its data, still encoded.
type InlineImage struct {
	Dict pdf.Dict
	Data []byte
}

// Content calls fn for each operator of a content stream (7.8.2) with
// its operands; an inline image comes as operator "BI" with one
// *InlineImage operand. Damaged syntax is skipped. fn returns false to
// stop.
func Content(data []byte, fn func(op string, args []any) bool) {
	l := &lexer{b: data}
	var args []any
	for {
		l.skipSpace()
		if l.pos >= len(data) {
			return
		}
		v, err := l.object(0)
		if err != nil {
			// A stray delimiter: skip it and drop the operands.
			if l.pos < len(data) && !isRegular(data[l.pos]) && !isSpace(data[l.pos]) {
				l.pos++
			}
			args = args[:0]
			continue
		}
		kw, ok := v.(keyword)
		if !ok {
			if len(args) < 64 {
				args = append(args, v)
			}
			continue
		}
		op := string(kw)
		if op == "BI" {
			im := l.inline()
			if im == nil {
				return
			}
			if !fn("BI", []any{im}) {
				return
			}
		} else if !fn(op, args) {
			return
		}
		args = args[:0]
	}
}

// inline reads an inline image after BI: key-value pairs up to ID, one
// white space byte, the data, then EI.
func (l *lexer) inline() *InlineImage {
	d := pdf.Dict{}
	for {
		v, err := l.object(0)
		if err != nil {
			return nil
		}
		if v == keyword("ID") {
			break
		}
		key, ok := v.(pdf.Name)
		if !ok {
			continue
		}
		if v, err = l.object(0); err != nil {
			return nil
		}
		if v == keyword("ID") {
			break
		}
		d[string(key)] = v
	}
	if l.pos < len(l.b) && isSpace(l.b[l.pos]) {
		l.pos++
	}
	start := l.pos
	end := -1
	// Unfiltered data has a known length; check that EI follows it.
	if n := inlineLength(d); n >= 0 && start+n <= len(l.b) {
		e := &lexer{b: l.b, pos: start + n}
		e.skipSpace()
		if eiAt(l.b, e.pos) {
			end = start + n
			l.pos = e.pos + 2
		}
	}
	if end < 0 {
		// EI between white space (or at the end), as viewers find it.
		for p := start; ; p++ {
			k := bytes.Index(l.b[p:], []byte("EI"))
			if k < 0 {
				end, l.pos = len(l.b), len(l.b)
				break
			}
			p += k
			if p > start && isSpace(l.b[p-1]) && eiAt(l.b, p) {
				end = p - 1
				l.pos = p + 2
				break
			}
		}
	}
	return &InlineImage{Dict: d, Data: l.b[start:end]}
}

// eiAt reports whether EI, followed by white space, a delimiter or the
// end, is at p.
func eiAt(b []byte, p int) bool {
	return p+2 <= len(b) && b[p] == 'E' && b[p+1] == 'I' && (p+2 == len(b) || !isRegular(b[p+2]))
}

// inlineLength is the data length of an unfiltered inline image, or -1.
func inlineLength(d pdf.Dict) int {
	if d["F"] != nil || d["Filter"] != nil {
		return -1
	}
	num := func(a, b string) int {
		if n, ok := d[a].(int); ok {
			return n
		}
		n, _ := d[b].(int)
		return n
	}
	w, h, bpc := num("W", "Width"), num("H", "Height"), num("BPC", "BitsPerComponent")
	comps := 1
	cs := d["CS"]
	if cs == nil {
		cs = d["ColorSpace"]
	}
	if im, _ := d["IM"].(bool); im {
		bpc = 1
	} else if im, _ := d["ImageMask"].(bool); im {
		bpc = 1
	} else {
		switch cs {
		case pdf.Name("RGB"), pdf.Name("DeviceRGB"):
			comps = 3
		case pdf.Name("CMYK"), pdf.Name("DeviceCMYK"):
			comps = 4
		case pdf.Name("G"), pdf.Name("DeviceGray"):
		default:
			if a, ok := cs.(pdf.Array); !ok || len(a) == 0 || (a[0] != pdf.Name("I") && a[0] != pdf.Name("Indexed")) {
				return -1 // a named colour space: unknown here
			}
		}
	}
	if w <= 0 || h <= 0 || bpc <= 0 || w > 1<<16 || h > 1<<16 {
		return -1
	}
	return (w*comps*bpc + 7) / 8 * h
}
